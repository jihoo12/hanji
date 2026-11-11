// Package tmpl is the HTML template engine for layouts and components.
//
//	{{ expr }}                      escaped output ({{ x | raw }} to opt out)
//	{{#if a}} … {{:else if b}} … {{:else}} … {{/if}}
//	{{#each item, i in list}} … {{:else}} empty … {{/each}}
//	{{> card title="Hi" href=page.url}}          a component, no body
//	{{#> card title="Hi"}} body {{/>}}           a component with a body
//	{{slot}}                        the body handed to this layout/component
//	{{! comment }}   \{{ literal braces
package tmpl

import (
	"fmt"
	"html"
	"strings"

	"hanji/internal/expr"
)

// Ctx carries what a render needs beyond the variable scope.
type Ctx struct {
	Slot      string
	Component func(name string, props map[string]any, body string, env *expr.Env) (string, error)
}

type Template struct {
	Name  string
	nodes []node
}

type node interface {
	render(b *strings.Builder, env *expr.Env, ctx *Ctx) error
}

type (
	textNode string
	outNode  struct{ e expr.Node }
	slotNode struct{}
	branch   struct {
		cond expr.Node
		body []node
	}
	ifNode   struct{ branches []branch }
	eachNode struct {
		v, idx      string
		list        expr.Node
		body, empty []node
	}
	Arg struct {
		Key string
		E   expr.Node
	}
	useNode struct {
		name string
		args []Arg
		body []node
	}
)

type tag struct {
	text string // trimmed content between {{ and }}
	line int
}

type parser struct {
	name string
	src  string
	pos  int
}

func Parse(name, src string) (*Template, error) {
	p := &parser{name: name, src: src}
	nodes, end, err := p.until()
	if err != nil {
		return nil, err
	}
	if end != nil {
		return nil, p.errf(end.line, "unexpected {{%s}}", end.text)
	}
	return &Template{Name: name, nodes: nodes}, nil
}

func (p *parser) errf(line int, format string, a ...any) error {
	return fmt.Errorf("%s:%d: %s", p.name, line, fmt.Sprintf(format, a...))
}

// nextTag returns the literal text before the next tag and the tag itself (nil at EOF).
func (p *parser) nextTag() (string, *tag, error) {
	var text strings.Builder
	for {
		i := strings.Index(p.src[p.pos:], "{{")
		if i < 0 {
			text.WriteString(p.src[p.pos:])
			p.pos = len(p.src)
			return text.String(), nil, nil
		}
		i += p.pos
		if i > 0 && p.src[i-1] == '\\' {
			text.WriteString(p.src[p.pos : i-1])
			text.WriteString("{{")
			p.pos = i + 2
			continue
		}
		text.WriteString(p.src[p.pos:i])
		line := 1 + strings.Count(p.src[:i], "\n")
		end := closeBraces(p.src, i+2)
		if end < 0 {
			return "", nil, p.errf(line, "unclosed {{")
		}
		p.pos = end + 2
		return text.String(), &tag{text: strings.TrimSpace(p.src[i+2 : end]), line: line}, nil
	}
}

// closeBraces finds the "}}" ending a tag, skipping quoted strings.
func closeBraces(s string, from int) int {
	var q byte
	for i := from; i < len(s)-1; i++ {
		c := s[i]
		switch {
		case q != 0:
			if c == '\\' {
				i++
			} else if c == q {
				q = 0
			}
		case c == '"' || c == '\'':
			q = c
		case c == '}' && s[i+1] == '}':
			return i
		}
	}
	return -1
}

// until parses nodes up to a closing or continuation tag ({{/…}} or {{:…}}),
// which it returns unconsumed-in-meaning for the caller to interpret.
func (p *parser) until() ([]node, *tag, error) {
	var nodes []node
	for {
		text, t, err := p.nextTag()
		if err != nil {
			return nil, nil, err
		}
		if text != "" {
			nodes = append(nodes, textNode(text))
		}
		if t == nil {
			return nodes, nil, nil
		}
		s := t.text
		switch {
		case s == "" || s[0] == '!':
		case s[0] == '/' || s[0] == ':':
			return nodes, t, nil
		case s == "slot":
			nodes = append(nodes, slotNode{})
		case strings.HasPrefix(s, "#if "):
			n, err := p.parseIf(t)
			if err != nil {
				return nil, nil, err
			}
			nodes = append(nodes, n)
		case strings.HasPrefix(s, "#each "):
			n, err := p.parseEach(t)
			if err != nil {
				return nil, nil, err
			}
			nodes = append(nodes, n)
		case strings.HasPrefix(s, "#>"), s[0] == '>':
			hasBody := s[0] == '#'
			name, args, err := ParseArgs(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(s, "#"), ">")))
			if err != nil {
				return nil, nil, p.errf(t.line, "%v", err)
			}
			u := &useNode{name: name, args: args}
			if hasBody {
				body, end, err := p.until()
				if err != nil {
					return nil, nil, err
				}
				if end == nil || end.text != "/>" {
					return nil, nil, p.errf(t.line, "{{#> %s}} is missing its {{/>}}", name)
				}
				u.body = body
			}
			nodes = append(nodes, u)
		case s[0] == '#':
			return nil, nil, p.errf(t.line, "unknown block {{%s}}", s)
		default:
			e, err := expr.Parse(s)
			if err != nil {
				return nil, nil, p.errf(t.line, "%v", err)
			}
			nodes = append(nodes, outNode{e})
		}
	}
}

func (p *parser) parseIf(t *tag) (node, error) {
	n := &ifNode{}
	condSrc := strings.TrimPrefix(t.text, "#if ")
	for {
		var cond expr.Node
		if condSrc != "" {
			var err error
			if cond, err = expr.Parse(condSrc); err != nil {
				return nil, p.errf(t.line, "%v", err)
			}
		}
		body, end, err := p.until()
		if err != nil {
			return nil, err
		}
		n.branches = append(n.branches, branch{cond, body})
		switch {
		case end == nil:
			return nil, p.errf(t.line, "{{#if}} is missing its {{/if}}")
		case end.text == "/if":
			return n, nil
		case end.text == ":else" && cond != nil:
			condSrc = ""
		case strings.HasPrefix(end.text, ":else if ") && cond != nil:
			condSrc = strings.TrimPrefix(end.text, ":else if ")
		default:
			return nil, p.errf(end.line, "unexpected {{%s}} inside {{#if}}", end.text)
		}
	}
}

func (p *parser) parseEach(t *tag) (node, error) {
	v, idx, listSrc, ok := SplitEach(strings.TrimPrefix(t.text, "#each "))
	if !ok {
		return nil, p.errf(t.line, "expected {{#each item in list}}")
	}
	list, err := expr.Parse(listSrc)
	if err != nil {
		return nil, p.errf(t.line, "%v", err)
	}
	n := &eachNode{v: v, idx: idx, list: list}
	body, end, err := p.until()
	if err != nil {
		return nil, err
	}
	n.body = body
	if end != nil && end.text == ":else" {
		if n.empty, end, err = p.until(); err != nil {
			return nil, err
		}
	}
	if end == nil || end.text != "/each" {
		return nil, p.errf(t.line, "{{#each}} is missing its {{/each}}")
	}
	return n, nil
}

// SplitEach splits "item, i in expr" into its parts.
func SplitEach(s string) (v, idx, list string, ok bool) {
	i := strings.Index(s, " in ")
	if i < 0 {
		return
	}
	names := strings.Split(s[:i], ",")
	v = strings.TrimSpace(names[0])
	if len(names) > 1 {
		idx = strings.TrimSpace(names[1])
	}
	list = strings.TrimSpace(s[i+4:])
	return v, idx, list, v != "" && list != "" && len(names) <= 2 && !strings.ContainsAny(v+idx, " (.")
}

// ParseArgs parses `name key=expr key2="str"`.
func ParseArgs(s string) (name string, args []Arg, err error) {
	parts := SplitArgs(s)
	if len(parts) == 0 {
		return "", nil, fmt.Errorf("component name expected")
	}
	name = parts[0]
	for _, part := range parts[1:] {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			k, v = part, "true"
		}
		e, err := expr.Parse(v)
		if err != nil {
			return "", nil, err
		}
		args = append(args, Arg{k, e})
	}
	return name, args, nil
}

// SplitArgs splits on whitespace outside quotes and brackets.
func SplitArgs(s string) []string {
	var out []string
	var q byte
	depth, start := 0, -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		if start < 0 && c != ' ' && c != '\t' && c != '\n' {
			start = i
		}
		switch {
		case q != 0:
			if c == '\\' {
				i++
			} else if c == q {
				q = 0
			}
		case c == '"' || c == '\'':
			q = c
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
		case (c == ' ' || c == '\t' || c == '\n') && depth <= 0 && start >= 0:
			out = append(out, s[start:i])
			start = -1
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}

func (t *Template) Render(env *expr.Env, ctx *Ctx) (string, error) {
	var b strings.Builder
	if err := renderAll(&b, t.nodes, env, ctx); err != nil {
		return "", fmt.Errorf("%s: %w", t.Name, err)
	}
	return b.String(), nil
}

func renderAll(b *strings.Builder, nodes []node, env *expr.Env, ctx *Ctx) error {
	for _, n := range nodes {
		if err := n.render(b, env, ctx); err != nil {
			return err
		}
	}
	return nil
}

// Escape renders a value for HTML output, leaving expr.HTML untouched.
func Escape(v any) string {
	if h, ok := v.(expr.HTML); ok {
		return string(h)
	}
	return html.EscapeString(expr.Str(v))
}

func (n textNode) render(b *strings.Builder, _ *expr.Env, _ *Ctx) error {
	b.WriteString(string(n))
	return nil
}

func (n outNode) render(b *strings.Builder, env *expr.Env, _ *Ctx) error {
	v, err := n.e.Eval(env)
	if err != nil {
		return err
	}
	b.WriteString(Escape(v))
	return nil
}

func (slotNode) render(b *strings.Builder, _ *expr.Env, ctx *Ctx) error {
	b.WriteString(ctx.Slot)
	return nil
}

func (n *ifNode) render(b *strings.Builder, env *expr.Env, ctx *Ctx) error {
	for _, br := range n.branches {
		if br.cond != nil {
			v, err := br.cond.Eval(env)
			if err != nil {
				return err
			}
			if !expr.Truthy(v) {
				continue
			}
		}
		return renderAll(b, br.body, env, ctx)
	}
	return nil
}

func (n *eachNode) render(b *strings.Builder, env *expr.Env, ctx *Ctx) error {
	v, err := n.list.Eval(env)
	if err != nil {
		return err
	}
	items := expr.List(v)
	if len(items) == 0 {
		return renderAll(b, n.empty, env, ctx)
	}
	for i, it := range items {
		vars := map[string]any{n.v: it}
		if n.idx != "" {
			vars[n.idx] = float64(i)
		}
		if err := renderAll(b, n.body, env.Child(vars), ctx); err != nil {
			return err
		}
	}
	return nil
}

func (n *useNode) render(b *strings.Builder, env *expr.Env, ctx *Ctx) error {
	if ctx.Component == nil {
		return fmt.Errorf("components are not available here")
	}
	props := map[string]any{}
	for _, a := range n.args {
		v, err := a.E.Eval(env)
		if err != nil {
			return err
		}
		props[a.Key] = v
	}
	var body strings.Builder
	if err := renderAll(&body, n.body, env, ctx); err != nil {
		return err
	}
	out, err := ctx.Component(n.name, props, body.String(), env)
	if err != nil {
		return err
	}
	b.WriteString(out)
	return nil
}
