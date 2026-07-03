package storage

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/ihamburglar/pdcarchive/internal/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInvalidDatasetID = errors.New("invalid dataset id")

type DatasetRecord struct {
	ID    uint           `gorm:"primaryKey"`
	RowID string         `gorm:"not null"`
	Data  datatypes.JSON `gorm:"type:jsonb;not null"`
}

type Store struct {
	db *gorm.DB
}

var fixedDatasetTables = map[string]string{
	"kv7h-kjye": "dataset_contributions",
	"tijg-9zyp": "dataset_expenditures",
	"7qr9-q2c9": "dataset_reporting_history",
	"3h9x-7bvm": "dataset_summary",
	"3r6b-hsaa": "dataset_debt",
	"d2ig-r3q4": "dataset_loans",
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func DatasetTableName(datasetID string) (string, error) {
	if table, ok := fixedDatasetTables[datasetID]; ok {
		return table, nil
	}
	slug, err := datasetIDSlug(datasetID)
	if err != nil {
		return "", err
	}
	return "dataset_" + slug, nil
}

func datasetIDSlug(datasetID string) (string, error) {
	if datasetID == "" {
		return "", ErrInvalidDatasetID
	}
	var b strings.Builder
	for _, r := range datasetID {
		switch {
		case r == '-' || r == '_':
			b.WriteRune('_')
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		default:
			return "", ErrInvalidDatasetID
		}
	}
	return b.String(), nil
}

func (s *Store) DatasetTableName(datasetID string) (string, error) {
	return DatasetTableName(datasetID)
}

func (s *Store) EnsureDatasetTable(datasetID string) (string, error) {
	table, err := DatasetTableName(datasetID)
	if err != nil {
		return "", err
	}
	if err := s.db.Table(table).AutoMigrate(&DatasetRecord{}); err != nil {
		return "", err
	}
	indexName := "idx_" + table + "_row_id"
	if err := s.db.Exec(fmt.Sprintf(
		`CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s ("row_id")`,
		quoteIdentifier(indexName),
		quoteIdentifier(table),
	)).Error; err != nil {
		return "", err
	}
	return table, nil
}

func (s *Store) DatasetTableExists(datasetID string) (string, bool, error) {
	table, err := DatasetTableName(datasetID)
	if err != nil {
		return "", false, err
	}
	return table, s.db.Migrator().HasTable(table), nil
}

func (s *Store) UpsertRecords(datasetID string, records []DatasetRecord) error {
	if len(records) == 0 {
		_, err := s.EnsureDatasetTable(datasetID)
		return err
	}
	table, err := s.EnsureDatasetTable(datasetID)
	if err != nil {
		return err
	}
	return s.db.Table(table).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "row_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"data"}),
	}).CreateInBatches(&records, 500).Error
}

func (s *Store) CountDatasetRows(datasetID string) (int64, error) {
	table, exists, err := s.DatasetTableExists(datasetID)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	var count int64
	if err := s.db.Table(table).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) UpsertDatasetOffset(datasetID, name string, offset int64) error {
	if name == "" {
		name = datasetID
	}
	ds := models.Dataset{
		ID:         datasetID,
		Name:       name,
		SyncOffset: offset,
		Columns:    datatypes.JSON("[]"),
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "sync_offset"}),
	}).Create(&ds).Error
}

func (s *Store) UpdateDatasetStats(datasetID, name string, offset, rowCount int64) error {
	if name == "" {
		name = datasetID
	}
	ds := models.Dataset{
		ID:       datasetID,
		Name:     name,
		RowCount: rowCount,
		Columns:  datatypes.JSON("[]"),
	}
	updateColumns := []string{"name", "row_count"}
	if offset >= 0 {
		ds.SyncOffset = offset
		updateColumns = append(updateColumns, "sync_offset")
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns(updateColumns),
	}).Create(&ds).Error
}

func (s *Store) ClearDataset(datasetID string) (int64, error) {
	table, exists, err := s.DatasetTableExists(datasetID)
	if err != nil {
		return 0, err
	}
	var deleted int64
	if exists {
		result := s.db.Table(table).Where("1 = 1").Delete(&DatasetRecord{})
		if result.Error != nil {
			return 0, result.Error
		}
		deleted += result.RowsAffected
	}

	if err := s.db.Model(&models.Dataset{}).Where("id = ?", datasetID).Updates(map[string]interface{}{
		"row_count":   0,
		"sync_offset": 0,
		"synced_at":   nil,
	}).Error; err != nil {
		return 0, err
	}
	return deleted, nil
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
