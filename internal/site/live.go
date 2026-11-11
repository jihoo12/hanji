package site

import (
	"sort"
	"strings"
)

// live compiles the reactive parts of one page. Markdown like
//
//	지금 {count}번 눌렀습니다. [하나 더](@count += 1)
//
// becomes plain HTML with data-hj-* markers plus a list of tiny JS functions
// that the client runtime (theme/assets/live.js) wires together.
type live struct {
	state   map[string]any
	derive  map[string]string
	persist bool
	scope   []string // loop variables of enclosing live `each` blocks
	fns     []string
	index   map[string]int
	used    bool
}

func (l *live) known(name string) bool {
	if _, ok := l.state[name]; ok {
		return true
	}
	if _, ok := l.derive[name]; ok {
		return true
	}
	for _, v := range l.scope {
		if v == name {
			return true
		}
	}
	return false
}

// scanJS walks the free identifiers of a JS snippet: names that are not
// property accesses, object keys, or inside strings.
func scanJS(src string, visit func(name string, start, end int)) {
	var prev byte // last significant byte
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '"' || c == '\'' || c == '`':
			j := i + 1
			for j < len(src) && src[j] != c {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			i, prev = j+1, c
		case c >= '0' && c <= '9':
			for i < len(src) && (isIdent(src[i]) || src[i] == '.') {
				i++
			}
			prev = '0'
		case isIdent(c):
			j := i
			for j < len(src) && isIdent(src[j]) {
				j++
			}
			k := j
			for k < len(src) && src[k] == ' ' {
				k++
			}
			member := prev == '.' && !strings.HasSuffix(strings.TrimRight(src[:i], " "), "...")
			key := k < len(src) && src[k] == ':' && (prev == '{' || prev == ',')
			if !member && !key {
				visit(src[i:j], i, j)
			}
			i, prev = j, 'a'
		default:
			if c != ' ' && c != '\n' && c != '\t' {
				prev = c
			}
			i++
		}
	}
}

func isIdent(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

// refs reports whether src mentions any live name.
func (l *live) refs(src string) bool {
	found := false
	scanJS(src, func(name string, _, _ int) { found = found || l.known(name) })
	return found
}

// rewrite prefixes live names with "$." so they resolve against the scope.
func (l *live) rewrite(src string) string {
	var b strings.Builder
	last := 0
	scanJS(src, func(name string, start, end int) {
		if l.known(name) {
			b.WriteString(src[last:start])
			b.WriteString("$.")
			b.WriteString(name)
			last = end
		}
	})
	b.WriteString(src[last:])
	return b.String()
}

// add registers a compiled function and returns its index.
func (l *live) add(fn string) int {
	l.used = true
	if i, ok := l.index[fn]; ok {
		return i
	}
	if l.index == nil {
		l.index = map[string]int{}
	}
	l.fns = append(l.fns, fn)
	l.index[fn] = len(l.fns) - 1
	return len(l.fns) - 1
}

func (l *live) getter(src string) int { return l.add("$=>(" + l.rewrite(src) + ")") }
func (l *live) setter(src string) int { return l.add("($,v)=>{" + l.rewrite(src) + "=v}") }
func (l *live) action(src string) int { return l.add("($,$event)=>{" + l.rewrite(src) + "}") }

// deriveJS renders the derive map as a JS object literal, keys sorted.
func (l *live) deriveJS() string {
	keys := make([]string, 0, len(l.derive))
	for k := range l.derive {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + ":$=>(" + l.rewrite(l.derive[k]) + ")"
	}
	return "{" + strings.Join(parts, ",") + "}"
}
