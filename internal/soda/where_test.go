package soda

import (
	"strings"
	"testing"
)

func TestParseWhere(t *testing.T) {
	types := ColumnTypes{
		"amount":        "number",
		"election_year": "number",
		"filer_name":    "text",
	}

	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"amount > 500", "((data->>'amount')::numeric > (500)::numeric)"},
		{"filer_name = 'Smith'", "(LOWER(data->>'filer_name') = LOWER('Smith'))"},
		{"amount > 500 AND election_year = 2024", "(((data->>'amount')::numeric > (500)::numeric) AND ((data->>'election_year')::numeric = (2024)::numeric))"},
	}

	for _, tt := range tests {
		got, err := ParseWhere(tt.input, types)
		if err != nil {
			t.Fatalf("ParseWhere(%q): %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("ParseWhere(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseWhereINAndLike(t *testing.T) {
	types := ColumnTypes{"party": "text", "amount": "number"}
	got, err := ParseWhere("party IN ('DEMOCRATIC', 'REPUBLICAN')", types)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "IN") {
		t.Fatalf("expected IN clause, got %s", got)
	}

	got, err = ParseWhere("party LIKE 'DEM%'", types)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "LIKE") {
		t.Fatalf("expected LIKE, got %s", got)
	}
}

func TestParseWhereNOT(t *testing.T) {
	types := ColumnTypes{"amount": "number"}
	got, err := ParseWhere("NOT amount > 500", types)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "(NOT ") {
		t.Fatalf("expected NOT wrapper, got %s", got)
	}
}
