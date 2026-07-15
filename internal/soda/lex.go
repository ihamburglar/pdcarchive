package soda

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokNumber
	tokString
	tokOp
	tokLParen
	tokRParen
	tokComma
	tokStar
)

type token struct {
	kind tokenKind
	val  string
}

type lexer struct {
	src string
	pos int
}

func (l *lexer) next() (token, error) {
	l.skipSpace()
	if l.pos >= len(l.src) {
		return token{kind: tokEOF}, nil
	}

	ch, size := utf8.DecodeRuneInString(l.src[l.pos:])
	switch ch {
	case '(':
		l.pos += size
		return token{kind: tokLParen, val: "("}, nil
	case ')':
		l.pos += size
		return token{kind: tokRParen, val: ")"}, nil
	case ',':
		l.pos += size
		return token{kind: tokComma, val: ","}, nil
	case '*':
		l.pos += size
		return token{kind: tokStar, val: "*"}, nil
	case '\'':
		return l.readString()
	}

	// Multi-char operators first
	rest := l.src[l.pos:]
	for _, op := range []string{"!=", "<>", ">=", "<=", "="} {
		if strings.HasPrefix(rest, op) {
			l.pos += len(op)
			return token{kind: tokOp, val: op}, nil
		}
	}
	if ch == '>' || ch == '<' {
		l.pos += size
		return token{kind: tokOp, val: string(ch)}, nil
	}
	if ch == '+' || ch == '-' || ch == '/' {
		// '-' may start a negative number; handled below if digit follows
		if ch == '-' && l.pos+size < len(l.src) {
			next, _ := utf8.DecodeRuneInString(l.src[l.pos+size:])
			if unicode.IsDigit(next) || next == '.' {
				return l.readNumber()
			}
		}
		l.pos += size
		return token{kind: tokOp, val: string(ch)}, nil
	}

	if unicode.IsDigit(ch) || ch == '.' {
		return l.readNumber()
	}

	if ch == '`' {
		return l.readQuotedIdent()
	}

	if isIdentStart(ch) {
		return l.readIdent()
	}

	return token{}, fmt.Errorf("unexpected character %q", string(ch))
}

func (l *lexer) skipSpace() {
	for l.pos < len(l.src) {
		ch, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if !unicode.IsSpace(ch) {
			return
		}
		l.pos += size
	}
}

func (l *lexer) readString() (token, error) {
	l.pos++ // opening '
	var b strings.Builder
	for l.pos < len(l.src) {
		ch, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if ch == '\'' {
			if l.pos+size < len(l.src) {
				next, nsize := utf8.DecodeRuneInString(l.src[l.pos+size:])
				if next == '\'' {
					b.WriteRune('\'')
					l.pos += size + nsize
					continue
				}
			}
			l.pos += size
			return token{kind: tokString, val: b.String()}, nil
		}
		b.WriteRune(ch)
		l.pos += size
	}
	return token{}, fmt.Errorf("unterminated string literal")
}

func (l *lexer) readQuotedIdent() (token, error) {
	l.pos++ // opening `
	start := l.pos
	for l.pos < len(l.src) {
		ch, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if ch == '`' {
			name := l.src[start:l.pos]
			l.pos += size
			return token{kind: tokIdent, val: name}, nil
		}
		l.pos += size
	}
	return token{}, fmt.Errorf("unterminated quoted identifier")
}

func (l *lexer) readNumber() (token, error) {
	start := l.pos
	if l.src[l.pos] == '-' || l.src[l.pos] == '+' {
		l.pos++
	}
	for l.pos < len(l.src) {
		ch, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if unicode.IsDigit(ch) || ch == '.' || ch == 'e' || ch == 'E' {
			l.pos += size
			continue
		}
		break
	}
	return token{kind: tokNumber, val: l.src[start:l.pos]}, nil
}

func (l *lexer) readIdent() (token, error) {
	start := l.pos
	for l.pos < len(l.src) {
		ch, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if isIdentContinue(ch) {
			l.pos += size
			continue
		}
		break
	}
	return token{kind: tokIdent, val: l.src[start:l.pos]}, nil
}

func isIdentStart(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_' || ch == ':'
}

func isIdentContinue(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == ':'
}

func normalizeIdent(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
