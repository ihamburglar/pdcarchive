package main

import (
	"fmt"
	"log"

	"github.com/ihamburglar/pdcarchive/internal/api"
	"github.com/ihamburglar/pdcarchive/internal/config"
	"github.com/ihamburglar/pdcarchive/internal/db"
	"github.com/ihamburglar/pdcarchive/internal/sync"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	database, err := db.Connect(cfg.DatabaseURL, cfg.Production)
	if err != nil {
		log.Fatalf("database: %v", err)
	}

	client := sync.NewClient(cfg.SourceBaseURL, cfg.SocrataAppToken)
	syncer := sync.NewSyncer(database, client, cfg.SyncPageSize, cfg.SyncPageInterval)

	server, err := api.NewServer(cfg, database, syncer)
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	scheduler := sync.NewScheduler(syncer, cfg.Datasets, cfg.SyncInterval)
	scheduler.Start()

	addr := fmt.Sprintf("0.0.0.0:%s", cfg.Port)
	log.Printf("listening on %s", addr)
	if err := server.Router.Run(addr); err != nil {
		log.Fatalf("server: %v", err)
	}
}
