// Package yaml implements the small YAML subset Hanji uses for front matter
// and hanji.yml: block maps, block lists, flow lists/maps, block scalars (|),
// quoted and plain scalars. Numbers are float64, dates are time.Time.
package yaml

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type line struct {
	indent int
	text   string // trimmed, comment-free for structure lines
	raw    string
	blank  bool
	n      int
}

type parser struct {
	lines []line
	pos   int
}

// Parse parses a document whose top level is a map. Empty input gives an empty map.
func Parse(src string) (map[string]any, error) {
	p := &parser{}
	for i, raw := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		raw = strings.ReplaceAll(raw, "\t", "  ")
		t := strings.TrimSpace(raw)
		l := line{raw: raw, n: i + 1, text: t, indent: len(raw) - len(strings.TrimLeft(raw, " "))}
		l.blank = t == "" || strings.HasPrefix(t, "#")
		p.lines = append(p.lines, l)
	}
	p.skip()
	if p.pos >= len(p.lines) {
		return map[string]any{}, nil
	}
	v, err := p.node(p.lines[p.pos].indent)
	if err != nil {
		return nil, err
	}
	p.skip()
	if p.pos < len(p.lines) {
		return nil, fmt.Errorf("line %d: unexpected content %q", p.lines[p.pos].n, p.lines[p.pos].text)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("top level must be a map of `key: value` pairs")
	}
	return m, nil
}

func (p *parser) skip() {
	for p.pos < len(p.lines) && p.lines[p.pos].blank {
		p.pos++
	}
}

func isItem(t string) bool { return t == "-" || strings.HasPrefix(t, "- ") }

func (p *parser) node(indent int) (any, error) {
	if isItem(p.lines[p.pos].text) {
		return p.list(indent)
	}
	return p.mapping(indent)
}

func (p *parser) list(indent int) (any, error) {
	out := []any{}
	for {
		p.skip()
		if p.pos >= len(p.lines) {
			break
		}
		l := &p.lines[p.pos]
		if l.indent != indent || !isItem(l.text) {
			break
		}
		rest := strings.TrimSpace(l.text[1:])
		if rest == "" {
			p.pos++
			v, err := p.child(indent)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			continue
		}
		if _, _, ok := splitKey(rest); ok && rest[0] != '[' && rest[0] != '{' {
			// "- key: value" starts a map whose keys sit two columns in.
			off := len(l.text) - len(rest)
			l.indent = indent + off
			l.text = rest
			v, err := p.mapping(l.indent)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			continue
		}
		p.pos++
		v, err := scalar(rest, l.n)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// child parses a nested block more indented than parent, or returns nil.
func (p *parser) child(parent int) (any, error) {
	p.skip()
	if p.pos < len(p.lines) && p.lines[p.pos].indent > parent {
		return p.node(p.lines[p.pos].indent)
	}
	return nil, nil
}

func (p *parser) mapping(indent int) (any, error) {
	out := map[string]any{}
	for {
		p.skip()
		if p.pos >= len(p.lines) {
			break
		}
		l := p.lines[p.pos]
		if l.indent != indent || isItem(l.text) {
			if l.indent > indent {
				return nil, fmt.Errorf("line %d: unexpected indentation", l.n)
			}
			break
		}
		key, val, ok := splitKey(l.text)
		if !ok {
			return nil, fmt.Errorf("line %d: expected `key: value`, got %q", l.n, l.text)
		}
		p.pos++
		switch {
		case val == "|" || val == ">":
			out[key] = p.blockScalar(indent, val == ">")
		case val == "":
			p.skip()
			if p.pos < len(p.lines) && p.lines[p.pos].indent == indent && isItem(p.lines[p.pos].text) {
				v, err := p.list(indent)
				if err != nil {
					return nil, err
				}
				out[key] = v
				continue
			}
			v, err := p.child(indent)
			if err != nil {
				return nil, err
			}
			out[key] = v
		default:
			v, err := scalar(val, l.n)
			if err != nil {
				return nil, err
			}
			out[key] = v
		}
	}
	return out, nil
}

func (p *parser) blockScalar(indent int, fold bool) string {
	var b []string
	min := -1
	for p.pos < len(p.lines) {
		l := p.lines[p.pos]
		if strings.TrimSpace(l.raw) != "" {
			if l.indent <= indent {
				break
			}
			if min < 0 || l.indent < min {
				min = l.indent
			}
		}
		b = append(b, l.raw)
		p.pos++
	}
	for len(b) > 0 && strings.TrimSpace(b[len(b)-1]) == "" {
		b = b[:len(b)-1]
	}
	for i := range b {
		if len(b[i]) >= min && min > 0 {
			b[i] = b[i][min:]
		} else {
			b[i] = strings.TrimSpace(b[i])
		}
	}
	if fold {
		return strings.Join(b, " ")
	}
	return strings.Join(b, "\n")
}

// splitKey splits "key: value" at the first top-level ": ".
func splitKey(t string) (key, val string, ok bool) {
	if t == "" {
		return
	}
	if t[0] == '"' || t[0] == '\'' {
		end := strings.IndexByte(t[1:], t[0])
		if end < 0 {
			return
		}
		rest := strings.TrimSpace(t[end+2:])
		if !strings.HasPrefix(rest, ":") {
			return
		}
		return t[1 : end+1], strings.TrimSpace(stripComment(rest[1:])), true
	}
	for i := 0; i < len(t); i++ {
		if t[i] == ':' && (i+1 == len(t) || t[i+1] == ' ') {
			key = strings.TrimSpace(t[:i])
			if key == "" || strings.ContainsAny(key, "[]{}\"'") {
				return "", "", false
			}
			return key, strings.TrimSpace(stripComment(t[i+1:])), true
		}
	}
	return
}

// stripComment removes a trailing " # comment" outside quotes.
func stripComment(s string) string {
	var q byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case q != 0:
			if c == '\\' && q == '"' {
				i++
			} else if c == q {
				q = 0
			}
		case c == '"' || c == '\'':
			q = c
		case c == '#' && (i == 0 || s[i-1] == ' '):
			return s[:i]
		}
	}
	return s
}

func scalar(s string, n int) (any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if s[0] == '[' || s[0] == '{' {
		f := &flow{s: s}
		v, err := f.value()
		if err != nil {
			return nil, fmt.Errorf("line %d: %v", n, err)
		}
		f.ws()
		if f.i < len(f.s) {
			return nil, fmt.Errorf("line %d: unexpected %q", n, f.s[f.i:])
		}
		return v, nil
	}
	return plain(s), nil
}

func plain(s string) any {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
		return s[1 : len(s)-1]
	}
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'")
	}
	switch s {
	case "true":
		return true
	case "false":
		return false
	case "null", "~":
		return nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04", "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return s
}

type flow struct {
	s string
	i int
}

func (f *flow) ws() {
	for f.i < len(f.s) && (f.s[f.i] == ' ' || f.s[f.i] == '\n') {
		f.i++
	}
}

func (f *flow) value() (any, error) {
	f.ws()
	if f.i >= len(f.s) {
		return nil, fmt.Errorf("unexpected end of value")
	}
	switch f.s[f.i] {
	case '[':
		f.i++
		out := []any{}
		for {
			f.ws()
			if f.i < len(f.s) && f.s[f.i] == ']' {
				f.i++
				return out, nil
			}
			v, err := f.value()
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			f.ws()
			if f.i < len(f.s) && f.s[f.i] == ',' {
				f.i++
			} else if f.i >= len(f.s) || f.s[f.i] != ']' {
				return nil, fmt.Errorf("expected , or ] in list")
			}
		}
	case '{':
		f.i++
		out := map[string]any{}
		for {
			f.ws()
			if f.i < len(f.s) && f.s[f.i] == '}' {
				f.i++
				return out, nil
			}
			k := f.token(":")
			if f.i >= len(f.s) || f.s[f.i] != ':' {
				return nil, fmt.Errorf("expected : after key %q", k)
			}
			f.i++
			v, err := f.value()
			if err != nil {
				return nil, err
			}
			out[fmt.Sprint(plain(k))] = v
			f.ws()
			if f.i < len(f.s) && f.s[f.i] == ',' {
				f.i++
			} else if f.i >= len(f.s) || f.s[f.i] != '}' {
				return nil, fmt.Errorf("expected , or } in map")
			}
		}
	}
	return plain(f.token(",]}")), nil
}

// token reads a quoted string or a run of characters up to one of stop.
func (f *flow) token(stop string) string {
	f.ws()
	start := f.i
	if f.i < len(f.s) && (f.s[f.i] == '"' || f.s[f.i] == '\'') {
		q := f.s[f.i]
		f.i++
		for f.i < len(f.s) && f.s[f.i] != q {
			if f.s[f.i] == '\\' && q == '"' {
				f.i++
			}
			f.i++
		}
		f.i++
		if f.i > len(f.s) {
			f.i = len(f.s)
		}
		tok := f.s[start:f.i]
		f.ws()
		return tok
	}
	for f.i < len(f.s) && !strings.ContainsRune(stop, rune(f.s[f.i])) {
		f.i++
	}
	return strings.TrimSpace(f.s[start:f.i])
}
