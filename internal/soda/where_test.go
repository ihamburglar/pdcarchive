package soda

import "testing"

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
		{"amount > 500", "(data->>'amount')::numeric > 500"},
		{"filer_name = 'Smith'", "LOWER(data->>'filer_name') = LOWER('Smith')"},
		{"amount > 500 AND election_year = 2024", "((data->>'amount')::numeric > 500 AND (data->>'election_year')::numeric = 2024)"},
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
