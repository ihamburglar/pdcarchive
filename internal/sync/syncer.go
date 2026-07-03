package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ihamburglar/pdcarchive/internal/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrCancelled = errors.New("sync cancelled")

type runningSync struct {
	cancel context.CancelFunc
}

type Syncer struct {
	db           *gorm.DB
	client       *Client
	pageSize     int
	pageInterval time.Duration
	mu           sync.Mutex
	active       map[string]runningSync
}

func NewSyncer(db *gorm.DB, client *Client, pageSize int, pageInterval time.Duration) *Syncer {
	if pageSize <= 0 {
		pageSize = 1000
	}
	if pageInterval <= 0 {
		pageInterval = time.Second
	}
	return &Syncer{
		db:           db,
		client:       client,
		pageSize:     pageSize,
		pageInterval: pageInterval,
		active:       make(map[string]runningSync),
	}
}

func (s *Syncer) IsRunning(datasetID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.active[datasetID]
	return ok
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

	var existing models.Dataset
	s.db.First(&existing, "id = ?", datasetID)

	offset := int(existing.SyncOffset)
	var totalSynced int64
	var lastModified *time.Time
	firstPage := true

	log.Printf("sync %s: resuming from offset %d (page size %d, interval %s)", datasetID, offset, s.pageSize, s.pageInterval)

	for {
		if err := ctx.Err(); err != nil {
			return totalSynced, ErrCancelled
		}

		if !firstPage {
			if err := sleepWithContext(ctx, s.pageInterval); err != nil {
				return totalSynced, ErrCancelled
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

		batch := make([]models.Record, 0, len(rows))
		for _, raw := range rows {
			rowID, _ := extractRowMeta(raw)
			if rowID == "" {
				continue
			}
			batch = append(batch, models.Record{
				DatasetID: datasetID,
				RowID:     rowID,
				Data:      datatypes.JSON(raw),
			})
		}

		if len(batch) > 0 {
			if err := s.db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "dataset_id"}, {Name: "row_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"data"}),
			}).CreateInBatches(&batch, 500).Error; err != nil {
				return totalSynced, fmt.Errorf("upsert batch: %w", err)
			}
			totalSynced += int64(len(batch))
		}

		offset += len(rows)
		s.saveProgress(datasetID, jobID, int64(offset), totalSynced)

		log.Printf("sync %s: offset %d, synced %d rows this run", datasetID, offset, totalSynced)

		if len(rows) < s.pageSize {
			break
		}
	}

	if err := ctx.Err(); err != nil {
		return totalSynced, ErrCancelled
	}

	now := time.Now()
	var count int64
	s.db.Model(&models.Record{}).Where("dataset_id = ?", datasetID).Count(&count)

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

func (s *Syncer) saveProgress(datasetID string, jobID uint, offset, rowsSynced int64) {
	s.db.Model(&models.Dataset{}).Where("id = ?", datasetID).Updates(map[string]interface{}{
		"sync_offset": offset,
	})
	s.db.Model(&models.SyncJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"last_offset": offset,
		"rows_synced": rowsSynced,
	})
}

func extractRowMeta(raw json.RawMessage) (rowID string, _ string) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", ""
	}
	if idRaw, ok := m["id"]; ok {
		var id string
		if err := json.Unmarshal(idRaw, &id); err == nil {
			rowID = id
		}
	}
	return rowID, ""
}
