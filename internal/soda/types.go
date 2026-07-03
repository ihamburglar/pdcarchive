package soda

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ihamburglar/pdcarchive/internal/sync"
)

type ColumnTypes map[string]string

func BuildColumnTypes(columns []sync.ColumnMeta) ColumnTypes {
	types := make(ColumnTypes, len(columns))
	for _, c := range columns {
		field := c.FieldName
		if field == "" {
			field = c.Name
		}
		types[strings.ToLower(field)] = c.DataTypeName
	}
	return types
}

func SodaFields(columns []sync.ColumnMeta) []string {
	fields := make([]string, 0, len(columns))
	for _, c := range columns {
		field := c.FieldName
		if field == "" {
			field = c.Name
		}
		fields = append(fields, field)
	}
	return fields
}

func SodaTypes(columns []sync.ColumnMeta) []string {
	types := make([]string, 0, len(columns))
	for _, c := range columns {
		types = append(types, mapDataType(c.DataTypeName))
	}
	return types
}

func mapDataType(dt string) string {
	switch strings.ToLower(dt) {
	case "number":
		return "number"
	case "floating timestamp", "calendar date", "fixed timestamp":
		return "floating_timestamp"
	case "url":
		return "url"
	case "location", "point":
		return "point"
	default:
		return "text"
	}
}

func FormatLastModified(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(httpTimeFormat)
}

const httpTimeFormat = "Mon, 02 Jan 2006 15:04:05 GMT"

func FieldsJSON(fields []string) string {
	b, _ := json.Marshal(fields)
	return string(b)
}

func TypesJSON(types []string) string {
	b, _ := json.Marshal(types)
	return string(b)
}

func SQLCast(fieldExpr string, dataType string) string {
	dt := strings.ToLower(dataType)
	switch {
	case strings.Contains(dt, "number"):
		return fmt.Sprintf("(%s)::numeric", fieldExpr)
	case strings.Contains(dt, "timestamp"), strings.Contains(dt, "date"):
		return fmt.Sprintf("(%s)::timestamptz", fieldExpr)
	default:
		return fmt.Sprintf("LOWER(%s)", fieldExpr)
	}
}

func FieldExpr(field string) string {
	field = strings.Trim(field, "` ")
	return fmt.Sprintf("data->>'%s'", escapeSQLString(field))
}

func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
