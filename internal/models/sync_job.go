package models

import "time"

const (
	SyncStatusPending   = "pending"
	SyncStatusRunning   = "running"
	SyncStatusCompleted = "completed"
	SyncStatusFailed    = "failed"
	SyncStatusCancelled = "cancelled"

	SyncTriggerScheduled = "scheduled"
	SyncTriggerManual    = "manual"
	SyncTriggerCLI       = "cli"
)

type SyncJob struct {
	ID         uint       `gorm:"primaryKey"`
	DatasetID  string     `gorm:"size:16;not null;index"`
	Status     string     `gorm:"not null"`
	StartedAt  *time.Time
	FinishedAt *time.Time
	RowsSynced int64 `gorm:"default:0"`
	LastOffset int64 `gorm:"default:0"`
	Error      string
	Trigger    string `gorm:"not null"`
}
