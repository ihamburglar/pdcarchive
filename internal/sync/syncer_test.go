package sync

import (
	"testing"

	"github.com/ihamburglar/pdcarchive/internal/models"
	"github.com/ihamburglar/pdcarchive/internal/storage"
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
	if err := db.AutoMigrate(&models.Dataset{}, &models.SyncJob{}); err != nil {
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

	s.upsertDatasetOffset("kv7h-kjye", "Contributions", 5000)

	var ds models.Dataset
	if err := db.First(&ds, "id = ?", "kv7h-kjye").Error; err != nil {
		t.Fatalf("dataset row not created: %v", err)
	}
	if ds.SyncOffset != 5000 {
		t.Fatalf("sync_offset = %d, want 5000", ds.SyncOffset)
	}
	if ds.Name != "Contributions" {
		t.Fatalf("name = %q, want Contributions", ds.Name)
	}
}

func TestSaveProgressPersistsOffsetWithoutExistingDataset(t *testing.T) {
	db := testSyncerDB(t)
	s := newTestSyncer(db)

	job := models.SyncJob{DatasetID: "kv7h-kjye", Status: models.SyncStatusRunning, Trigger: models.SyncTriggerManual}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}

	s.saveProgress("kv7h-kjye", "Contributions", job.ID, 3000, 2500)

	var ds models.Dataset
	if err := db.First(&ds, "id = ?", "kv7h-kjye").Error; err != nil {
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

func TestFinalizeDatasetProgressCountsDatasetTable(t *testing.T) {
	db := testSyncerDB(t)
	s := newTestSyncer(db)

	if err := s.store.UpsertRecords("kv7h-kjye", []storage.DatasetRecord{
		{RowID: "offset:0", Data: datatypes.JSON(`{"id":"1"}`)},
		{RowID: "offset:1", Data: datatypes.JSON(`{"id":"1"}`)},
	}); err != nil {
		t.Fatalf("upsert records: %v", err)
	}

	s.finalizeDatasetProgress("kv7h-kjye", "Contributions", 2000)

	var ds models.Dataset
	if err := db.First(&ds, "id = ?", "kv7h-kjye").Error; err != nil {
		t.Fatalf("dataset row not created: %v", err)
	}
	if ds.SyncOffset != 2000 {
		t.Fatalf("sync_offset = %d, want 2000", ds.SyncOffset)
	}
	if ds.RowCount != 2 {
		t.Fatalf("row_count = %d, want 2", ds.RowCount)
	}
}

func TestClearDatasetRemovesDatasetTableRowsAndResetsOffset(t *testing.T) {
	db := testSyncerDB(t)
	s := newTestSyncer(db)

	ds := models.Dataset{ID: "kv7h-kjye", Name: "Contributions", SyncOffset: 9000, RowCount: 3, Columns: datatypes.JSON("[]")}
	if err := db.Create(&ds).Error; err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if err := s.store.UpsertRecords("kv7h-kjye", []storage.DatasetRecord{
		{RowID: "offset:0", Data: datatypes.JSON(`{"id":"1"}`)},
		{RowID: "offset:1", Data: datatypes.JSON(`{"id":"2"}`)},
		{RowID: "offset:2", Data: datatypes.JSON(`{"id":"3"}`)},
	}); err != nil {
		t.Fatalf("create records: %v", err)
	}

	deleted, err := s.ClearDataset("kv7h-kjye")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("deleted = %d, want 3", deleted)
	}

	count, err := s.store.CountDatasetRows("kv7h-kjye")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("record count = %d, want 0", count)
	}

	var updated models.Dataset
	if err := db.First(&updated, "id = ?", "kv7h-kjye").Error; err != nil {
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

func TestRenameDatasetTablesBlocksWhileImportRunning(t *testing.T) {
	db := testSyncerDB(t)
	s := newTestSyncer(db)

	_, release, err := s.begin("running-id")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer release()

	_, err = s.RenameDatasetTables([]string{"kv7h-kjye"})
	if err != ErrImportInProgress {
		t.Fatalf("rename while import running: got %v, want ErrImportInProgress", err)
	}
}
