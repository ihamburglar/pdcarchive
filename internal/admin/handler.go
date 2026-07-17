package admin

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/ihamburglar/pdcarchive/internal/config"
	"github.com/ihamburglar/pdcarchive/internal/datasets"
	"github.com/ihamburglar/pdcarchive/internal/models"
	"github.com/ihamburglar/pdcarchive/internal/storage"
	"github.com/ihamburglar/pdcarchive/internal/sync"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const sessionKey = "admin_authenticated"

type Handler struct {
	DB     *gorm.DB
	Config *config.Config
	Syncer *sync.Syncer
}

func NewHandler(db *gorm.DB, cfg *config.Config, syncer *sync.Syncer) *Handler {
	return &Handler{DB: db, Config: cfg, Syncer: syncer}
}

func (h *Handler) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		if auth, ok := session.Get(sessionKey).(bool); !ok || !auth {
			if strings.HasPrefix(c.Request.URL.Path, "/admin/api/") {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
				c.Abort()
				return
			}
			c.Redirect(http.StatusFound, "/admin/login")
			c.Abort()
			return
		}
		c.Next()
	}
}

func (h *Handler) LoginForm(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", gin.H{"Error": ""})
}

func (h *Handler) Login(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := c.PostForm("password")

	if username != h.Config.AdminUsername || !checkPassword(password, h.Config.AdminPassword) {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{"Error": "Invalid username or password"})
		return
	}

	session := sessions.Default(c)
	session.Set(sessionKey, true)
	if err := session.Save(); err != nil {
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{"Error": "Session error"})
		return
	}
	c.Redirect(http.StatusFound, "/admin")
}

func (h *Handler) Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	_ = session.Save()
	c.Redirect(http.StatusFound, "/admin/login")
}

func (h *Handler) Dashboard(c *gin.Context) {
	data := h.buildStatusData()
	c.HTML(http.StatusOK, "admin.html", data)
}

type datasetStatus struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	RowCount    int64   `json:"row_count"`
	SyncOffset  int64   `json:"sync_offset"`
	TableName   string  `json:"table_name"`
	TableExists bool    `json:"table_exists"`
	Incomplete  bool    `json:"incomplete"`
	SyncedAt    *string `json:"synced_at,omitempty"`
	Running     bool    `json:"running"`
}

type jobStatus struct {
	ID         uint    `json:"id"`
	DatasetID  string  `json:"dataset_id"`
	Status     string  `json:"status"`
	Trigger    string  `json:"trigger"`
	RowsSynced int64   `json:"rows_synced"`
	LastOffset int64   `json:"last_offset"`
	StartedAt  *string `json:"started_at,omitempty"`
	FinishedAt *string `json:"finished_at,omitempty"`
	Error      string  `json:"error,omitempty"`
}

func (h *Handler) StatusAPI(c *gin.Context) {
	c.JSON(http.StatusOK, h.buildStatusData())
}

func (h *Handler) buildStatusData() gin.H {
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

	dsOut := make([]datasetStatus, 0, len(catalog))
	for _, d := range catalog {
		tableName, tableExists, _ := store.DatasetTableExists(d.ID)
		rowCount, err := store.CountDatasetRows(d.ID)
		if err != nil {
			rowCount = d.RowCount
		}
		ds := datasetStatus{
			ID:          d.ID,
			Name:        d.Name,
			RowCount:    rowCount,
			SyncOffset:  d.SyncOffset,
			TableName:   tableName,
			TableExists: tableExists,
			Incomplete:  d.SyncOffset > rowCount,
			Running:     h.Syncer.IsRunning(d.ID),
		}
		if d.SyncedAt != nil {
			s := d.SyncedAt.Format(time.RFC3339)
			ds.SyncedAt = &s
		}
		dsOut = append(dsOut, ds)
	}

	var jobs []models.SyncJob
	h.DB.Order("id DESC").Limit(len(dsOut) * 2).Find(&jobs)

	jobsOut := make([]jobStatus, 0, len(jobs))
	for _, j := range jobs {
		js := jobStatus{
			ID:         j.ID,
			DatasetID:  j.DatasetID,
			Status:     j.Status,
			Trigger:    j.Trigger,
			RowsSynced: j.RowsSynced,
			LastOffset: j.LastOffset,
			Error:      j.Error,
		}
		if j.StartedAt != nil {
			s := j.StartedAt.Format(time.RFC3339)
			js.StartedAt = &s
		}
		if j.FinishedAt != nil {
			s := j.FinishedAt.Format(time.RFC3339)
			js.FinishedAt = &s
		}
		jobsOut = append(jobsOut, js)
	}

	return gin.H{
		"Datasets": dsOut,
		"Jobs":     jobsOut,
	}
}

func (h *Handler) SyncDataset(c *gin.Context) {
	id := c.Param("id")
	if !h.isConfigured(id) {
		c.JSON(http.StatusNotFound, gin.H{"error": "dataset not configured"})
		return
	}
	if !h.Syncer.SyncDatasetAsync(id, models.SyncTriggerManual) {
		c.JSON(http.StatusConflict, gin.H{"error": "sync already running"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "sync started", "dataset_id": id})
}

func (h *Handler) SyncAll(c *gin.Context) {
	if h.Syncer.AnyRunning() {
		c.JSON(http.StatusConflict, gin.H{"error": sync.ErrImportInProgress.Error()})
		return
	}
	h.Syncer.SyncAllAsync(datasets.IDs(), models.SyncTriggerManual)
	c.JSON(http.StatusAccepted, gin.H{"message": "sync started for all datasets"})
}

func (h *Handler) StopDataset(c *gin.Context) {
	id := c.Param("id")
	if !h.isConfigured(id) {
		c.JSON(http.StatusNotFound, gin.H{"error": "dataset not configured"})
		return
	}
	if !h.Syncer.StopSync(id) {
		c.JSON(http.StatusConflict, gin.H{"error": "sync not running"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "sync stop requested", "dataset_id": id})
}

func (h *Handler) ClearDataset(c *gin.Context) {
	id := c.Param("id")
	if !h.isConfigured(id) {
		c.JSON(http.StatusNotFound, gin.H{"error": "dataset not configured"})
		return
	}
	deleted, err := h.Syncer.ClearDataset(id)
	if err != nil {
		if errors.Is(err, sync.ErrSyncRunning) || errors.Is(err, sync.ErrImportInProgress) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "records deleted", "dataset_id": id, "deleted": deleted})
}

func (h *Handler) writeSyncerError(c *gin.Context, err error) {
	if errors.Is(err, sync.ErrSyncRunning) || errors.Is(err, sync.ErrImportInProgress) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func (h *Handler) isConfigured(id string) bool {
	_, ok := datasets.ByID(id)
	return ok
}

func checkPassword(password, expected string) bool {
	expected = strings.TrimSpace(expected)
	if len(expected) >= 4 && expected[0] == '$' && expected[1] == '2' && (expected[2] == 'a' || expected[2] == 'b' || expected[2] == 'y') && expected[3] == '$' {
		return bcrypt.CompareHashAndPassword([]byte(expected), []byte(password)) == nil
	}
	return password == expected
}
