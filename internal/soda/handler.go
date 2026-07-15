package soda

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ihamburglar/pdcarchive/internal/models"
	"github.com/ihamburglar/pdcarchive/internal/sync"
	"gorm.io/gorm"
)

type Handler struct {
	DB *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{DB: db}
}

func (h *Handler) Resource(c *gin.Context) {
	rawID := c.Param("id")
	format := "json"
	id := rawID
	switch {
	case strings.HasSuffix(strings.ToLower(rawID), ".csv"):
		format = "csv"
		id = rawID[:len(rawID)-4]
	case strings.HasSuffix(strings.ToLower(rawID), ".json"):
		id = rawID[:len(rawID)-5]
	}

	var dataset models.Dataset
	if err := h.DB.First(&dataset, "id = ?", id).Error; err != nil {
		SocrataError(c.Writer, http.StatusNotFound, "dataset not found: "+id)
		return
	}

	params := ParseQueryParams(c.Request)
	params.Format = format
	if format == "csv" && params.Limit == defaultLimit {
		// allow larger default page for CSV via explicit $limit; keep requested limit
	}
	colTypes := BuildColumnTypesFromJSON(dataset.Columns)

	result, err := ExecuteQuery(h.DB, id, colTypes, params)
	if err != nil {
		SocrataError(c.Writer, http.StatusBadRequest, err.Error())
		return
	}

	if format == "csv" {
		h.writeCSV(c, &dataset, result)
		return
	}

	SetSodaHeaders(c.Writer, &dataset)
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Write([]byte("["))
	for i, row := range result.RowsJSON {
		if i > 0 {
			c.Writer.Write([]byte(","))
		}
		c.Writer.Write(row)
	}
	c.Writer.Write([]byte("]"))
}

func (h *Handler) writeCSV(c *gin.Context, dataset *models.Dataset, result *QueryResult) {
	header, records, err := BuildCSV(result, dataset.Columns)
	if err != nil {
		SocrataError(c.Writer, http.StatusInternalServerError, err.Error())
		return
	}

	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Content-Type", "text/csv;charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+dataset.ID+`.csv"`)
	if dataset.LastModified != nil {
		c.Header("Last-Modified", FormatLastModified(dataset.LastModified))
	}
	c.Status(http.StatusOK)

	w := csv.NewWriter(c.Writer)
	if len(header) > 0 {
		_ = w.Write(header)
	}
	for _, row := range records {
		_ = w.Write(row)
	}
	w.Flush()
}

func (h *Handler) Columns(c *gin.Context) {
	id := strings.TrimSuffix(c.Param("id"), ".json")
	var dataset models.Dataset
	if err := h.DB.First(&dataset, "id = ?", id).Error; err != nil {
		SocrataError(c.Writer, http.StatusNotFound, "dataset not found: "+id)
		return
	}

	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Content-Type", "application/json;charset=utf-8")
	c.Writer.Write(dataset.Columns)
}

func (h *Handler) Views(c *gin.Context) {
	id := strings.TrimSuffix(c.Param("id"), ".json")
	var dataset models.Dataset
	if err := h.DB.First(&dataset, "id = ?", id).Error; err != nil {
		SocrataError(c.Writer, http.StatusNotFound, "dataset not found: "+id)
		return
	}

	var columns []sync.ColumnMeta
	if len(dataset.Columns) > 0 {
		_ = json.Unmarshal(dataset.Columns, &columns)
	}

	view := gin.H{
		"id":         dataset.ID,
		"name":       dataset.Name,
		"columns":    columns,
		"rowCount":   dataset.RowCount,
		"rowsUpdatedAt": nil,
	}
	if dataset.LastModified != nil {
		view["rowsUpdatedAt"] = dataset.LastModified.UTC().Format("2006-01-02T15:04:05.000Z")
	} else if dataset.SyncedAt != nil {
		view["rowsUpdatedAt"] = dataset.SyncedAt.UTC().Format("2006-01-02T15:04:05.000Z")
	}

	c.Header("Access-Control-Allow-Origin", "*")
	c.JSON(http.StatusOK, view)
}
