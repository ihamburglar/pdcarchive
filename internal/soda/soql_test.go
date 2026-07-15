package soda

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseSelectListAggregates(t *testing.T) {
	all, items, err := ParseSelectList("count(*), sum(amount) AS total")
	if err != nil {
		t.Fatal(err)
	}
	if all {
		t.Fatal("expected not select all")
	}
	if len(items) != 2 {
		t.Fatalf("items = %d", len(items))
	}
	if items[1].Alias != "total" {
		t.Fatalf("alias = %q", items[1].Alias)
	}
}

func TestParseQueryFull(t *testing.T) {
	stmt, err := ParseQuery(`SELECT party, count(*) AS n WHERE amount > 100 GROUP BY party HAVING count(*) > 2 ORDER BY n DESC LIMIT 10 OFFSET 5`)
	if err != nil {
		t.Fatal(err)
	}
	if stmt.SelectAll || len(stmt.SelectItems) != 2 {
		t.Fatalf("select items = %d", len(stmt.SelectItems))
	}
	if stmt.Where == nil || len(stmt.GroupBy) != 1 || stmt.Having == nil {
		t.Fatal("missing clauses")
	}
	if !stmt.HasLimit || stmt.Limit != 10 || stmt.Offset != 5 {
		t.Fatalf("limit/offset = %v %d %d", stmt.HasLimit, stmt.Limit, stmt.Offset)
	}
	if len(stmt.OrderBy) != 1 || !stmt.OrderBy[0].Desc {
		t.Fatal("order by")
	}
}

func TestParseOrderMulti(t *testing.T) {
	items, err := ParseOrderList("amount DESC, filer_name ASC")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || !items[0].Desc || items[1].Desc {
		t.Fatalf("%+v", items)
	}
}

func TestCompileSelectWhereOrder(t *testing.T) {
	types := ColumnTypes{"amount": "number", "filer_name": "text"}
	stmt, err := BuildSelectStmt(QueryParams{
		Select: "filer_name, amount",
		Where:  "amount > 500",
		Order:  "amount DESC",
		Limit:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileSelect(stmt, "dataset_contrib", types)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compiled.SQL, "json_build_object") {
		t.Fatalf("sql = %s", compiled.SQL)
	}
	if !strings.Contains(compiled.SQL, "ORDER BY") {
		t.Fatalf("missing order: %s", compiled.SQL)
	}
	if !strings.Contains(compiled.SQL, "LIMIT 10") {
		t.Fatalf("missing limit: %s", compiled.SQL)
	}
	if len(compiled.Args) < 2 {
		t.Fatalf("expected bound args, got %#v", compiled.Args)
	}
}

func TestCompileGroupHaving(t *testing.T) {
	types := ColumnTypes{"amount": "number", "party": "text"}
	stmt, err := BuildSelectStmt(QueryParams{
		Select: "party, sum(amount) AS total",
		Group:  "party",
		Having: "sum(amount) > 1000",
		Limit:  5,
	})
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileSelect(stmt, "dataset_contrib", types)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compiled.SQL, "GROUP BY") {
		t.Fatalf("sql = %s", compiled.SQL)
	}
	if !strings.Contains(compiled.SQL, "HAVING") {
		t.Fatalf("sql = %s", compiled.SQL)
	}
}

func TestCompileQ(t *testing.T) {
	types := ColumnTypes{"filer_name": "text", "amount": "number"}
	stmt, err := BuildSelectStmt(QueryParams{Q: "smith", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileSelect(stmt, "dataset_contrib", types)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compiled.SQL, "LIKE") {
		t.Fatalf("expected LIKE for $q: %s", compiled.SQL)
	}
}

func TestCompileQueryParam(t *testing.T) {
	types := ColumnTypes{"amount": "number"}
	stmt, err := BuildSelectStmt(QueryParams{
		Query: "SELECT count(*) WHERE amount > 10",
		Limit: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileSelect(stmt, "dataset_contrib", types)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compiled.SQL, "count(*)") {
		t.Fatalf("sql = %s", compiled.SQL)
	}
}

func TestCompileStartsWith(t *testing.T) {
	expr, err := ParseExpr("starts_with(filer_name, 'Smi')")
	if err != nil {
		t.Fatal(err)
	}
	c := newCompiler(ColumnTypes{"filer_name": "text"})
	sql, err := c.compileExpr(expr, exprContext{clause: "where"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "LIKE") {
		t.Fatalf("sql = %s", sql)
	}
}

func TestCompileUnsupportedGeo(t *testing.T) {
	expr, err := ParseExpr("within_box(location, 1, 2, 3, 4)")
	if err != nil {
		t.Fatal(err)
	}
	c := newCompiler(ColumnTypes{})
	_, err = c.compileExpr(expr, exprContext{clause: "where"})
	if err == nil || !strings.Contains(err.Error(), "geospatial") {
		t.Fatalf("expected geospatial error, got %v", err)
	}
}

func TestBuildCSV(t *testing.T) {
	result := &QueryResult{
		OutputKeys: []string{"party", "total"},
		RowsJSON: []json.RawMessage{
			json.RawMessage(`{"party":"DEM","total":10}`),
		},
	}
	header, records, err := BuildCSV(result, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(header) != 2 || header[0] != "party" {
		t.Fatalf("header = %#v", header)
	}
	if len(records) != 1 || records[0][0] != "DEM" || records[0][1] != "10" {
		t.Fatalf("records = %#v", records)
	}
}
