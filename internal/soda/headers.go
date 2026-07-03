package soda

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ihamburglar/pdcarchive/internal/models"
	"github.com/ihamburglar/pdcarchive/internal/sync"
)

func SetSodaHeaders(w http.ResponseWriter, dataset *models.Dataset) {
	var columns []sync.ColumnMeta
	if len(dataset.Columns) > 0 {
		_ = json.Unmarshal(dataset.Columns, &columns)
	}

	fields := SodaFields(columns)
	types := SodaTypes(columns)

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-SODA2-Fields", FieldsJSON(fields))
	w.Header().Set("X-SODA2-Types", TypesJSON(types))
	if dataset.LastModified != nil {
		w.Header().Set("X-SODA2-Truth-Last-Modified", FormatLastModified(dataset.LastModified))
		w.Header().Set("Last-Modified", FormatLastModified(dataset.LastModified))
	}
}

func SocrataError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    fmt.Sprintf("%d", code),
		"error":   true,
		"message": message,
	})
}
