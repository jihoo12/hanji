// Package expr is Hanji's build-time expression language: the thing inside
// {{ ... }}. It is a small, lenient, JS-flavoured language with pipes:
//
//	pages("blog/*") | where("tags", "go") | limit(3)
//	page.date | date("2006-01-02")
//	page.draft ? "draft" : "published"
package expr

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// HTML marks a string as already-escaped markup.
type HTML string

// Func is a callable value.
type Func func(args []any) (any, error)

// Filter is the right-hand side of a pipe.
type Filter func(in any, args []any) (any, error)

// Env is a lexical scope.
type Env struct {
	vars   map[string]any
	parent *Env
}

func NewEnv(vars map[string]any) *Env { return &Env{vars: vars} }

func (e *Env) Child(vars map[string]any) *Env { return &Env{vars: vars, parent: e} }

func (e *Env) Get(name string) (any, bool) {
	for s := e; s != nil; s = s.parent {
		if v, ok := s.vars[name]; ok {
			return v, true
		}
	}
	return nil, false
}

// Node is a compiled expression.
type Node interface {
	Eval(env *Env) (any, error)
}

var (
	cacheMu sync.Mutex
	cache   = map[string]Node{}
)

// Parse compiles src, caching the result.
func Parse(src string) (Node, error) {
	cacheMu.Lock()
	n, ok := cache[src]
	cacheMu.Unlock()
	if ok {
		return n, nil
	}
	p := &parser{}
	if err := p.lex(src); err != nil {
		return nil, fmt.Errorf("%v in `%s`", err, src)
	}
	n, err := p.expr()
	if err == nil && p.peek().kind != tEOF {
		err = fmt.Errorf("unexpected %q", p.peek().text)
	}
	if err != nil {
		return nil, fmt.Errorf("%v in `%s`", err, src)
	}
	cacheMu.Lock()
	cache[src] = n
	cacheMu.Unlock()
	return n, nil
}

// Eval parses and evaluates src.
func Eval(src string, env *Env) (any, error) {
	n, err := Parse(src)
	if err != nil {
		return nil, err
	}
	v, err := n.Eval(env)
	if err != nil {
		return nil, fmt.Errorf("%v in `%s`", err, src)
	}
	return v, nil
}

// ---- lexer ----

type tkind int

const (
	tEOF tkind = iota
	tNum
	tStr
	tIdent
	tOp
)

type token struct {
	kind tkind
	text string
	num  float64
}

type parser struct {
	toks []token
	pos  int
}

var ops = []string{"===", "!==", "==", "!=", "<=", ">=", "&&", "||", "??", "(", ")", "[", "]", "{", "}", ",", ".", "?", ":", "+", "-", "*", "/", "%", "<", ">", "!", "|"}

func (p *parser) lex(s string) error {
	rs := []rune(s)
	for i := 0; i < len(rs); {
		c := rs[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case unicode.IsDigit(c):
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.') {
				j++
			}
			f, err := strconv.ParseFloat(string(rs[i:j]), 64)
			if err != nil {
				return fmt.Errorf("bad number %q", string(rs[i:j]))
			}
			p.toks = append(p.toks, token{kind: tNum, num: f, text: string(rs[i:j])})
			i = j
		case c == '"' || c == '\'':
			var b strings.Builder
			j := i + 1
			for ; j < len(rs) && rs[j] != c; j++ {
				if rs[j] == '\\' && j+1 < len(rs) {
					j++
					switch rs[j] {
					case 'n':
						b.WriteRune('\n')
					case 't':
						b.WriteRune('\t')
					default:
						b.WriteRune(rs[j])
					}
					continue
				}
				b.WriteRune(rs[j])
			}
			if j >= len(rs) {
				return fmt.Errorf("unterminated string")
			}
			p.toks = append(p.toks, token{kind: tStr, text: b.String()})
			i = j + 1
		case unicode.IsLetter(c) || c == '_' || c == '$':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_' || rs[j] == '$') {
				j++
			}
			p.toks = append(p.toks, token{kind: tIdent, text: string(rs[i:j])})
			i = j
		default:
			found := false
			for _, op := range ops {
				if strings.HasPrefix(string(rs[i:min(i+3, len(rs))]), op) {
					p.toks = append(p.toks, token{kind: tOp, text: op})
					i += len([]rune(op))
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("unexpected character %q", string(c))
			}
		}
	}
	p.toks = append(p.toks, token{kind: tEOF, text: "end of expression"})
	return nil
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token {
	t := p.toks[p.pos]
	if t.kind != tEOF {
		p.pos++
	}
	return t
}
func (p *parser) isOp(s string) bool { t := p.peek(); return t.kind == tOp && t.text == s }
func (p *parser) accept(s string) bool {
	if p.isOp(s) {
		p.pos++
		return true
	}
	return false
}
func (p *parser) expect(s string) error {
	if !p.accept(s) {
		return fmt.Errorf("expected %q but found %q", s, p.peek().text)
	}
	return nil
}

// ---- parser ----

func (p *parser) expr() (Node, error) {
	n, err := p.ternary()
	if err != nil {
		return nil, err
	}
	for p.accept("|") {
		name := p.next()
		if name.kind != tIdent {
			return nil, fmt.Errorf("expected a filter name after |")
		}
		var args []Node
		if p.accept("(") {
			if args, err = p.args(")"); err != nil {
				return nil, err
			}
		}
		n = &pipeNode{in: n, name: name.text, args: args}
	}
	return n, nil
}

func (p *parser) args(end string) ([]Node, error) {
	var out []Node
	for !p.accept(end) {
		a, err := p.expr()
		if err != nil {
			return nil, err
		}
		out = append(out, a)
		if !p.accept(",") {
			if err := p.expect(end); err != nil {
				return nil, err
			}
			break
		}
	}
	return out, nil
}

func (p *parser) ternary() (Node, error) {
	c, err := p.binary(0)
	if err != nil {
		return nil, err
	}
	if !p.accept("?") {
		return c, nil
	}
	a, err := p.ternary()
	if err != nil {
		return nil, err
	}
	if err := p.expect(":"); err != nil {
		return nil, err
	}
	b, err := p.ternary()
	if err != nil {
		return nil, err
	}
	return &condNode{c, a, b}, nil
}

var prec = map[string]int{"??": 1, "||": 1, "&&": 2, "==": 3, "!=": 3, "===": 3, "!==": 3, "<": 4, ">": 4, "<=": 4, ">=": 4, "+": 5, "-": 5, "*": 6, "/": 6, "%": 6}

func (p *parser) binary(minPrec int) (Node, error) {
	l, err := p.unary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		pr, ok := prec[t.text]
		if t.kind != tOp || !ok || pr <= minPrec {
			return l, nil
		}
		p.pos++
		r, err := p.binary(pr)
		if err != nil {
			return nil, err
		}
		op := t.text
		if op == "===" || op == "!==" {
			op = op[:2]
		}
		l = &binNode{op: op, l: l, r: r}
	}
}

func (p *parser) unary() (Node, error) {
	if p.accept("!") {
		n, err := p.unary()
		return &unNode{"!", n}, err
	}
	if p.accept("-") {
		n, err := p.unary()
		return &unNode{"-", n}, err
	}
	return p.postfix()
}

func (p *parser) postfix() (Node, error) {
	n, err := p.primary()
	if err != nil {
		return nil, err
	}
	for {
		switch {
		case p.accept("."):
			t := p.next()
			if t.kind != tIdent {
				return nil, fmt.Errorf("expected a name after .")
			}
			n = &indexNode{n, &lit{t.text}}
		case p.accept("["):
			k, err := p.expr()
			if err != nil {
				return nil, err
			}
			if err := p.expect("]"); err != nil {
				return nil, err
			}
			n = &indexNode{n, k}
		case p.accept("("):
			args, err := p.args(")")
			if err != nil {
				return nil, err
			}
			n = &callNode{n, args}
		default:
			return n, nil
		}
	}
}

func (p *parser) primary() (Node, error) {
	t := p.next()
	switch t.kind {
	case tNum:
		return &lit{t.num}, nil
	case tStr:
		return &lit{t.text}, nil
	case tIdent:
		switch t.text {
		case "true":
			return &lit{true}, nil
		case "false":
			return &lit{false}, nil
		case "null", "nil":
			return &lit{nil}, nil
		}
		return &identNode{t.text}, nil
	case tOp:
		switch t.text {
		case "(":
			n, err := p.expr()
			if err != nil {
				return nil, err
			}
			return n, p.expect(")")
		case "[":
			items, err := p.args("]")
			return &listNode{items}, err
		}
	}
	return nil, fmt.Errorf("unexpected %q", t.text)
}

// ---- nodes ----

type lit struct{ v any }

func (n *lit) Eval(*Env) (any, error) { return n.v, nil }

type identNode struct{ name string }

func (n *identNode) Eval(env *Env) (any, error) {
	v, _ := env.Get(n.name)
	return v, nil
}

type listNode struct{ items []Node }

func (n *listNode) Eval(env *Env) (any, error) {
	out := make([]any, 0, len(n.items))
	for _, it := range n.items {
		v, err := it.Eval(env)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

type indexNode struct{ obj, key Node }

func (n *indexNode) Eval(env *Env) (any, error) {
	o, err := n.obj.Eval(env)
	if err != nil {
		return nil, err
	}
	k, err := n.key.Eval(env)
	if err != nil {
		return nil, err
	}
	switch o := o.(type) {
	case map[string]any:
		return o[Str(k)], nil
	case []any:
		if ks, ok := k.(string); ok && ks == "length" {
			return float64(len(o)), nil
		}
		if f, ok := k.(float64); ok {
			i := int(f)
			if i < 0 {
				i += len(o)
			}
			if i >= 0 && i < len(o) {
				return o[i], nil
			}
		}
	case string:
		if ks, ok := k.(string); ok && ks == "length" {
			return float64(len([]rune(o))), nil
		}
	case time.Time:
		switch Str(k) {
		case "year":
			return float64(o.Year()), nil
		case "month":
			return float64(o.Month()), nil
		case "day":
			return float64(o.Day()), nil
		}
	}
	return nil, nil
}

type callNode struct {
	fn   Node
	args []Node
}

func (n *callNode) Eval(env *Env) (any, error) {
	f, err := n.fn.Eval(env)
	if err != nil {
		return nil, err
	}
	fn, ok := f.(Func)
	if !ok {
		name := "value"
		if id, ok := n.fn.(*identNode); ok {
			name = id.name
		}
		return nil, fmt.Errorf("%s is not a function", name)
	}
	args, err := evalAll(n.args, env)
	if err != nil {
		return nil, err
	}
	return fn(args)
}

func evalAll(ns []Node, env *Env) ([]any, error) {
	out := make([]any, len(ns))
	for i, a := range ns {
		v, err := a.Eval(env)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

type pipeNode struct {
	in   Node
	name string
	args []Node
}

func (n *pipeNode) Eval(env *Env) (any, error) {
	in, err := n.in.Eval(env)
	if err != nil {
		return nil, err
	}
	args, err := evalAll(n.args, env)
	if err != nil {
		return nil, err
	}
	if f, ok := Filters[n.name]; ok {
		return f(in, args)
	}
	// Any function in scope can also be used as a filter: x | f(a) == f(x, a)
	if v, ok := env.Get(n.name); ok {
		if fn, ok := v.(Func); ok {
			return fn(append([]any{in}, args...))
		}
	}
	return nil, fmt.Errorf("unknown filter %q", n.name)
}

type condNode struct{ c, a, b Node }

func (n *condNode) Eval(env *Env) (any, error) {
	c, err := n.c.Eval(env)
	if err != nil {
		return nil, err
	}
	if Truthy(c) {
		return n.a.Eval(env)
	}
	return n.b.Eval(env)
}

type unNode struct {
	op string
	x  Node
}

func (n *unNode) Eval(env *Env) (any, error) {
	v, err := n.x.Eval(env)
	if err != nil {
		return nil, err
	}
	if n.op == "!" {
		return !Truthy(v), nil
	}
	f, _ := Num(v)
	return -f, nil
}

type binNode struct {
	op   string
	l, r Node
}

func (n *binNode) Eval(env *Env) (any, error) {
	l, err := n.l.Eval(env)
	if err != nil {
		return nil, err
	}
	switch n.op {
	case "&&":
		if !Truthy(l) {
			return l, nil
		}
		return n.r.Eval(env)
	case "||":
		if Truthy(l) {
			return l, nil
		}
		return n.r.Eval(env)
	case "??":
		if l != nil {
			return l, nil
		}
		return n.r.Eval(env)
	}
	r, err := n.r.Eval(env)
	if err != nil {
		return nil, err
	}
	switch n.op {
	case "==":
		return Equal(l, r), nil
	case "!=":
		return !Equal(l, r), nil
	case "<":
		return Compare(l, r) < 0, nil
	case ">":
		return Compare(l, r) > 0, nil
	case "<=":
		return Compare(l, r) <= 0, nil
	case ">=":
		return Compare(l, r) >= 0, nil
	case "+":
		lf, lok := l.(float64)
		rf, rok := r.(float64)
		if lok && rok {
			return lf + rf, nil
		}
		if la, ok := l.([]any); ok {
			if ra, ok := r.([]any); ok {
				return append(append([]any{}, la...), ra...), nil
			}
		}
		return Str(l) + Str(r), nil
	}
	lf, _ := Num(l)
	rf, _ := Num(r)
	switch n.op {
	case "-":
		return lf - rf, nil
	case "*":
		return lf * rf, nil
	case "/":
		if rf == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return lf / rf, nil
	case "%":
		if rf == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return math.Mod(lf, rf), nil
	}
	return nil, fmt.Errorf("unknown operator %s", n.op)
}

// ---- value helpers ----

func Truthy(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		return v != ""
	case HTML:
		return v != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	case time.Time:
		return !v.IsZero()
	}
	return true
}

func Num(v any) (float64, bool) {
	switch v := v.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case bool:
		if v {
			return 1, true
		}
		return 0, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	}
	return 0, false
}

// Str renders a value the way {{ }} prints it.
func Str(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case HTML:
		return string(v)
	case float64:
		if v == math.Trunc(v) && math.Abs(v) < 1e15 {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case time.Time:
		return v.Format("2006-01-02")
	case []any:
		parts := make([]string, len(v))
		for i, x := range v {
			parts[i] = Str(x)
		}
		return strings.Join(parts, ", ")
	}
	return fmt.Sprint(v)
}

func Equal(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	switch a := a.(type) {
	case float64, bool, string, HTML:
		switch b.(type) {
		case float64, bool, string, HTML:
			if af, ok := a.(float64); ok {
				if bf, ok := b.(float64); ok {
					return af == bf
				}
			}
			return Str(a) == Str(b)
		}
		return false
	case time.Time:
		if bt, ok := b.(time.Time); ok {
			return a.Equal(bt)
		}
		return false
	case map[string]any:
		// Pages are maps; identity is good enough and avoids deep walks.
		if bm, ok := b.(map[string]any); ok {
			return fmt.Sprintf("%p", a) == fmt.Sprintf("%p", bm)
		}
	}
	return false
}

// Compare orders numbers, times and strings; mixed types compare as strings.
func Compare(a, b any) int {
	if at, ok := a.(time.Time); ok {
		if bt, ok := b.(time.Time); ok {
			return at.Compare(bt)
		}
	}
	af, aok := a.(float64)
	bf, bok := b.(float64)
	if aok && bok {
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		}
		return 0
	}
	return strings.Compare(Str(a), Str(b))
}
