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

func TestLegacyDatasetTableName(t *testing.T) {
	got, err := LegacyDatasetTableName("kv7h-kjye")
	if err != nil {
		t.Fatalf("LegacyDatasetTableName: %v", err)
	}
	if got != "dataset_kv7h_kjye_records" {
		t.Fatalf("legacy table name = %q, want dataset_kv7h_kjye_records", got)
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

func TestRenameDatasetTableRenamesLegacyTable(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	oldTable, err := LegacyDatasetTableName("kv7h-kjye")
	if err != nil {
		t.Fatalf("legacy table name: %v", err)
	}
	if err := db.Table(oldTable).AutoMigrate(&DatasetRecord{}); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if err := db.Table(oldTable).Create(&DatasetRecord{RowID: "offset:0", Data: datatypes.JSON(`{"id":"1"}`)}).Error; err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	result, err := store.RenameDatasetTable("kv7h-kjye")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if result.Action != "renamed" {
		t.Fatalf("action = %q, want renamed", result.Action)
	}
	if db.Migrator().HasTable(oldTable) {
		t.Fatal("legacy table still exists")
	}
	if !db.Migrator().HasTable("dataset_contributions") {
		t.Fatal("readable table was not created")
	}
	if result.Rows != 1 {
		t.Fatalf("rows = %d, want 1", result.Rows)
	}
}

func TestRenameDatasetTableMergesWhenNewExists(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	oldTable, err := LegacyDatasetTableName("kv7h-kjye")
	if err != nil {
		t.Fatalf("legacy table name: %v", err)
	}
	if err := db.Table(oldTable).AutoMigrate(&DatasetRecord{}); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if err := db.Table(oldTable).Create(&DatasetRecord{RowID: "offset:0", Data: datatypes.JSON(`{"id":"old"}`)}).Error; err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	if err := store.UpsertRecords("kv7h-kjye", []DatasetRecord{
		{RowID: "offset:1", Data: datatypes.JSON(`{"id":"new"}`)},
	}); err != nil {
		t.Fatalf("insert new row: %v", err)
	}

	result, err := store.RenameDatasetTable("kv7h-kjye")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if result.Action != "merged" {
		t.Fatalf("action = %q, want merged", result.Action)
	}
	if db.Migrator().HasTable(oldTable) {
		t.Fatal("legacy table still exists")
	}
	if result.Rows != 2 {
		t.Fatalf("rows = %d, want 2", result.Rows)
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
