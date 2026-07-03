package models

import "gorm.io/datatypes"

type Record struct {
	ID        uint           `gorm:"primaryKey"`
	DatasetID string         `gorm:"size:16;not null;uniqueIndex:idx_dataset_row"`
	RowID     string         `gorm:"not null;uniqueIndex:idx_dataset_row"`
	Data      datatypes.JSON `gorm:"type:jsonb;not null"`
}
