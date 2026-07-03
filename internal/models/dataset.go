package models

import (
	"time"

	"gorm.io/datatypes"
)

type Dataset struct {
	ID           string         `gorm:"primaryKey;size:16"`
	Name         string         `gorm:"not null"`
	Columns      datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	LastModified *time.Time
	SyncedAt     *time.Time
	RowCount     int64 `gorm:"default:0"`
	SyncOffset   int64 `gorm:"default:0"`
}
