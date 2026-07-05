package datasets

import (
	"strings"
	"testing"
)

func TestAllDatasetsUniqueIDs(t *testing.T) {
	seen := make(map[string]bool)
	for _, d := range All {
		if seen[d.ID] {
			t.Fatalf("duplicate dataset ID: %q", d.ID)
		}
		seen[d.ID] = true
	}
	if len(All) != 26 {
		t.Fatalf("expected 26 datasets, got %d", len(All))
	}
}

func TestAllDatasetsUniqueTableNames(t *testing.T) {
	seen := make(map[string]bool)
	for _, d := range All {
		if seen[d.TableName] {
			t.Fatalf("duplicate table name: %q", d.TableName)
		}
		seen[d.TableName] = true
		if !strings.HasPrefix(d.TableName, "dataset_") {
			t.Fatalf("table name %q must start with dataset_", d.TableName)
		}
	}
}

func TestByID(t *testing.T) {
	d, ok := ByID("kv7h-kjye")
	if !ok {
		t.Fatal("expected kv7h-kjye to be found")
	}
	if d.TableName != "dataset_contributions" {
		t.Fatalf("table = %q, want dataset_contributions", d.TableName)
	}

	if _, ok := ByID("not-a-dataset"); ok {
		t.Fatal("expected unknown ID to not be found")
	}
}

func TestTableName(t *testing.T) {
	name, err := TableName("tijg-9zyp")
	if err != nil {
		t.Fatalf("TableName: %v", err)
	}
	if name != "dataset_expenditures" {
		t.Fatalf("table = %q, want dataset_expenditures", name)
	}

	if _, err := TableName("unknown"); err != ErrUnknownDataset {
		t.Fatalf("err = %v, want ErrUnknownDataset", err)
	}
}

func TestIDs(t *testing.T) {
	ids := IDs()
	if len(ids) != len(All) {
		t.Fatalf("IDs length = %d, want %d", len(ids), len(All))
	}
	if ids[0] != All[0].ID {
		t.Fatalf("first ID = %q, want %q", ids[0], All[0].ID)
	}
}
