package admin

import (
	"net/http"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/ihamburglar/pdcarchive/internal/config"
	"github.com/ihamburglar/pdcarchive/internal/models"
	"github.com/ihamburglar/pdcarchive/internal/sync"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const sessionKey = "admin_authenticated"

type Handler struct {
	DB      *gorm.DB
	Config  *config.Config
	Syncer  *sync.Syncer
}

func NewHandler(db *gorm.DB, cfg *config.Config, syncer *sync.Syncer) *Handler {
	return &Handler{DB: db, Config: cfg, Syncer: syncer}
}

func (h *Handler) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		if auth, ok := session.Get(sessionKey).(bool); !ok || !auth {
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
	var datasets []models.Dataset
	h.DB.Order("name").Find(&datasets)

	// Ensure configured datasets appear even if not yet synced
	existing := make(map[string]bool)
	for _, d := range datasets {
		existing[d.ID] = true
	}
	for _, id := range h.Config.Datasets {
		if !existing[id] {
			datasets = append(datasets, models.Dataset{ID: id, Name: id})
		}
	}

	var jobs []models.SyncJob
	h.DB.Order("id DESC").Limit(20).Find(&jobs)

	running := make(map[string]bool)
	for _, id := range h.Config.Datasets {
		running[id] = h.Syncer.IsRunning(id)
	}

	c.HTML(http.StatusOK, "admin.html", gin.H{
		"Datasets": datasets,
		"Jobs":     jobs,
		"Running":  running,
	})
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
	h.Syncer.SyncAllAsync(h.Config.Datasets, models.SyncTriggerManual)
	c.JSON(http.StatusAccepted, gin.H{"message": "sync started for all datasets"})
}

func (h *Handler) isConfigured(id string) bool {
	for _, d := range h.Config.Datasets {
		if d == id {
			return true
		}
	}
	return false
}

func checkPassword(password, expected string) bool {
	expected = strings.TrimSpace(expected)
	// Support bcrypt-hashed passwords in env ($2a$, $2b$, $2y$) or plain text for dev
	if len(expected) >= 4 && expected[0] == '$' && expected[1] == '2' && (expected[2] == 'a' || expected[2] == 'b' || expected[2] == 'y') && expected[3] == '$' {
		return bcrypt.CompareHashAndPassword([]byte(expected), []byte(password)) == nil
	}
	return password == expected
}
