package sync

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ihamburglar/pdcarchive/internal/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Syncer struct {
	db     *gorm.DB
	client *Client
	mu     sync.Mutex
	active map[string]bool
}

func NewSyncer(db *gorm.DB, client *Client) *Syncer {
	return &Syncer{
		db:     db,
		client: client,
		active: make(map[string]bool),
	}
}

func (s *Syncer) IsRunning(datasetID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[datasetID]
}

func (s *Syncer) SyncDataset(datasetID, trigger string) error {
	if !s.tryStart(datasetID) {
		return fmt.Errorf("sync already running for dataset %s", datasetID)
	}
	defer s.finish(datasetID)

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

	rowsSynced, err := s.syncDataset(datasetID)
	job.RowsSynced = rowsSynced
	finished := time.Now()
	job.FinishedAt = &finished

	if err != nil {
		job.Status = models.SyncStatusFailed
		job.Error = err.Error()
		s.db.Save(&job)
		return err
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
			log.Printf("sync %s failed: %v", datasetID, err)
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
			log.Printf("sync %s failed: %v", id, err)
		}
	}
}

func (s *Syncer) SyncAllAsync(datasetIDs []string, trigger string) {
	go s.SyncAll(datasetIDs, trigger)
}

func (s *Syncer) tryStart(datasetID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[datasetID] {
		return false
	}
	s.active[datasetID] = true
	return true
}

func (s *Syncer) finish(datasetID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, datasetID)
}

func (s *Syncer) syncDataset(datasetID string) (int64, error) {
	columns, err := s.client.FetchColumns(datasetID)
	if err != nil {
		return 0, fmt.Errorf("fetch columns: %w", err)
	}
	columnsJSON, _ := json.Marshal(columns)

	datasetName, _ := s.client.FetchDatasetName(datasetID)
	if datasetName == "" {
		datasetName = datasetID
	}

	offset := 0
	var totalSynced int64
	var lastModified *time.Time

	for {
		rows, headers, err := s.client.FetchPage(datasetID, offset)
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

		log.Printf("sync %s: offset %d, synced %d rows so far", datasetID, offset, totalSynced)

		if len(rows) < pageSize {
			break
		}
		offset += pageSize
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
	}
	if err := s.db.Save(&dataset).Error; err != nil {
		return totalSynced, fmt.Errorf("save dataset metadata: %w", err)
	}

	return totalSynced, nil
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
