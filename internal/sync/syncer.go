package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/ihamburglar/pdcarchive/internal/models"
	"github.com/ihamburglar/pdcarchive/internal/storage"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrCancelled        = errors.New("sync cancelled")
	ErrImportInProgress = errors.New("import in progress")
	ErrSyncRunning      = errors.New("sync already running for this dataset")
)

type runningSync struct {
	cancel context.CancelFunc
}

type Syncer struct {
	db              *gorm.DB
	client          *Client
	store           *storage.Store
	pageSize        int
	pageIntervalMin time.Duration
	pageIntervalMax time.Duration
	mu              sync.Mutex
	active          map[string]runningSync
}

func NewSyncer(db *gorm.DB, client *Client, pageSize int, pageIntervalMin, pageIntervalMax time.Duration) *Syncer {
	if pageSize <= 0 {
		pageSize = 1000
	}
	if pageIntervalMin <= 0 {
		pageIntervalMin = 5 * time.Second
	}
	if pageIntervalMax < pageIntervalMin {
		pageIntervalMax = pageIntervalMin
	}
	return &Syncer{
		db:              db,
		client:          client,
		store:           storage.NewStore(db),
		pageSize:        pageSize,
		pageIntervalMin: pageIntervalMin,
		pageIntervalMax: pageIntervalMax,
		active:          make(map[string]runningSync),
	}
}

func (s *Syncer) nextPageInterval() time.Duration {
	span := s.pageIntervalMax - s.pageIntervalMin
	if span <= 0 {
		return s.pageIntervalMin
	}
	return s.pageIntervalMin + time.Duration(rand.Int63n(int64(span)+1))
}

func (s *Syncer) IsRunning(datasetID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.active[datasetID]
	return ok
}

func (s *Syncer) AnyRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.active) > 0
}

func (s *Syncer) StopSync(datasetID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.active[datasetID]
	if !ok {
		return false
	}
	r.cancel()
	return true
}

func (s *Syncer) SyncDataset(datasetID, trigger string) error {
	ctx, release, err := s.begin(datasetID)
	if err != nil {
		return err
	}
	defer release()

	job := models.SyncJob{
		DatasetID: datasetID,
		Status:    models.SyncStatusRunning,
		Trigger:   trigger,
	}
	now := time.Now()
	job.StartedAt = &now
	if err := s.db.Create(&job).Error; err != nil {
		return err
	}

	rowsSynced, syncErr := s.syncDataset(ctx, datasetID, job.ID)
	job.RowsSynced = rowsSynced
	finished := time.Now()
	job.FinishedAt = &finished

	if syncErr != nil {
		if errors.Is(syncErr, ErrCancelled) {
			job.Status = models.SyncStatusCancelled
		} else {
			job.Status = models.SyncStatusFailed
			job.Error = syncErr.Error()
		}
		s.db.Save(&job)
		return syncErr
	}

	job.Status = models.SyncStatusCompleted
	s.db.Save(&job)
	return nil
}

func (s *Syncer) SyncDatasetAsync(datasetID, trigger string) bool {
	if s.IsRunning(datasetID) {
		return false
	}
	go func() {
		if err := s.SyncDataset(datasetID, trigger); err != nil {
			if errors.Is(err, ErrCancelled) {
				log.Printf("sync %s cancelled", datasetID)
			} else {
				log.Printf("sync %s failed: %v", datasetID, err)
			}
		} else {
			log.Printf("sync %s completed", datasetID)
		}
	}()
	return true
}

func (s *Syncer) SyncAll(datasetIDs []string, trigger string) {
	if s.AnyRunning() {
		log.Printf("sync all skipped (%s): import in progress", trigger)
		return
	}
	for _, id := range datasetIDs {
		if s.IsRunning(id) {
			log.Printf("skipping %s: sync already running", id)
			continue
		}
		if err := s.SyncDataset(id, trigger); err != nil {
			if errors.Is(err, ErrCancelled) {
				log.Printf("sync %s cancelled", id)
			} else {
				log.Printf("sync %s failed: %v", id, err)
			}
		}
	}
}

func (s *Syncer) SyncAllAsync(datasetIDs []string, trigger string) {
	go s.SyncAll(datasetIDs, trigger)
}

func (s *Syncer) begin(datasetID string) (context.Context, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.active[datasetID]; ok {
		return nil, nil, fmt.Errorf("sync already running for dataset %s", datasetID)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.active[datasetID] = runningSync{cancel: cancel}
	return ctx, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.active, datasetID)
	}, nil
}

func (s *Syncer) syncDataset(ctx context.Context, datasetID string, jobID uint) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, ErrCancelled
	}

	columns, err := s.client.FetchColumns(datasetID)
	if err != nil {
		return 0, fmt.Errorf("fetch columns: %w", err)
	}
	columnsJSON, _ := json.Marshal(columns)

	datasetName, _ := s.client.FetchDatasetName(datasetID)
	if datasetName == "" {
		datasetName = datasetID
	}
	if _, err := s.store.EnsureDatasetTable(datasetID); err != nil {
		return 0, fmt.Errorf("ensure dataset table: %w", err)
	}

	var existing models.Dataset
	s.db.First(&existing, "id = ?", datasetID)

	offset := int(existing.SyncOffset)
	var totalSynced int64
	var lastModified *time.Time
	firstPage := true

	log.Printf("sync %s: resuming from offset %d (page size %d, interval %s-%s)", datasetID, offset, s.pageSize, s.pageIntervalMin, s.pageIntervalMax)

	for {
		if err := ctx.Err(); err != nil {
			return s.abortSync(datasetID, datasetName, int64(offset), totalSynced)
		}

		if !firstPage {
			delay := s.nextPageInterval()
			log.Printf("sync %s: waiting %s before next page", datasetID, delay)
			if err := sleepWithContext(ctx, delay); err != nil {
				return s.abortSync(datasetID, datasetName, int64(offset), totalSynced)
			}
		}
		firstPage = false

		rows, headers, err := s.client.FetchPage(datasetID, offset, s.pageSize)
		if err != nil {
			return totalSynced, fmt.Errorf("fetch page offset %d: %w", offset, err)
		}
		if lm := ParseLastModified(headers); lm != nil {
			lastModified = lm
		}

		if len(rows) == 0 {
			break
		}

		batch := make([]storage.DatasetRecord, 0, len(rows))
		for i, raw := range rows {
			batch = append(batch, storage.DatasetRecord{
				RowID: fmt.Sprintf("offset:%d", offset+i),
				Data:  datatypes.JSON(raw),
			})
		}

		if len(batch) > 0 {
			if err := s.store.UpsertRecords(datasetID, batch); err != nil {
				return totalSynced, fmt.Errorf("upsert batch: %w", err)
			}
			totalSynced += int64(len(batch))
		}

		offset += len(rows)
		s.saveProgress(datasetID, datasetName, jobID, int64(offset), totalSynced)

		log.Printf("sync %s: offset %d, synced %d rows this run", datasetID, offset, totalSynced)

		if len(rows) < s.pageSize {
			break
		}
	}

	if err := ctx.Err(); err != nil {
		return s.abortSync(datasetID, datasetName, int64(offset), totalSynced)
	}

	now := time.Now()
	count, err := s.store.CountDatasetRows(datasetID)
	if err != nil {
		return totalSynced, fmt.Errorf("count dataset rows: %w", err)
	}

	dataset := models.Dataset{
		ID:           datasetID,
		Name:         datasetName,
		Columns:      datatypes.JSON(columnsJSON),
		LastModified: lastModified,
		SyncedAt:     &now,
		RowCount:     count,
		SyncOffset:   int64(offset),
	}
	if err := s.db.Save(&dataset).Error; err != nil {
		return totalSynced, fmt.Errorf("save dataset metadata: %w", err)
	}

	return totalSynced, nil
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (s *Syncer) saveProgress(datasetID, datasetName string, jobID uint, offset, rowsSynced int64) {
	s.upsertDatasetOffset(datasetID, datasetName, offset)
	s.db.Model(&models.SyncJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"last_offset": offset,
		"rows_synced": rowsSynced,
	})
}

func (s *Syncer) upsertDatasetOffset(datasetID, name string, offset int64) {
	if err := s.store.UpsertDatasetOffset(datasetID, name, offset); err != nil {
		log.Printf("sync %s: save offset failed: %v", datasetID, err)
	}
}

func (s *Syncer) finalizeDatasetProgress(datasetID, name string, offset int64) {
	count, err := s.store.CountDatasetRows(datasetID)
	if err != nil {
		log.Printf("sync %s: count rows failed: %v", datasetID, err)
		return
	}
	if err := s.store.UpdateDatasetStats(datasetID, name, offset, count); err != nil {
		log.Printf("sync %s: finalize progress failed: %v", datasetID, err)
	}
}

func (s *Syncer) abortSync(datasetID, name string, offset int64, totalSynced int64) (int64, error) {
	s.finalizeDatasetProgress(datasetID, name, offset)
	return totalSynced, ErrCancelled
}

func (s *Syncer) ClearDataset(datasetID string) (int64, error) {
	if s.IsRunning(datasetID) {
		return 0, ErrSyncRunning
	}
	if s.AnyRunning() {
		return 0, ErrImportInProgress
	}

	deleted, err := s.store.ClearDataset(datasetID)
	if err != nil {
		return 0, err
	}

	log.Printf("cleared %d records from dataset %s", deleted, datasetID)
	return deleted, nil
}
