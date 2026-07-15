package soda

// SelectStmt is a compiled SoQL SELECT statement.
type SelectStmt struct {
	Distinct    bool
	SelectAll   bool
	SelectItems []SelectItem
	Where       Expr
	GroupBy     []Expr
	Having      Expr
	OrderBy     []OrderItem
	Limit       int
	Offset      int
	HasLimit    bool
	Q           string
}

type SelectItem struct {
	Expr  Expr
	Alias string
}

type OrderItem struct {
	Expr Expr
	Desc bool
}

// Expr is a SoQL expression node.
type Expr interface {
	exprNode()
}

type IdentExpr struct{ Name string }
type StarExpr struct{}
type LiteralExpr struct {
	Kind  string // "string", "number", "null", "bool"
	Value string // raw text; number/bool/null also stored here
}
type UnaryExpr struct {
	Op string
	X  Expr
}
type BinaryExpr struct {
	Op          string
	Left, Right Expr
}
type BetweenExpr struct {
	X, Low, High Expr
	Not          bool
}
type InExpr struct {
	X      Expr
	Values []Expr
	Not    bool
}
type FuncExpr struct {
	Name string
	Args []Expr
}
type IsNullExpr struct {
	X   Expr
	Not bool
}
type CaseExpr struct {
	Whens []CaseWhen
	Else  Expr
}
type CaseWhen struct {
	Cond, Result Expr
}

func (IdentExpr) exprNode()   {}
func (StarExpr) exprNode()    {}
func (LiteralExpr) exprNode() {}
func (UnaryExpr) exprNode()   {}
func (BinaryExpr) exprNode()  {}
func (BetweenExpr) exprNode() {}
func (InExpr) exprNode()      {}
func (FuncExpr) exprNode()    {}
func (IsNullExpr) exprNode()  {}
func (CaseExpr) exprNode()    {}

func (s *SelectStmt) HasAggregates() bool {
	for _, item := range s.SelectItems {
		if containsAggregate(item.Expr) {
			return true
		}
	}
	if len(s.GroupBy) > 0 || s.Having != nil {
		return true
	}
	return false
}

func containsAggregate(e Expr) bool {
	switch n := e.(type) {
	case *FuncExpr:
		switch normalizeIdent(n.Name) {
		case "count", "sum", "avg", "min", "max":
			return true
		}
		for _, a := range n.Args {
			if containsAggregate(a) {
				return true
			}
		}
	case *UnaryExpr:
		return containsAggregate(n.X)
	case *BinaryExpr:
		return containsAggregate(n.Left) || containsAggregate(n.Right)
	case *BetweenExpr:
		return containsAggregate(n.X) || containsAggregate(n.Low) || containsAggregate(n.High)
	case *InExpr:
		if containsAggregate(n.X) {
			return true
		}
		for _, v := range n.Values {
			if containsAggregate(v) {
				return true
			}
		}
	case *IsNullExpr:
		return containsAggregate(n.X)
	case *CaseExpr:
		for _, w := range n.Whens {
			if containsAggregate(w.Cond) || containsAggregate(w.Result) {
				return true
			}
		}
		if n.Else != nil {
			return containsAggregate(n.Else)
		}
	}
	return false
}
