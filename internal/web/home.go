package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ihamburglar/pdcarchive/internal/config"
	"github.com/ihamburglar/pdcarchive/internal/datasets"
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
	var catalog []models.Dataset
	h.DB.Order("name").Find(&catalog)
	store := storage.NewStore(h.DB)

	existing := make(map[string]bool)
	for _, d := range catalog {
		existing[d.ID] = true
	}
	for _, reg := range datasets.All {
		if !existing[reg.ID] {
			catalog = append(catalog, models.Dataset{ID: reg.ID, Name: reg.Name})
		}
	}
	for i := range catalog {
		if count, err := store.CountDatasetRows(catalog[i].ID); err == nil {
			catalog[i].RowCount = count
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
		"Datasets": catalog,
		"BaseURL":  baseURL,
	})
}
