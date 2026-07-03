package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ihamburglar/pdcarchive/internal/api"
	"github.com/ihamburglar/pdcarchive/internal/config"
	"github.com/ihamburglar/pdcarchive/internal/db"
	"github.com/ihamburglar/pdcarchive/internal/models"
	"github.com/ihamburglar/pdcarchive/internal/sync"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	switch os.Args[1] {
	case "serve":
		runServe(cfg)
	case "sync":
		runSync(cfg)
	default:
		printUsage()
		os.Exit(1)
	}
}

func runServe(cfg *config.Config) {
	database, err := db.Connect(cfg.DatabaseURL, cfg.Production)
	if err != nil {
		log.Fatalf("database: %v", err)
	}

	client := sync.NewClient(cfg.SourceBaseURL, cfg.SocrataAppToken)
	syncer := sync.NewSyncer(database, client)

	server, err := api.NewServer(cfg, database, syncer)
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	scheduler := sync.NewScheduler(syncer, cfg.Datasets, cfg.SyncInterval)
	scheduler.Start()

	addr := fmt.Sprintf("0.0.0.0:%s", cfg.Port)
	log.Printf("listening on %s", addr)
	if err := server.Router.Run(addr); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func runSync(cfg *config.Config) {
	database, err := db.Connect(cfg.DatabaseURL, cfg.Production)
	if err != nil {
		log.Fatalf("database: %v", err)
	}

	client := sync.NewClient(cfg.SourceBaseURL, cfg.SocrataAppToken)
	syncer := sync.NewSyncer(database, client)

	log.Printf("syncing %d datasets", len(cfg.Datasets))
	syncer.SyncAll(cfg.Datasets, models.SyncTriggerCLI)
	log.Printf("sync complete")
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <serve|sync>\n", os.Args[0])
}
