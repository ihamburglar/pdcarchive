package api

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/ihamburglar/pdcarchive/internal/admin"
	"github.com/ihamburglar/pdcarchive/internal/config"
	"github.com/ihamburglar/pdcarchive/internal/db"
	"github.com/ihamburglar/pdcarchive/internal/soda"
	"github.com/ihamburglar/pdcarchive/internal/sync"
	"github.com/ihamburglar/pdcarchive/internal/web"
	"gorm.io/gorm"
)

type Server struct {
	Router *gin.Engine
	DB     *gorm.DB
	Config *config.Config
	Syncer *sync.Syncer
}

func NewServer(cfg *config.Config, database *gorm.DB, syncer *sync.Syncer) (*Server, error) {
	if cfg.Production {
		gin.SetMode(gin.ReleaseMode)
	}

	tmpl, err := web.LoadTemplates()
	if err != nil {
		return nil, err
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.SetHTMLTemplate(tmpl)

	store := cookie.NewStore([]byte(cfg.SessionSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		Secure:   cfg.Production,
		SameSite: http.SameSiteLaxMode,
	})
	r.Use(sessions.Sessions("pdcarchive_session", store))

	webHandler := web.NewHandler(database, cfg)
	sodaHandler := soda.NewHandler(database)
	adminHandler := admin.NewHandler(database, cfg, syncer)

	r.GET("/health", func(c *gin.Context) {
		if err := db.Ping(database); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/", webHandler.Home)

	r.GET("/resource/:id", sodaHandler.Resource)
	r.GET("/api/views/:id/columns.json", sodaHandler.Columns)

	adminRoutes := r.Group("/admin")
	{
		adminRoutes.GET("/login", adminHandler.LoginForm)
		adminRoutes.POST("/login", adminHandler.Login)

		protected := adminRoutes.Group("")
		protected.Use(adminHandler.RequireAuth())
		{
			protected.GET("", adminHandler.Dashboard)
			protected.GET("/api/status", adminHandler.StatusAPI)
			protected.POST("/logout", adminHandler.Logout)
			protected.POST("/sync", adminHandler.SyncAll)
			protected.POST("/datasets/migrate", adminHandler.MigrateAll)
			protected.POST("/datasets/reconcile", adminHandler.ReconcileAll)
			protected.POST("/datasets/:id/sync", adminHandler.SyncDataset)
			protected.POST("/datasets/:id/stop", adminHandler.StopDataset)
			protected.POST("/datasets/:id/clear", adminHandler.ClearDataset)
			protected.POST("/datasets/:id/migrate", adminHandler.MigrateDataset)
			protected.POST("/datasets/:id/reconcile", adminHandler.ReconcileDataset)
		}
	}

	return &Server{
		Router: r,
		DB:     database,
		Config: cfg,
		Syncer: syncer,
	}, nil
}
