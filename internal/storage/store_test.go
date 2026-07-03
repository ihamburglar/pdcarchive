package storage

import (
	"testing"

	"github.com/ihamburglar/pdcarchive/internal/models"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
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

func TestDatasetTableName(t *testing.T) {
	got, err := DatasetTableName("kv7h-kjye")
	if err != nil {
		t.Fatalf("DatasetTableName: %v", err)
	}
	if got != "dataset_kv7h_kjye_records" {
		t.Fatalf("table name = %q, want dataset_kv7h_kjye_records", got)
	}

	if _, err := DatasetTableName("bad;drop"); err != ErrInvalidDatasetID {
		t.Fatalf("invalid id error = %v, want ErrInvalidDatasetID", err)
	}
}

func TestEnsureDatasetTableAndUpsertRecords(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	if _, err := store.EnsureDatasetTable("kv7h-kjye"); err != nil {
		t.Fatalf("ensure table: %v", err)
	}
	if err := store.UpsertRecords("kv7h-kjye", []DatasetRecord{
		{RowID: "offset:0", Data: datatypes.JSON(`{"id":"1"}`)},
		{RowID: "offset:1", Data: datatypes.JSON(`{"id":"1"}`)},
	}); err != nil {
		t.Fatalf("upsert records: %v", err)
	}

	count, err := store.CountDatasetRows("kv7h-kjye")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

func TestMigrateDatasetCopiesSharedRecordsAndReconcilesCount(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	shared := []models.Record{
		{DatasetID: "kv7h-kjye", RowID: "old-1", Data: datatypes.JSON(`{"id":"1"}`)},
		{DatasetID: "kv7h-kjye", RowID: "old-2", Data: datatypes.JSON(`{"id":"2"}`)},
		{DatasetID: "other", RowID: "old-3", Data: datatypes.JSON(`{"id":"3"}`)},
	}
	if err := db.Create(&shared).Error; err != nil {
		t.Fatalf("create shared records: %v", err)
	}

	count, err := store.MigrateDataset("kv7h-kjye", "Contributions")
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if count != 2 {
		t.Fatalf("migrated count = %d, want 2", count)
	}

	var ds models.Dataset
	if err := db.First(&ds, "id = ?", "kv7h-kjye").Error; err != nil {
		t.Fatalf("load dataset: %v", err)
	}
	if ds.RowCount != 2 {
		t.Fatalf("row_count = %d, want 2", ds.RowCount)
	}
}

func TestClearDatasetClearsDatasetTableAndSharedRecords(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	if err := store.UpsertRecords("kv7h-kjye", []DatasetRecord{
		{RowID: "offset:0", Data: datatypes.JSON(`{"id":"1"}`)},
	}); err != nil {
		t.Fatalf("upsert dataset records: %v", err)
	}
	if err := db.Create(&models.Record{DatasetID: "kv7h-kjye", RowID: "old-1", Data: datatypes.JSON(`{"id":"1"}`)}).Error; err != nil {
		t.Fatalf("create shared record: %v", err)
	}

	deleted, err := store.ClearDataset("kv7h-kjye")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
	count, err := store.CountDatasetRows("kv7h-kjye")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}
