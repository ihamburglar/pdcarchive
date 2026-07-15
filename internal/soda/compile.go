package soda

import (
	"fmt"
	"strings"
)

type compiledSQL struct {
	SQL        string
	Args       []interface{}
	OutputKeys []string // JSON object keys for projected/aggregate rows; empty means raw data jsonb
	SelectAll  bool
	Distinct   bool
}

type compiler struct {
	colTypes ColumnTypes
	args     []interface{}
	dialect  string // "postgres" (default)
	aliases  map[string]string // select alias -> already-compiled SQL
}

func newCompiler(colTypes ColumnTypes) *compiler {
	if colTypes == nil {
		colTypes = ColumnTypes{}
	}
	return &compiler{colTypes: colTypes, dialect: "postgres", aliases: map[string]string{}}
}

func (c *compiler) addArg(v interface{}) string {
	c.args = append(c.args, v)
	return fmt.Sprintf("$%d", len(c.args))
}

// CompileSelect compiles a SelectStmt against tableName into SQL.
func CompileSelect(stmt *SelectStmt, tableName string, colTypes ColumnTypes) (*compiledSQL, error) {
	c := newCompiler(colTypes)
	return c.compileSelect(stmt, tableName)
}

func (c *compiler) compileSelect(stmt *SelectStmt, tableName string) (*compiledSQL, error) {
	out := &compiledSQL{SelectAll: stmt.SelectAll, Distinct: stmt.Distinct}

	var selectSQL string
	var err error
	if stmt.SelectAll || (len(stmt.SelectItems) == 0 && !stmt.HasAggregates()) {
		selectSQL = "data"
		out.SelectAll = true
		out.OutputKeys = nil
	} else {
		selectSQL, out.OutputKeys, err = c.compileSelectList(stmt.SelectItems)
		if err != nil {
			return nil, err
		}
	}

	distinct := ""
	if stmt.Distinct {
		distinct = "DISTINCT "
	}

	var b strings.Builder
	fmt.Fprintf(&b, "SELECT %s%s FROM %s", distinct, selectSQL, quoteIdent(tableName))

	whereParts := make([]string, 0, 2)
	if stmt.Where != nil {
		w, err := c.compileExpr(stmt.Where, exprContext{clause: "where"})
		if err != nil {
			return nil, fmt.Errorf("$where: %w", err)
		}
		whereParts = append(whereParts, w)
	}
	if stmt.Q != "" {
		qSQL, err := c.compileQ(stmt.Q)
		if err != nil {
			return nil, err
		}
		if qSQL != "" {
			whereParts = append(whereParts, qSQL)
		}
	}
	if len(whereParts) > 0 {
		b.WriteString(" WHERE ")
		b.WriteString(strings.Join(whereParts, " AND "))
	}

	if len(stmt.GroupBy) > 0 {
		b.WriteString(" GROUP BY ")
		parts := make([]string, 0, len(stmt.GroupBy))
		for _, g := range stmt.GroupBy {
			s, err := c.compileExpr(g, exprContext{clause: "group"})
			if err != nil {
				return nil, fmt.Errorf("$group: %w", err)
			}
			parts = append(parts, s)
		}
		b.WriteString(strings.Join(parts, ", "))
	}

	if stmt.Having != nil {
		h, err := c.compileExpr(stmt.Having, exprContext{clause: "having"})
		if err != nil {
			return nil, fmt.Errorf("$having: %w", err)
		}
		b.WriteString(" HAVING ")
		b.WriteString(h)
	}

	if len(stmt.OrderBy) > 0 {
		b.WriteString(" ORDER BY ")
		parts := make([]string, 0, len(stmt.OrderBy))
		for _, o := range stmt.OrderBy {
			s, err := c.compileExpr(o.Expr, exprContext{clause: "order"})
			if err != nil {
				return nil, fmt.Errorf("$order: %w", err)
			}
			dir := "ASC"
			if o.Desc {
				dir = "DESC"
			}
			parts = append(parts, s+" "+dir)
		}
		b.WriteString(strings.Join(parts, ", "))
	} else if out.SelectAll {
		b.WriteString(" ORDER BY id ASC")
	}

	if stmt.HasLimit {
		fmt.Fprintf(&b, " LIMIT %d", stmt.Limit)
	}
	if stmt.Offset > 0 {
		fmt.Fprintf(&b, " OFFSET %d", stmt.Offset)
	}

	out.SQL = b.String()
	out.Args = c.args
	return out, nil
}

func (c *compiler) compileSelectList(items []SelectItem) (string, []string, error) {
	if len(items) == 0 {
		return "", nil, fmt.Errorf("empty $select")
	}
	keys := make([]string, 0, len(items))
	parts := make([]string, 0, len(items)*2)
	for i, item := range items {
		if _, ok := item.Expr.(*StarExpr); ok {
			return "", nil, fmt.Errorf("cannot mix * with other select items")
		}
		alias := item.Alias
		if alias == "" {
			alias = defaultAlias(item.Expr, i)
		}
		sql, err := c.compileExpr(item.Expr, exprContext{clause: "select"})
		if err != nil {
			return "", nil, err
		}
		keys = append(keys, alias)
		c.aliases[strings.ToLower(alias)] = sql
		parts = append(parts, c.addArg(alias), sql)
	}
	var b strings.Builder
	b.WriteString("json_build_object(")
	for i := 0; i < len(parts); i += 2 {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s, %s", parts[i], parts[i+1])
	}
	b.WriteString(")")
	return b.String(), keys, nil
}

func defaultAlias(e Expr, idx int) string {
	switch n := e.(type) {
	case *IdentExpr:
		return n.Name
	case *FuncExpr:
		name := normalizeIdent(n.Name)
		if len(n.Args) == 1 {
			if id, ok := n.Args[0].(*IdentExpr); ok {
				return name + "_" + id.Name
			}
			if _, ok := n.Args[0].(*StarExpr); ok && name == "count" {
				return "count"
			}
		}
		return name
	default:
		return fmt.Sprintf("column_%d", idx+1)
	}
}

type exprContext struct {
	clause string
}

func (c *compiler) compileExpr(e Expr, ctx exprContext) (string, error) {
	switch n := e.(type) {
	case *IdentExpr:
		return c.compileIdent(n.Name, ctx)
	case *StarExpr:
		return "*", nil
	case *LiteralExpr:
		return c.compileLiteral(n)
	case *UnaryExpr:
		x, err := c.compileExpr(n.X, ctx)
		if err != nil {
			return "", err
		}
		switch n.Op {
		case "NOT":
			return fmt.Sprintf("(NOT %s)", x), nil
		case "+", "-":
			return fmt.Sprintf("(%s%s)", n.Op, x), nil
		default:
			return "", fmt.Errorf("unsupported unary operator %s", n.Op)
		}
	case *BinaryExpr:
		return c.compileBinary(n, ctx)
	case *BetweenExpr:
		x, err := c.compileExpr(n.X, ctx)
		if err != nil {
			return "", err
		}
		low, err := c.compileExpr(n.Low, ctx)
		if err != nil {
			return "", err
		}
		high, err := c.compileExpr(n.High, ctx)
		if err != nil {
			return "", err
		}
		if n.Not {
			return fmt.Sprintf("(%s NOT BETWEEN %s AND %s)", x, low, high), nil
		}
		return fmt.Sprintf("(%s BETWEEN %s AND %s)", x, low, high), nil
	case *InExpr:
		x, err := c.compileExpr(n.X, ctx)
		if err != nil {
			return "", err
		}
		parts := make([]string, 0, len(n.Values))
		for _, v := range n.Values {
			s, err := c.compileExpr(v, ctx)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		op := "IN"
		if n.Not {
			op = "NOT IN"
		}
		return fmt.Sprintf("(%s %s (%s))", x, op, strings.Join(parts, ", ")), nil
	case *IsNullExpr:
		x, err := c.compileExpr(n.X, ctx)
		if err != nil {
			return "", err
		}
		// For JSON text extraction, NULL means SQL NULL or JSON null → both as IS NULL on ->>
		if n.Not {
			return fmt.Sprintf("(%s IS NOT NULL)", x), nil
		}
		return fmt.Sprintf("(%s IS NULL)", x), nil
	case *FuncExpr:
		return c.compileFunc(n, ctx)
	case *CaseExpr:
		return c.compileCase(n, ctx)
	default:
		return "", fmt.Errorf("unsupported expression")
	}
}

func (c *compiler) compileIdent(name string, ctx exprContext) (string, error) {
	name = strings.Trim(name, "` ")
	if (ctx.clause == "order" || ctx.clause == "having") && c.aliases != nil {
		if sql, ok := c.aliases[strings.ToLower(name)]; ok {
			return sql, nil
		}
	}
	raw := FieldExpr(name)
	dt := c.colTypes[strings.ToLower(name)]
	// In SELECT/GROUP/ORDER use typed cast without LOWER so values stay useful.
	// Comparisons apply LOWER for text in compileBinary.
	if ctx.clause == "where" || ctx.clause == "having" {
		return raw, nil
	}
	if isNumericType(dt) {
		return fmt.Sprintf("(%s)::numeric", raw), nil
	}
	if isTimestampType(dt) {
		return fmt.Sprintf("(%s)::timestamptz", raw), nil
	}
	return raw, nil
}

func (c *compiler) compileLiteral(n *LiteralExpr) (string, error) {
	switch n.Kind {
	case "string":
		return c.addArg(n.Value), nil
	case "number":
		return c.addArg(n.Value), nil // pass as string; compare with ::numeric casts as needed
	case "null":
		return "NULL", nil
	case "bool":
		if n.Value == "true" {
			return "TRUE", nil
		}
		return "FALSE", nil
	default:
		return "", fmt.Errorf("unknown literal kind %s", n.Kind)
	}
}

func (c *compiler) compileBinary(n *BinaryExpr, ctx exprContext) (string, error) {
	switch n.Op {
	case "AND", "OR":
		left, err := c.compileExpr(n.Left, ctx)
		if err != nil {
			return "", err
		}
		right, err := c.compileExpr(n.Right, ctx)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(%s %s %s)", left, n.Op, right), nil
	case "LIKE", "NOT LIKE":
		left, err := c.compileText(n.Left, ctx)
		if err != nil {
			return "", err
		}
		right, err := c.compileExpr(n.Right, ctx)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(%s %s %s)", left, n.Op, right), nil
	case "=", "!=", ">", ">=", "<", "<=":
		return c.compileComparison(n.Op, n.Left, n.Right, ctx)
	case "+", "-", "*", "/":
		left, err := c.compileNumeric(n.Left, ctx)
		if err != nil {
			return "", err
		}
		right, err := c.compileNumeric(n.Right, ctx)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(%s %s %s)", left, n.Op, right), nil
	default:
		return "", fmt.Errorf("unsupported operator %s", n.Op)
	}
}

func (c *compiler) compileComparison(op string, left, right Expr, ctx exprContext) (string, error) {
	// Prefer typed compare based on left identifier type.
	if id, ok := left.(*IdentExpr); ok {
		dt := c.colTypes[strings.ToLower(id.Name)]
		lSQL := FieldExpr(id.Name)
		rSQL, err := c.compileExpr(right, ctx)
		if err != nil {
			return "", err
		}
		if isNumericType(dt) {
			// If right is a number literal placeholder, cast both sides.
			return fmt.Sprintf("((%s)::numeric %s (%s)::numeric)", lSQL, op, rSQL), nil
		}
		if isTimestampType(dt) {
			return fmt.Sprintf("((%s)::timestamptz %s (%s)::timestamptz)", lSQL, op, rSQL), nil
		}
		// Text: case-insensitive
		return fmt.Sprintf("(LOWER(%s) %s LOWER(%s))", lSQL, op, rSQL), nil
	}
	lSQL, err := c.compileExpr(left, ctx)
	if err != nil {
		return "", err
	}
	rSQL, err := c.compileExpr(right, ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("(%s %s %s)", lSQL, op, rSQL), nil
}

func (c *compiler) compileText(e Expr, ctx exprContext) (string, error) {
	if id, ok := e.(*IdentExpr); ok {
		return FieldExpr(id.Name), nil
	}
	return c.compileExpr(e, ctx)
}

func (c *compiler) compileNumeric(e Expr, ctx exprContext) (string, error) {
	if id, ok := e.(*IdentExpr); ok {
		return fmt.Sprintf("(%s)::numeric", FieldExpr(id.Name)), nil
	}
	s, err := c.compileExpr(e, ctx)
	if err != nil {
		return "", err
	}
	if _, ok := e.(*LiteralExpr); ok {
		return fmt.Sprintf("(%s)::numeric", s), nil
	}
	return s, nil
}

var unsupportedFuncs = map[string]string{
	"within_box":     "geospatial functions are not supported",
	"within_circle":  "geospatial functions are not supported",
	"within_polygon": "geospatial functions are not supported",
	"intersects":     "geospatial functions are not supported",
	"convex_hull":    "geospatial functions are not supported",
	"distance_in_meters": "geospatial functions are not supported",
	"extent":         "geospatial functions are not supported",
	"simplify":       "geospatial functions are not supported",
	"num_points":     "geospatial functions are not supported",
}

func (c *compiler) compileFunc(n *FuncExpr, ctx exprContext) (string, error) {
	name := normalizeIdent(n.Name)
	if msg, bad := unsupportedFuncs[name]; bad {
		return "", fmt.Errorf("%s: %s", name, msg)
	}

	switch name {
	case "count":
		if len(n.Args) == 0 || (len(n.Args) == 1 && isStar(n.Args[0])) {
			return "count(*)", nil
		}
		if len(n.Args) != 1 {
			return "", fmt.Errorf("count() takes 0 or 1 argument")
		}
		a, err := c.compileExpr(n.Args[0], ctx)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("count(%s)", a), nil
	case "sum", "avg", "min", "max":
		if len(n.Args) != 1 {
			return "", fmt.Errorf("%s() takes 1 argument", name)
		}
		a, err := c.compileNumeric(n.Args[0], ctx)
		if err != nil {
			return "", err
		}
		// min/max work on text too; use raw for non-numeric when possible
		if name == "min" || name == "max" {
			if id, ok := n.Args[0].(*IdentExpr); ok && !isNumericType(c.colTypes[strings.ToLower(id.Name)]) {
				a = FieldExpr(id.Name)
			}
		}
		return fmt.Sprintf("%s(%s)", name, a), nil
	case "upper", "lower":
		if len(n.Args) != 1 {
			return "", fmt.Errorf("%s() takes 1 argument", name)
		}
		a, err := c.compileText(n.Args[0], ctx)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s(%s)", name, a), nil
	case "abs":
		if len(n.Args) != 1 {
			return "", fmt.Errorf("abs() takes 1 argument")
		}
		a, err := c.compileNumeric(n.Args[0], ctx)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("abs(%s)", a), nil
	case "starts_with":
		if len(n.Args) != 2 {
			return "", fmt.Errorf("starts_with() takes 2 arguments")
		}
		left, err := c.compileText(n.Args[0], ctx)
		if err != nil {
			return "", err
		}
		// right should be a string literal → prepend for LIKE 'prefix%'
		if lit, ok := n.Args[1].(*LiteralExpr); ok && lit.Kind == "string" {
			ph := c.addArg(lit.Value + "%")
			return fmt.Sprintf("(LOWER(%s) LIKE LOWER(%s))", left, ph), nil
		}
		right, err := c.compileExpr(n.Args[1], ctx)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(LOWER(%s) LIKE LOWER(%s) || '%%')", left, right), nil
	case "contains":
		if len(n.Args) != 2 {
			return "", fmt.Errorf("contains() takes 2 arguments")
		}
		left, err := c.compileText(n.Args[0], ctx)
		if err != nil {
			return "", err
		}
		if lit, ok := n.Args[1].(*LiteralExpr); ok && lit.Kind == "string" {
			ph := c.addArg("%" + lit.Value + "%")
			return fmt.Sprintf("(LOWER(%s) LIKE LOWER(%s))", left, ph), nil
		}
		right, err := c.compileExpr(n.Args[1], ctx)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(LOWER(%s) LIKE '%%' || LOWER(%s) || '%%')", left, right), nil
	case "date_extract_y":
		return c.compileExtract("YEAR", n.Args, ctx)
	case "date_extract_m":
		return c.compileExtract("MONTH", n.Args, ctx)
	case "date_extract_d":
		return c.compileExtract("DAY", n.Args, ctx)
	case "date_extract_hh":
		return c.compileExtract("HOUR", n.Args, ctx)
	case "date_extract_mm":
		return c.compileExtract("MINUTE", n.Args, ctx)
	case "date_extract_ss":
		return c.compileExtract("SECOND", n.Args, ctx)
	case "date_extract_dow":
		return c.compileExtract("DOW", n.Args, ctx)
	case "date_extract_woy":
		return c.compileExtract("WEEK", n.Args, ctx)
	case "greatest", "least", "coalesce":
		if len(n.Args) < 1 {
			return "", fmt.Errorf("%s() requires arguments", name)
		}
		parts := make([]string, 0, len(n.Args))
		for _, a := range n.Args {
			s, err := c.compileExpr(a, ctx)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return fmt.Sprintf("%s(%s)", name, strings.Join(parts, ", ")), nil
	default:
		return "", fmt.Errorf("unsupported function %s()", name)
	}
}

func (c *compiler) compileExtract(field string, args []Expr, ctx exprContext) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("date_extract takes 1 argument")
	}
	var a string
	var err error
	if id, ok := args[0].(*IdentExpr); ok {
		a = fmt.Sprintf("(%s)::timestamptz", FieldExpr(id.Name))
	} else {
		a, err = c.compileExpr(args[0], ctx)
		if err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("EXTRACT(%s FROM %s)", field, a), nil
}

func (c *compiler) compileCase(n *CaseExpr, ctx exprContext) (string, error) {
	var b strings.Builder
	b.WriteString("CASE")
	for _, w := range n.Whens {
		cond, err := c.compileExpr(w.Cond, ctx)
		if err != nil {
			return "", err
		}
		result, err := c.compileExpr(w.Result, ctx)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, " WHEN %s THEN %s", cond, result)
	}
	if n.Else != nil {
		els, err := c.compileExpr(n.Else, ctx)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, " ELSE %s", els)
	}
	b.WriteString(" END")
	return b.String(), nil
}

func (c *compiler) compileQ(q string) (string, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", nil
	}
	textFields := c.textFields()
	if len(textFields) == 0 {
		// Fallback: search all known fields
		for name := range c.colTypes {
			textFields = append(textFields, name)
		}
	}
	if len(textFields) == 0 {
		return "FALSE", nil
	}
	ph := c.addArg("%" + q + "%")
	parts := make([]string, 0, len(textFields))
	for _, f := range textFields {
		parts = append(parts, fmt.Sprintf("LOWER(%s) LIKE LOWER(%s)", FieldExpr(f), ph))
	}
	return "(" + strings.Join(parts, " OR ") + ")", nil
}

func (c *compiler) textFields() []string {
	var out []string
	for name, dt := range c.colTypes {
		dt = strings.ToLower(dt)
		if dt == "" || dt == "text" || dt == "url" || (!strings.Contains(dt, "number") && !strings.Contains(dt, "timestamp") && !strings.Contains(dt, "date") && !strings.Contains(dt, "point") && !strings.Contains(dt, "location") && !strings.Contains(dt, "checkbox") && !strings.Contains(dt, "boolean")) {
			out = append(out, name)
		}
	}
	return out
}

func isStar(e Expr) bool {
	_, ok := e.(*StarExpr)
	return ok
}

func quoteIdent(name string) string {
	// table names are controlled (dataset_*), still quote safely
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// ParseWhere remains as a convenience for tests / simple callers.
// It returns SQL with inline literals via a non-parameterized compile for backward-compatible tests.
func ParseWhere(where string, colTypes ColumnTypes) (string, error) {
	expr, err := ParseExpr(where)
	if err != nil {
		return "", err
	}
	if expr == nil {
		return "", nil
	}
	c := newCompiler(colTypes)
	sql, err := c.compileExpr(expr, exprContext{clause: "where"})
	if err != nil {
		return "", err
	}
	// Substitute $N placeholders with literal args for legacy test expectations.
	return substituteArgsForTest(sql, c.args), nil
}

func substituteArgsForTest(sql string, args []interface{}) string {
	// Replace from the end so $10 does not clobber $1
	for i := len(args); i >= 1; i-- {
		ph := fmt.Sprintf("$%d", i)
		var lit string
		switch v := args[i-1].(type) {
		case string:
			// Heuristic: if placeholder was used in numeric cast context we may still quote;
			// for legacy tests, numbers without non-digit stay bare.
			if isBareNumber(v) {
				lit = v
			} else {
				lit = "'" + escapeSQLString(v) + "'"
			}
		default:
			lit = fmt.Sprint(v)
		}
		sql = strings.Replace(sql, ph, lit, 1)
	}
	return sql
}

func isBareNumber(s string) bool {
	if s == "" {
		return false
	}
	for i, ch := range s {
		if ch >= '0' && ch <= '9' || ch == '.' || (i == 0 && (ch == '-' || ch == '+')) {
			continue
		}
		return false
	}
	return true
}
