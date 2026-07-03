package db

import (
	"fmt"
	"strings"

	"github.com/ihamburglar/pdcarchive/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(databaseURL string, production bool) (*gorm.DB, error) {
	dsn := databaseURL
	if !strings.Contains(dsn, "sslmode=") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		if production {
			dsn += sep + "sslmode=require"
		} else {
			dsn += sep + "sslmode=disable"
		}
	}

	logLevel := logger.Info
	if production {
		logLevel = logger.Warn
	}

	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	if err := database.AutoMigrate(
		&models.Dataset{},
		&models.SyncJob{},
	); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	sqlDB, err := database.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)

	return database, nil
}

func Ping(database *gorm.DB) error {
	sqlDB, err := database.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}
