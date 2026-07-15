package soda

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ihamburglar/pdcarchive/internal/storage"
	"github.com/ihamburglar/pdcarchive/internal/sync"
	"gorm.io/gorm"
)

const (
	defaultLimit = 1000
	maxLimit     = 50000
	maxJSONLimit = 1000
)

type QueryParams struct {
	Select   string
	Where    string
	Order    string
	Group    string
	Having   string
	Q        string
	Query    string
	Distinct bool
	Limit    int
	Offset   int
	Format   string // "json" or "csv"
}

type QueryResult struct {
	SelectAll  bool
	OutputKeys []string
	RowsJSON   []json.RawMessage // projected or full data objects
	CSVHeader  []string
	CSVRows    [][]string
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
		limit = maxJSONLimit
	}

	offset := 0
	if raw := q.Get("$offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			offset = n
		}
	}

	distinct := false
	if raw := q.Get("$distinct"); raw != "" {
		distinct = raw == "true" || raw == "TRUE" || raw == "1"
	}

	return QueryParams{
		Select:   strings.TrimSpace(q.Get("$select")),
		Where:    strings.TrimSpace(q.Get("$where")),
		Order:    strings.TrimSpace(q.Get("$order")),
		Group:    strings.TrimSpace(q.Get("$group")),
		Having:   strings.TrimSpace(q.Get("$having")),
		Q:        strings.TrimSpace(q.Get("$q")),
		Query:    strings.TrimSpace(q.Get("$query")),
		Distinct: distinct,
		Limit:    limit,
		Offset:   offset,
	}
}

// BuildSelectStmt merges URL params (or $query) into a SelectStmt.
func BuildSelectStmt(params QueryParams) (*SelectStmt, error) {
	var stmt *SelectStmt
	var err error

	if params.Query != "" {
		stmt, err = ParseQuery(params.Query)
		if err != nil {
			return nil, fmt.Errorf("invalid $query: %w", err)
		}
		// $q still applies alongside $query
		stmt.Q = params.Q
		if !stmt.HasLimit {
			stmt.Limit = params.Limit
			stmt.HasLimit = true
		}
		if stmt.Offset == 0 && params.Offset > 0 {
			stmt.Offset = params.Offset
		}
		if params.Distinct {
			stmt.Distinct = true
		}
		return stmt, nil
	}

	stmt = &SelectStmt{
		Distinct: params.Distinct,
		Q:        params.Q,
		Limit:    params.Limit,
		HasLimit: true,
		Offset:   params.Offset,
	}

	all, items, err := ParseSelectList(params.Select)
	if err != nil {
		return nil, fmt.Errorf("invalid $select: %w", err)
	}
	stmt.SelectAll = all
	stmt.SelectItems = items

	if params.Where != "" {
		expr, err := ParseExpr(params.Where)
		if err != nil {
			return nil, fmt.Errorf("invalid $where: %w", err)
		}
		stmt.Where = expr
	}
	if params.Group != "" {
		groups, err := ParseGroupList(params.Group)
		if err != nil {
			return nil, fmt.Errorf("invalid $group: %w", err)
		}
		stmt.GroupBy = groups
	}
	if params.Having != "" {
		expr, err := ParseExpr(params.Having)
		if err != nil {
			return nil, fmt.Errorf("invalid $having: %w", err)
		}
		stmt.Having = expr
	}
	if params.Order != "" {
		orders, err := ParseOrderList(params.Order)
		if err != nil {
			return nil, fmt.Errorf("invalid $order: %w", err)
		}
		stmt.OrderBy = orders
	}

	return stmt, nil
}

func ExecuteQuery(db *gorm.DB, datasetID string, colTypes ColumnTypes, params QueryParams) (*QueryResult, error) {
	stmt, err := BuildSelectStmt(params)
	if err != nil {
		return nil, err
	}

	// Cap JSON default-style limits unless CSV (handler may raise before call)
	if params.Format != "csv" && stmt.Limit > maxJSONLimit {
		stmt.Limit = maxJSONLimit
	}

	store := storage.NewStore(db)
	table, exists, err := store.DatasetTableExists(datasetID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return emptyResult(stmt), nil
	}

	compiled, err := CompileSelect(stmt, table, colTypes)
	if err != nil {
		return nil, err
	}

	rows, err := db.Raw(compiled.SQL, compiled.Args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := &QueryResult{
		SelectAll:  compiled.SelectAll,
		OutputKeys: compiled.OutputKeys,
	}

	if compiled.SelectAll {
		for rows.Next() {
			var data []byte
			if err := rows.Scan(&data); err != nil {
				return nil, err
			}
			result.RowsJSON = append(result.RowsJSON, json.RawMessage(data))
		}
	} else {
		for rows.Next() {
			var data []byte
			if err := rows.Scan(&data); err != nil {
				return nil, err
			}
			result.RowsJSON = append(result.RowsJSON, json.RawMessage(data))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result.RowsJSON == nil {
		result.RowsJSON = []json.RawMessage{}
	}

	// Normalize sole count(*) to string value for SODA compatibility
	if isSoleCountStar(stmt) {
		result.RowsJSON = normalizeCountRows(result.RowsJSON)
	}

	return result, nil
}

func emptyResult(stmt *SelectStmt) *QueryResult {
	if isSoleCountStar(stmt) {
		return &QueryResult{
			RowsJSON: []json.RawMessage{json.RawMessage(`{"count":"0"}`)},
		}
	}
	return &QueryResult{RowsJSON: []json.RawMessage{}, SelectAll: stmt.SelectAll}
}

func isSoleCountStar(stmt *SelectStmt) bool {
	if stmt == nil || len(stmt.SelectItems) != 1 || len(stmt.GroupBy) > 0 {
		return false
	}
	fn, ok := stmt.SelectItems[0].Expr.(*FuncExpr)
	if !ok || normalizeIdent(fn.Name) != "count" {
		return false
	}
	return len(fn.Args) == 0 || (len(fn.Args) == 1 && isStar(fn.Args[0]))
}

func normalizeCountRows(rows []json.RawMessage) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(rows))
	for _, raw := range rows {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			out = append(out, raw)
			continue
		}
		if v, ok := m["count"]; ok {
			var num json.Number
			if err := json.Unmarshal(v, &num); err == nil {
				m["count"] = json.RawMessage(strconv.Quote(num.String()))
				b, _ := json.Marshal(m)
				out = append(out, b)
				continue
			}
		}
		out = append(out, raw)
	}
	return out
}

func BuildColumnTypesFromJSON(columnsJSON []byte) ColumnTypes {
	var columns []sync.ColumnMeta
	if err := json.Unmarshal(columnsJSON, &columns); err != nil {
		return ColumnTypes{}
	}
	return BuildColumnTypes(columns)
}

// BuildCSV converts query JSON rows into CSV header + records.
func BuildCSV(result *QueryResult, columnsJSON []byte) (header []string, records [][]string, err error) {
	if result.SelectAll {
		var columns []sync.ColumnMeta
		_ = json.Unmarshal(columnsJSON, &columns)
		header = SodaFields(columns)
		if len(header) == 0 && len(result.RowsJSON) > 0 {
			var first map[string]json.RawMessage
			if err := json.Unmarshal(result.RowsJSON[0], &first); err == nil {
				for k := range first {
					header = append(header, k)
				}
			}
		}
	} else if len(result.OutputKeys) > 0 {
		header = result.OutputKeys
	}

	records = make([][]string, 0, len(result.RowsJSON))
	for _, raw := range result.RowsJSON {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, nil, err
		}
		row := make([]string, len(header))
		for i, key := range header {
			if v, ok := m[key]; ok {
				row[i] = jsonValueToCSV(v)
			}
		}
		records = append(records, row)
	}
	return header, records, nil
}

func jsonValueToCSV(v json.RawMessage) string {
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(v, &n); err == nil {
		return n.String()
	}
	var b bool
	if err := json.Unmarshal(v, &b); err == nil {
		if b {
			return "true"
		}
		return "false"
	}
	if string(v) == "null" {
		return ""
	}
	return string(v)
}
