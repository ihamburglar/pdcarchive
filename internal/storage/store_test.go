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
	if err := db.AutoMigrate(&models.Dataset{}, &models.SyncJob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestDatasetTableName(t *testing.T) {
	got, err := DatasetTableName("kv7h-kjye")
	if err != nil {
		t.Fatalf("DatasetTableName: %v", err)
	}
	if got != "dataset_contributions" {
		t.Fatalf("table name = %q, want dataset_contributions", got)
	}

	got, err = DatasetTableName("abcd-1234")
	if err != nil {
		t.Fatalf("DatasetTableName fallback: %v", err)
	}
	if got != "dataset_abcd_1234" {
		t.Fatalf("fallback table name = %q, want dataset_abcd_1234", got)
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

func TestClearDatasetClearsReadableDatasetTable(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	if err := store.UpsertRecords("kv7h-kjye", []DatasetRecord{
		{RowID: "offset:0", Data: datatypes.JSON(`{"id":"1"}`)},
	}); err != nil {
		t.Fatalf("upsert dataset records: %v", err)
	}

	deleted, err := store.ClearDataset("kv7h-kjye")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	count, err := store.CountDatasetRows("kv7h-kjye")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}
