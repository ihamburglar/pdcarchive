package soda

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ihamburglar/pdcarchive/internal/storage"
	"github.com/ihamburglar/pdcarchive/internal/sync"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	defaultLimit = 1000
	maxLimit     = 1000
)

type QueryParams struct {
	Select string
	Where  string
	Order  string
	Limit  int
	Offset int
}

type QueryResult struct {
	CountMode bool
	Count     int64
	Rows      []queryRecord
}

type queryRecord struct {
	ID    uint
	RowID string
	Data  datatypes.JSON
}

func ParseQueryParams(r *http.Request) QueryParams {
	q := r.URL.Query()
	limit := defaultLimit
	if raw := q.Get("$limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	if limit < 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if limit == 0 {
		limit = maxLimit
	}

	offset := 0
	if raw := q.Get("$offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			offset = n
		}
	}

	return QueryParams{
		Select: strings.TrimSpace(q.Get("$select")),
		Where:  strings.TrimSpace(q.Get("$where")),
		Order:  strings.TrimSpace(q.Get("$order")),
		Limit:  limit,
		Offset: offset,
	}
}

func ExecuteQuery(db *gorm.DB, datasetID string, colTypes ColumnTypes, params QueryParams) (*QueryResult, error) {
	whereSQL, err := ParseWhere(params.Where, colTypes)
	if err != nil {
		return nil, fmt.Errorf("invalid $where: %w", err)
	}
	store := storage.NewStore(db)
	table, exists, err := store.DatasetTableExists(datasetID)
	if err != nil {
		return nil, err
	}
	if !exists {
		if strings.ToLower(strings.ReplaceAll(params.Select, " ", "")) == "count(*)" {
			return &QueryResult{CountMode: true, Count: 0}, nil
		}
		return &QueryResult{Rows: []queryRecord{}}, nil
	}

	selectLower := strings.ToLower(strings.ReplaceAll(params.Select, " ", ""))
	if selectLower == "count(*)" {
		var count int64
		q := db.Table(table)
		if whereSQL != "" {
			q = q.Where(whereSQL)
		}
		if err := q.Count(&count).Error; err != nil {
			return nil, err
		}
		return &QueryResult{CountMode: true, Count: count}, nil
	}

	q := db.Table(table)
	if whereSQL != "" {
		q = q.Where(whereSQL)
	}

	orderSQL, err := parseOrder(params.Order, colTypes)
	if err != nil {
		return nil, fmt.Errorf("invalid $order: %w", err)
	}
	if orderSQL != "" {
		q = q.Order(orderSQL)
	} else {
		q = q.Order("id ASC")
	}

	q = q.Limit(params.Limit).Offset(params.Offset)

	var rows []queryRecord
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}

	return &QueryResult{Rows: rows}, nil
}

func parseOrder(order string, colTypes ColumnTypes) (string, error) {
	if order == "" {
		return "", nil
	}
	parts := strings.Fields(order)
	if len(parts) == 0 {
		return "", nil
	}
	field := strings.Trim(parts[0], "`")
	dir := "ASC"
	if len(parts) > 1 {
		d := strings.ToUpper(parts[1])
		if d == "DESC" {
			dir = "DESC"
		}
	}
	expr := FieldExpr(field)
	dt := colTypes[strings.ToLower(field)]
	if strings.Contains(strings.ToLower(dt), "number") {
		return fmt.Sprintf("(%s)::numeric %s", expr, dir), nil
	}
	if strings.Contains(strings.ToLower(dt), "timestamp") || strings.Contains(strings.ToLower(dt), "date") {
		return fmt.Sprintf("(%s)::timestamptz %s", expr, dir), nil
	}
	return fmt.Sprintf("LOWER(%s) %s", expr, dir), nil
}

func BuildColumnTypesFromJSON(columnsJSON []byte) ColumnTypes {
	var columns []sync.ColumnMeta
	if err := json.Unmarshal(columnsJSON, &columns); err != nil {
		return ColumnTypes{}
	}
	return BuildColumnTypes(columns)
}

func ProjectRows(rows []queryRecord, selectFields string) ([]json.RawMessage, error) {
	if selectFields == "" || selectFields == "*" {
		out := make([]json.RawMessage, len(rows))
		for i, r := range rows {
			out[i] = json.RawMessage(r.Data)
		}
		return out, nil
	}

	fields := strings.Split(selectFields, ",")
	out := make([]json.RawMessage, len(rows))
	for i, r := range rows {
		var data map[string]json.RawMessage
		if err := json.Unmarshal(r.Data, &data); err != nil {
			return nil, err
		}
		projected := make(map[string]json.RawMessage, len(fields))
		for _, f := range fields {
			f = strings.TrimSpace(strings.Trim(f, "`"))
			if v, ok := data[f]; ok {
				projected[f] = v
			}
		}
		b, err := json.Marshal(projected)
		if err != nil {
			return nil, err
		}
		out[i] = b
	}
	return out, nil
}
