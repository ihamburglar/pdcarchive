package soda

import (
	"fmt"
	"strconv"
	"strings"
)

type parser struct {
	tokens []token
	pos    int
}

func tokenizeAll(src string) ([]token, error) {
	l := &lexer{src: src}
	var tokens []token
	for {
		t, err := l.next()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
		if t.kind == tokEOF {
			return tokens, nil
		}
	}
}

func newParser(src string) (*parser, error) {
	tokens, err := tokenizeAll(src)
	if err != nil {
		return nil, err
	}
	return &parser{tokens: tokens}, nil
}

func (p *parser) peek() token {
	if p.pos >= len(p.tokens) {
		return token{kind: tokEOF}
	}
	return p.tokens[p.pos]
}

func (p *parser) next() token {
	t := p.peek()
	if t.kind != tokEOF {
		p.pos++
	}
	return t
}

func (p *parser) matchIdent(names ...string) bool {
	t := p.peek()
	if t.kind != tokIdent {
		return false
	}
	upper := strings.ToUpper(t.val)
	for _, n := range names {
		if upper == strings.ToUpper(n) {
			p.pos++
			return true
		}
	}
	return false
}

func (p *parser) matchOp(ops ...string) bool {
	t := p.peek()
	if t.kind != tokOp {
		return false
	}
	for _, op := range ops {
		if t.val == op {
			p.pos++
			return true
		}
	}
	return false
}

func (p *parser) expectIdent(name string) error {
	if p.matchIdent(name) {
		return nil
	}
	return fmt.Errorf("expected %s, got %s", name, p.peek().val)
}

func (p *parser) expectKind(kind tokenKind, label string) (token, error) {
	t := p.peek()
	if t.kind != kind {
		return token{}, fmt.Errorf("expected %s, got %q", label, t.val)
	}
	p.pos++
	return t, nil
}

// ParseExpr parses a SoQL expression (used for $where / $having / select items).
func ParseExpr(src string) (Expr, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, nil
	}
	p, err := newParser(src)
	if err != nil {
		return nil, err
	}
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tokEOF {
		return nil, fmt.Errorf("unexpected token %q", p.peek().val)
	}
	return expr, nil
}

// ParseSelectList parses a comma-separated $select list.
func ParseSelectList(src string) (all bool, items []SelectItem, err error) {
	src = strings.TrimSpace(src)
	if src == "" || src == "*" {
		return true, nil, nil
	}
	p, err := newParser(src)
	if err != nil {
		return false, nil, err
	}
	if p.peek().kind == tokStar && len(p.tokens) == 2 { // * and EOF
		return true, nil, nil
	}
	for {
		item, err := p.parseSelectItem()
		if err != nil {
			return false, nil, err
		}
		items = append(items, item)
		if p.peek().kind == tokComma {
			p.next()
			continue
		}
		break
	}
	if p.peek().kind != tokEOF {
		return false, nil, fmt.Errorf("unexpected token %q in $select", p.peek().val)
	}
	return false, items, nil
}

func (p *parser) parseSelectItem() (SelectItem, error) {
	if p.peek().kind == tokStar {
		p.next()
		return SelectItem{Expr: &StarExpr{}}, nil
	}
	expr, err := p.parseExpr()
	if err != nil {
		return SelectItem{}, err
	}
	alias := ""
	if p.matchIdent("AS") {
		t, err := p.expectKind(tokIdent, "alias")
		if err != nil {
			return SelectItem{}, err
		}
		alias = t.val
	} else if p.peek().kind == tokIdent {
		// Bare alias without AS is common in SoQL (select amount total)
		// Only treat as alias if not a keyword that starts a new clause.
		name := strings.ToUpper(p.peek().val)
		if !isClauseKeyword(name) && name != "ASC" && name != "DESC" {
			alias = p.next().val
		}
	}
	return SelectItem{Expr: expr, Alias: alias}, nil
}

// ParseOrderList parses $order (multi-column).
func ParseOrderList(src string) ([]OrderItem, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, nil
	}
	p, err := newParser(src)
	if err != nil {
		return nil, err
	}
	var items []OrderItem
	for {
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		desc := false
		if p.matchIdent("DESC") {
			desc = true
		} else {
			p.matchIdent("ASC")
		}
		items = append(items, OrderItem{Expr: expr, Desc: desc})
		if p.peek().kind == tokComma {
			p.next()
			continue
		}
		break
	}
	if p.peek().kind != tokEOF {
		return nil, fmt.Errorf("unexpected token %q in $order", p.peek().val)
	}
	return items, nil
}

// ParseGroupList parses $group.
func ParseGroupList(src string) ([]Expr, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, nil
	}
	p, err := newParser(src)
	if err != nil {
		return nil, err
	}
	var items []Expr
	for {
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		items = append(items, expr)
		if p.peek().kind == tokComma {
			p.next()
			continue
		}
		break
	}
	if p.peek().kind != tokEOF {
		return nil, fmt.Errorf("unexpected token %q in $group", p.peek().val)
	}
	return items, nil
}

// ParseQuery parses a full SoQL $query statement.
func ParseQuery(src string) (*SelectStmt, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, fmt.Errorf("empty $query")
	}
	p, err := newParser(src)
	if err != nil {
		return nil, err
	}
	stmt := &SelectStmt{}

	if err := p.expectIdent("SELECT"); err != nil {
		return nil, err
	}
	if p.matchIdent("DISTINCT") {
		stmt.Distinct = true
	}

	if p.peek().kind == tokStar {
		p.next()
		stmt.SelectAll = true
		if p.peek().kind == tokComma {
			return nil, fmt.Errorf("cannot mix * with other $select items")
		}
	} else {
		for {
			item, err := p.parseSelectItem()
			if err != nil {
				return nil, err
			}
			if _, ok := item.Expr.(*StarExpr); ok && len(stmt.SelectItems) == 0 && p.peek().kind != tokComma {
				stmt.SelectAll = true
				break
			}
			stmt.SelectItems = append(stmt.SelectItems, item)
			if p.peek().kind == tokComma {
				p.next()
				continue
			}
			break
		}
	}

	if p.matchIdent("WHERE") {
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		stmt.Where = expr
	}
	if p.matchIdent("GROUP") {
		if err := p.expectIdent("BY"); err != nil {
			return nil, err
		}
		for {
			expr, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			stmt.GroupBy = append(stmt.GroupBy, expr)
			if p.peek().kind == tokComma {
				p.next()
				continue
			}
			break
		}
	}
	if p.matchIdent("HAVING") {
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		stmt.Having = expr
	}
	if p.matchIdent("ORDER") {
		if err := p.expectIdent("BY"); err != nil {
			return nil, err
		}
		for {
			expr, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			desc := false
			if p.matchIdent("DESC") {
				desc = true
			} else {
				p.matchIdent("ASC")
			}
			stmt.OrderBy = append(stmt.OrderBy, OrderItem{Expr: expr, Desc: desc})
			if p.peek().kind == tokComma {
				p.next()
				continue
			}
			break
		}
	}
	if p.matchIdent("LIMIT") {
		n, err := p.parseIntLiteral()
		if err != nil {
			return nil, err
		}
		stmt.Limit = n
		stmt.HasLimit = true
	}
	if p.matchIdent("OFFSET") {
		n, err := p.parseIntLiteral()
		if err != nil {
			return nil, err
		}
		stmt.Offset = n
	}
	if p.peek().kind != tokEOF {
		return nil, fmt.Errorf("unexpected token %q in $query", p.peek().val)
	}
	return stmt, nil
}

func (p *parser) parseIntLiteral() (int, error) {
	t := p.peek()
	if t.kind != tokNumber {
		return 0, fmt.Errorf("expected integer, got %q", t.val)
	}
	p.next()
	n, err := strconv.Atoi(t.val)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q", t.val)
	}
	return n, nil
}

func isClauseKeyword(name string) bool {
	switch name {
	case "WHERE", "GROUP", "HAVING", "ORDER", "LIMIT", "OFFSET", "BY", "AND", "OR", "NOT", "IN", "LIKE", "BETWEEN", "IS", "AS", "SELECT", "DISTINCT", "FROM":
		return true
	}
	return false
}

// ---- expression grammar ----
// expr := or
// or := and (OR and)*
// and := not (AND not)*
// not := NOT not | predicate
// predicate := compare ( (NOT)? IN (...) | (NOT)? BETWEEN ... AND ... | (NOT)? LIKE ... )?
// compare := add (compOp add)?
// add := mul ((+|-) mul)*
// mul := unary ((*|/) unary)*
// unary := (+|-) unary | primary
// primary := literal | ident | func | CASE | ( expr ) | *

func (p *parser) parseExpr() (Expr, error) {
	return p.parseOr()
}

func (p *parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.matchIdent("OR") {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: "OR", Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (Expr, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.matchIdent("AND") {
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: "AND", Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseNot() (Expr, error) {
	if p.matchIdent("NOT") {
		x, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Op: "NOT", X: x}, nil
	}
	return p.parsePredicate()
}

func (p *parser) parsePredicate() (Expr, error) {
	left, err := p.parseCompare()
	if err != nil {
		return nil, err
	}

	// IS [NOT] NULL
	if p.matchIdent("IS") {
		not := p.matchIdent("NOT")
		if !p.matchIdent("NULL") {
			return nil, fmt.Errorf("expected NULL after IS")
		}
		return &IsNullExpr{X: left, Not: not}, nil
	}

	not := false
	if p.matchIdent("NOT") {
		not = true
	}

	if p.matchIdent("IN") {
		if _, err := p.expectKind(tokLParen, "("); err != nil {
			return nil, err
		}
		var values []Expr
		for {
			v, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			values = append(values, v)
			if p.peek().kind == tokComma {
				p.next()
				continue
			}
			break
		}
		if _, err := p.expectKind(tokRParen, ")"); err != nil {
			return nil, err
		}
		return &InExpr{X: left, Values: values, Not: not}, nil
	}

	if p.matchIdent("BETWEEN") {
		low, err := p.parseCompare()
		if err != nil {
			return nil, err
		}
		if !p.matchIdent("AND") {
			return nil, fmt.Errorf("expected AND in BETWEEN")
		}
		high, err := p.parseCompare()
		if err != nil {
			return nil, err
		}
		return &BetweenExpr{X: left, Low: low, High: high, Not: not}, nil
	}

	if p.matchIdent("LIKE") {
		right, err := p.parseCompare()
		if err != nil {
			return nil, err
		}
		op := "LIKE"
		if not {
			op = "NOT LIKE"
		}
		return &BinaryExpr{Op: op, Left: left, Right: right}, nil
	}

	if not {
		return nil, fmt.Errorf("unexpected NOT")
	}
	return left, nil
}

func (p *parser) parseCompare() (Expr, error) {
	left, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	t := p.peek()
	if t.kind == tokOp && (t.val == "=" || t.val == "!=" || t.val == "<>" || t.val == ">" || t.val == ">=" || t.val == "<" || t.val == "<=") {
		op := p.next().val
		if op == "<>" {
			op = "!="
		}
		right, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		return &BinaryExpr{Op: op, Left: left, Right: right}, nil
	}
	return left, nil
}

func (p *parser) parseAdd() (Expr, error) {
	left, err := p.parseMul()
	if err != nil {
		return nil, err
	}
	for p.matchOp("+", "-") {
		op := p.tokens[p.pos-1].val
		right, err := p.parseMul()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseMul() (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind == tokStar {
			p.next()
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			left = &BinaryExpr{Op: "*", Left: left, Right: right}
			continue
		}
		if t.kind == tokOp && t.val == "/" {
			p.next()
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			left = &BinaryExpr{Op: "/", Left: left, Right: right}
			continue
		}
		break
	}
	return left, nil
}

func (p *parser) parseUnary() (Expr, error) {
	if p.matchOp("+", "-") {
		op := p.tokens[p.pos-1].val
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Op: op, X: x}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (Expr, error) {
	t := p.peek()
	switch t.kind {
	case tokLParen:
		p.next()
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expectKind(tokRParen, ")"); err != nil {
			return nil, err
		}
		return expr, nil
	case tokStar:
		p.next()
		return &StarExpr{}, nil
	case tokString:
		p.next()
		return &LiteralExpr{Kind: "string", Value: t.val}, nil
	case tokNumber:
		p.next()
		return &LiteralExpr{Kind: "number", Value: t.val}, nil
	case tokIdent:
		name := t.val
		upper := strings.ToUpper(name)
		if upper == "NULL" {
			p.next()
			return &LiteralExpr{Kind: "null", Value: "null"}, nil
		}
		if upper == "TRUE" || upper == "FALSE" {
			p.next()
			return &LiteralExpr{Kind: "bool", Value: strings.ToLower(name)}, nil
		}
		if upper == "CASE" {
			p.next()
			return p.parseCase()
		}
		p.next()
		if p.peek().kind == tokLParen {
			return p.parseFuncCall(name)
		}
		return &IdentExpr{Name: name}, nil
	default:
		return nil, fmt.Errorf("unexpected token %q", t.val)
	}
}

func (p *parser) parseFuncCall(name string) (Expr, error) {
	p.next() // (
	var args []Expr
	if p.peek().kind != tokRParen {
		for {
			// COUNT(*) special-case
			if p.peek().kind == tokStar {
				p.next()
				args = append(args, &StarExpr{})
			} else {
				arg, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
			}
			if p.peek().kind == tokComma {
				p.next()
				continue
			}
			break
		}
	}
	if _, err := p.expectKind(tokRParen, ")"); err != nil {
		return nil, err
	}
	return &FuncExpr{Name: name, Args: args}, nil
}

func (p *parser) parseCase() (Expr, error) {
	var whens []CaseWhen
	for p.matchIdent("WHEN") {
		cond, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if !p.matchIdent("THEN") {
			return nil, fmt.Errorf("expected THEN in CASE")
		}
		result, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		whens = append(whens, CaseWhen{Cond: cond, Result: result})
	}
	var elseExpr Expr
	if p.matchIdent("ELSE") {
		var err error
		elseExpr, err = p.parseExpr()
		if err != nil {
			return nil, err
		}
	}
	if !p.matchIdent("END") {
		return nil, fmt.Errorf("expected END in CASE")
	}
	if len(whens) == 0 {
		return nil, fmt.Errorf("CASE requires at least one WHEN")
	}
	return &CaseExpr{Whens: whens, Else: elseExpr}, nil
}
