package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ihamburglar/pdcarchive/internal/config"
	"github.com/ihamburglar/pdcarchive/internal/models"
	"github.com/ihamburglar/pdcarchive/internal/storage"
	"gorm.io/gorm"
)

type Handler struct {
	DB     *gorm.DB
	Config *config.Config
}

func NewHandler(db *gorm.DB, cfg *config.Config) *Handler {
	return &Handler{DB: db, Config: cfg}
}

func (h *Handler) Home(c *gin.Context) {
	var datasets []models.Dataset
	h.DB.Order("name").Find(&datasets)
	store := storage.NewStore(h.DB)

	existing := make(map[string]bool)
	for _, d := range datasets {
		existing[d.ID] = true
	}
	for _, id := range h.Config.Datasets {
		if !existing[id] {
			datasets = append(datasets, models.Dataset{ID: id, Name: id})
		}
	}
	for i := range datasets {
		if count, err := store.CountDatasetRows(datasets[i].ID); err == nil {
			datasets[i].RowCount = count
		}
	}

	host := c.Request.Host
	if host == "" {
		host = "localhost:" + h.Config.Port
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	baseURL := scheme + "://" + host

	c.HTML(http.StatusOK, "home.html", gin.H{
		"Datasets": datasets,
		"BaseURL":  baseURL,
	})
}
