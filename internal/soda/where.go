package soda

import (
	"fmt"
	"strings"
	"unicode"
)

// ParseWhere converts a SODA $where clause into a SQL fragment for GORM Where().
func ParseWhere(where string, colTypes ColumnTypes) (string, error) {
	where = strings.TrimSpace(where)
	if where == "" {
		return "", nil
	}
	tokens := tokenize(where)
	if len(tokens) == 0 {
		return "", nil
	}
	p := &parser{tokens: tokens, colTypes: colTypes}
	expr, err := p.parseExpr()
	if err != nil {
		return "", err
	}
	if p.pos < len(p.tokens) {
		return "", fmt.Errorf("unexpected token: %s", p.tokens[p.pos])
	}
	return expr, nil
}

type parser struct {
	tokens   []string
	pos      int
	colTypes ColumnTypes
}

func (p *parser) parseExpr() (string, error) {
	return p.parseOr()
}

func (p *parser) parseOr() (string, error) {
	left, err := p.parseAnd()
	if err != nil {
		return "", err
	}
	for p.match("OR") {
		right, err := p.parseAnd()
		if err != nil {
			return "", err
		}
		left = fmt.Sprintf("(%s OR %s)", left, right)
	}
	return left, nil
}

func (p *parser) parseAnd() (string, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return "", err
	}
	for p.match("AND") {
		right, err := p.parsePrimary()
		if err != nil {
			return "", err
		}
		left = fmt.Sprintf("(%s AND %s)", left, right)
	}
	return left, nil
}

func (p *parser) parsePrimary() (string, error) {
	if p.match("(") {
		expr, err := p.parseExpr()
		if err != nil {
			return "", err
		}
		if !p.match(")") {
			return "", fmt.Errorf("expected )")
		}
		return expr, nil
	}
	return p.parseComparison()
}

func (p *parser) parseComparison() (string, error) {
	field := p.next()
	if field == "" {
		return "", fmt.Errorf("expected field name")
	}
	field = strings.Trim(field, "`")

	if p.peek() == "IS" {
		p.next()
		if p.match("NOT") {
			if !p.match("NULL") {
				return "", fmt.Errorf("expected NULL")
			}
			return fmt.Sprintf("%s IS NOT NULL", FieldExpr(field)), nil
		}
		if !p.match("NULL") {
			return "", fmt.Errorf("expected NULL")
		}
		return fmt.Sprintf("%s IS NULL", FieldExpr(field)), nil
	}

	op := p.next()
	if op == "" {
		return "", fmt.Errorf("expected operator")
	}

	value := p.next()
	if value == "" {
		return "", fmt.Errorf("expected value")
	}

	expr := FieldExpr(field)
	dt := p.colTypes[strings.ToLower(field)]
	val := strings.Trim(value, "'\"")

	switch strings.ToUpper(op) {
	case "=", "!=", "<>", ">", ">=", "<", "<=":
		sqlOp := op
		if sqlOp == "<>" {
			sqlOp = "!="
		}
		if isNumericType(dt) {
			return fmt.Sprintf("(%s)::numeric %s %s", expr, sqlOp, val), nil
		}
		if isTimestampType(dt) {
			return fmt.Sprintf("(%s)::timestamptz %s '%s'", expr, sqlOp, escapeSQLString(val)), nil
		}
		return fmt.Sprintf("LOWER(%s) %s LOWER('%s')", expr, sqlOp, escapeSQLString(val)), nil
	default:
		return "", fmt.Errorf("unsupported operator: %s", op)
	}
}

func (p *parser) peek() string {
	if p.pos >= len(p.tokens) {
		return ""
	}
	return strings.ToUpper(p.tokens[p.pos])
}

func (p *parser) next() string {
	if p.pos >= len(p.tokens) {
		return ""
	}
	t := p.tokens[p.pos]
	p.pos++
	return t
}

func (p *parser) match(upper string) bool {
	if p.pos < len(p.tokens) && strings.ToUpper(p.tokens[p.pos]) == upper {
		p.pos++
		return true
	}
	return false
}

func tokenize(s string) []string {
	var tokens []string
	i := 0
	for i < len(s) {
		for i < len(s) && unicode.IsSpace(rune(s[i])) {
			i++
		}
		if i >= len(s) {
			break
		}

		switch s[i] {
		case '(':
			tokens = append(tokens, "(")
			i++
		case ')':
			tokens = append(tokens, ")")
			i++
		case '\'':
			j := i + 1
			for j < len(s) {
				if s[j] == '\'' {
					if j+1 < len(s) && s[j+1] == '\'' {
						j += 2
						continue
					}
					break
				}
				j++
			}
			tokens = append(tokens, s[i:j+1])
			i = j + 1
		default:
			j := i
			for j < len(s) && !unicode.IsSpace(rune(s[j])) && s[j] != '(' && s[j] != ')' && s[j] != '\'' {
				j++
			}
			tokens = append(tokens, s[i:j])
			i = j
		}
	}
	return tokens
}

func isNumericType(dt string) bool {
	return strings.Contains(strings.ToLower(dt), "number")
}

func isTimestampType(dt string) bool {
	dt = strings.ToLower(dt)
	return strings.Contains(dt, "timestamp") || strings.Contains(dt, "date")
}
