package soda

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ihamburglar/pdcarchive/internal/models"
	"gorm.io/gorm"
)

type Handler struct {
	DB *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{DB: db}
}

func (h *Handler) Resource(c *gin.Context) {
	id := strings.TrimSuffix(c.Param("id"), ".json")

	var dataset models.Dataset
	if err := h.DB.First(&dataset, "id = ?", id).Error; err != nil {
		SocrataError(c.Writer, http.StatusNotFound, "dataset not found: "+id)
		return
	}

	params := ParseQueryParams(c.Request)
	colTypes := BuildColumnTypesFromJSON(dataset.Columns)

	result, err := ExecuteQuery(h.DB, id, colTypes, params)
	if err != nil {
		SocrataError(c.Writer, http.StatusBadRequest, err.Error())
		return
	}

	SetSodaHeaders(c.Writer, &dataset)

	if result.CountMode {
		c.JSON(http.StatusOK, []map[string]string{
			{"count": strconv.FormatInt(result.Count, 10)},
		})
		return
	}

	projected, err := ProjectRows(result.Rows, params.Select)
	if err != nil {
		SocrataError(c.Writer, http.StatusInternalServerError, err.Error())
		return
	}

	if projected == nil {
		projected = []json.RawMessage{}
	}

	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Write([]byte("["))
	for i, row := range projected {
		if i > 0 {
			c.Writer.Write([]byte(","))
		}
		c.Writer.Write(row)
	}
	c.Writer.Write([]byte("]"))
}

func (h *Handler) Columns(c *gin.Context) {
	id := c.Param("id")
	var dataset models.Dataset
	if err := h.DB.First(&dataset, "id = ?", id).Error; err != nil {
		SocrataError(c.Writer, http.StatusNotFound, "dataset not found: "+id)
		return
	}

	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Content-Type", "application/json;charset=utf-8")
	c.Writer.Write(dataset.Columns)
}
