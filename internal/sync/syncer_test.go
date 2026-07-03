package sync

import (
	"testing"

	"github.com/ihamburglar/pdcarchive/internal/models"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testSyncerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Dataset{}, &models.Record{}, &models.SyncJob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func newTestSyncer(db *gorm.DB) *Syncer {
	return NewSyncer(db, NewClient("https://example.com", ""), 1000, 0)
}

func TestUpsertDatasetOffsetCreatesRow(t *testing.T) {
	db := testSyncerDB(t)
	s := newTestSyncer(db)

	s.upsertDatasetOffset("abc123", "Test Dataset", 5000)

	var ds models.Dataset
	if err := db.First(&ds, "id = ?", "abc123").Error; err != nil {
		t.Fatalf("dataset row not created: %v", err)
	}
	if ds.SyncOffset != 5000 {
		t.Fatalf("sync_offset = %d, want 5000", ds.SyncOffset)
	}
	if ds.Name != "Test Dataset" {
		t.Fatalf("name = %q, want Test Dataset", ds.Name)
	}
}

func TestSaveProgressPersistsOffsetWithoutExistingDataset(t *testing.T) {
	db := testSyncerDB(t)
	s := newTestSyncer(db)

	job := models.SyncJob{DatasetID: "abc123", Status: models.SyncStatusRunning, Trigger: models.SyncTriggerManual}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}

	s.saveProgress("abc123", "Test Dataset", job.ID, 3000, 2500)

	var ds models.Dataset
	if err := db.First(&ds, "id = ?", "abc123").Error; err != nil {
		t.Fatalf("dataset row not created: %v", err)
	}
	if ds.SyncOffset != 3000 {
		t.Fatalf("sync_offset = %d, want 3000", ds.SyncOffset)
	}

	var updated models.SyncJob
	if err := db.First(&updated, job.ID).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}
	if updated.LastOffset != 3000 || updated.RowsSynced != 2500 {
		t.Fatalf("job progress = offset %d rows %d, want 3000/2500", updated.LastOffset, updated.RowsSynced)
	}
}

func TestFinalizeDatasetProgressUpdatesRowCount(t *testing.T) {
	db := testSyncerDB(t)
	s := newTestSyncer(db)

	records := []models.Record{
		{DatasetID: "abc123", RowID: "1", Data: datatypes.JSON(`{"id":"1"}`)},
		{DatasetID: "abc123", RowID: "2", Data: datatypes.JSON(`{"id":"2"}`)},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatalf("create records: %v", err)
	}

	s.finalizeDatasetProgress("abc123", "Test Dataset", 2000)

	var ds models.Dataset
	if err := db.First(&ds, "id = ?", "abc123").Error; err != nil {
		t.Fatalf("dataset row not created: %v", err)
	}
	if ds.SyncOffset != 2000 {
		t.Fatalf("sync_offset = %d, want 2000", ds.SyncOffset)
	}
	if ds.RowCount != 2 {
		t.Fatalf("row_count = %d, want 2", ds.RowCount)
	}
}

func TestClearDatasetRemovesRecordsAndResetsOffset(t *testing.T) {
	db := testSyncerDB(t)
	s := newTestSyncer(db)

	ds := models.Dataset{ID: "abc123", Name: "Test", SyncOffset: 9000, RowCount: 3, Columns: datatypes.JSON("[]")}
	if err := db.Create(&ds).Error; err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	records := []models.Record{
		{DatasetID: "abc123", RowID: "1", Data: datatypes.JSON(`{"id":"1"}`)},
		{DatasetID: "abc123", RowID: "2", Data: datatypes.JSON(`{"id":"2"}`)},
		{DatasetID: "abc123", RowID: "3", Data: datatypes.JSON(`{"id":"3"}`)},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatalf("create records: %v", err)
	}

	deleted, err := s.ClearDataset("abc123")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("deleted = %d, want 3", deleted)
	}

	var count int64
	db.Model(&models.Record{}).Where("dataset_id = ?", "abc123").Count(&count)
	if count != 0 {
		t.Fatalf("record count = %d, want 0", count)
	}

	var updated models.Dataset
	if err := db.First(&updated, "id = ?", "abc123").Error; err != nil {
		t.Fatalf("load dataset: %v", err)
	}
	if updated.SyncOffset != 0 || updated.RowCount != 0 {
		t.Fatalf("dataset not reset: offset=%d row_count=%d", updated.SyncOffset, updated.RowCount)
	}
	if updated.SyncedAt != nil {
		t.Fatal("synced_at should be cleared")
	}
}

func TestClearDatasetBlocksWhileImportRunning(t *testing.T) {
	db := testSyncerDB(t)
	s := newTestSyncer(db)

	_, release, err := s.begin("running-id")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer release()

	_, err = s.ClearDataset("other-id")
	if err != ErrImportInProgress {
		t.Fatalf("clear while import running: got %v, want ErrImportInProgress", err)
	}

	_, err = s.ClearDataset("running-id")
	if err != ErrSyncRunning {
		t.Fatalf("clear while same dataset running: got %v, want ErrSyncRunning", err)
	}
}
