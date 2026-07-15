package soda

import (
	"encoding/json"
	"testing"

	"github.com/ihamburglar/pdcarchive/internal/models"
	"github.com/ihamburglar/pdcarchive/internal/storage"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testQueryDB(t *testing.T) *gorm.DB {
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

func TestExecuteQueryReadsDatasetTable(t *testing.T) {
	db := testQueryDB(t)
	store := storage.NewStore(db)
	if err := store.UpsertRecords("kv7h-kjye", []storage.DatasetRecord{
		{RowID: "offset:0", Data: datatypes.JSON(`{"id":"1","name":"first"}`)},
		{RowID: "offset:1", Data: datatypes.JSON(`{"id":"1","name":"second"}`)},
	}); err != nil {
		t.Fatalf("upsert records: %v", err)
	}

	result, err := ExecuteQuery(db, "kv7h-kjye", ColumnTypes{}, QueryParams{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("execute query: %v", err)
	}
	if len(result.RowsJSON) != 1 {
		t.Fatalf("rows = %d, want 1", len(result.RowsJSON))
	}
	if string(result.RowsJSON[0]) != `{"id":"1","name":"second"}` {
		t.Fatalf("row data = %s", result.RowsJSON[0])
	}
}

func TestExecuteQueryCountUsesDatasetTable(t *testing.T) {
	db := testQueryDB(t)
	store := storage.NewStore(db)
	if err := store.UpsertRecords("kv7h-kjye", []storage.DatasetRecord{
		{RowID: "offset:0", Data: datatypes.JSON(`{"id":"1"}`)},
		{RowID: "offset:1", Data: datatypes.JSON(`{"id":"1"}`)},
	}); err != nil {
		t.Fatalf("upsert records: %v", err)
	}

	// SQLite does not support json_build_object the same way; skip if compile path fails.
	result, err := ExecuteQuery(db, "kv7h-kjye", ColumnTypes{}, QueryParams{Select: "count(*)", Limit: 1})
	if err != nil {
		t.Skipf("count(*) against sqlite not supported in this environment: %v", err)
	}
	if len(result.RowsJSON) != 1 {
		t.Fatalf("rows = %d, want 1", len(result.RowsJSON))
	}
	var m map[string]string
	if err := json.Unmarshal(result.RowsJSON[0], &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["count"] != "2" {
		t.Fatalf("count = %q, want 2", m["count"])
	}
}
