// Package md is Hanji's markdown engine: a CommonMark-flavoured block and
// inline parser extended with directives (::: name … :::), build-time
// expressions ({{ … }}), live expressions ({ … }), action links
// ([label](@code)) and bound inputs ([? name]).
package md

import (
	"regexp"
	"strconv"
	"strings"
)

type Kind int

const (
	Para Kind = iota
	Heading
	Code
	Quote
	List
	Item
	HR
	HTML
	Table
	Row
	Directive // ::: name args … :::
	Leaf      // ::name args
)

type Node struct {
	Kind     Kind
	Level    int    // heading level; ordered list start
	Text     string // inline source, code, raw html; cell source
	Info     string // code fence info string; directive name
	Args     string // directive arguments, raw
	ID       string // explicit heading id
	Ordered  bool
	Tight    bool
	Task     int // 0 none, 1 unchecked, 2 checked
	Align    []string
	Children []*Node
	Line     int
}

var (
	fenceRe   = regexp.MustCompile("^( {0,3})(`{3,}|~{3,})[ \t]*(.*)$")
	dirOpenRe = regexp.MustCompile(`^(:{3,})[ \t]*([A-Za-z][\w-]*)[ \t]*(.*)$`)
	dirEndRe  = regexp.MustCompile(`^(:{3,})[ \t]*$`)
	leafRe    = regexp.MustCompile(`^::([A-Za-z][\w-]*)[ \t]*(.*)$`)
	headRe    = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$`)
	headIDRe  = regexp.MustCompile(`[ \t]*\{#([^\s{}]+)\}$`)
	hrRe      = regexp.MustCompile(`^ {0,3}(?:(?:\*[ \t]*){3,}|(?:-[ \t]*){3,}|(?:_[ \t]*){3,})$`)
	listRe    = regexp.MustCompile(`^( {0,3})([-*+]|\d{1,9}[.)])( +|$)(.*)$`)
	delimRe   = regexp.MustCompile(`^ {0,3}\|?[ \t]*:?-+:?[ \t]*(?:\|[ \t]*:?-+:?[ \t]*)*\|?[ \t]*$`)
	htmlRe    = regexp.MustCompile(`^ {0,3}<(?:!--|/?([A-Za-z][A-Za-z0-9-]*)(?:[\s/>]|$))`)
	taskRe    = regexp.MustCompile(`^\[([ xX])\][ \t]+`)
)

var blockTags = map[string]bool{}

func init() {
	for _, t := range strings.Fields("address article aside blockquote body canvas center dd details dialog div dl dt fieldset figcaption figure footer form h1 h2 h3 h4 h5 h6 header hr iframe li link main menu nav ol p pre script section source style summary svg table tbody td template textarea tfoot th thead tr ul video audio picture") {
		blockTags[t] = true
	}
}

// Parse turns markdown source into a block tree.
func Parse(src string) []*Node {
	src = strings.ReplaceAll(strings.ReplaceAll(src, "\r\n", "\n"), "\t", "    ")
	return parse(strings.Split(src, "\n"), 1)
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }

func indentOf(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }

func isHTMLStart(l string) bool {
	m := htmlRe.FindStringSubmatch(l)
	if m == nil {
		return false
	}
	return m[1] == "" || blockTags[strings.ToLower(m[1])] || strings.Contains(m[1], "-")
}

// startsBlock reports whether a line would interrupt a paragraph.
func startsBlock(l string) bool {
	if fenceRe.MatchString(l) || headRe.MatchString(l) || hrRe.MatchString(l) || isHTMLStart(l) {
		return true
	}
	t := strings.TrimLeft(l, " ")
	if strings.HasPrefix(t, ">") || strings.HasPrefix(t, "::") {
		return true
	}
	if m := listRe.FindStringSubmatch(l); m != nil && m[4] != "" {
		return len(m[2]) == 1 || m[2] == "1." || m[2] == "1)"
	}
	return false
}

func parse(lines []string, base int) []*Node {
	var out []*Node
	i := 0
	for i < len(lines) {
		l := lines[i]
		ln := base + i
		if blank(l) {
			i++
			continue
		}
		// fenced code
		if m := fenceRe.FindStringSubmatch(l); m != nil && !(m[2][0] == '`' && strings.Contains(m[3], "`")) {
			j, code := i+1, []string{}
			for ; j < len(lines); j++ {
				t := strings.TrimSpace(lines[j])
				if strings.HasPrefix(t, m[2]) && strings.Trim(t, m[2][:1]) == "" {
					break
				}
				code = append(code, cut(lines[j], len(m[1])))
			}
			out = append(out, &Node{Kind: Code, Info: strings.TrimSpace(m[3]), Text: strings.Join(code, "\n"), Line: ln})
			i = j + 1
			continue
		}
		// container directive
		if m := dirOpenRe.FindStringSubmatch(l); m != nil {
			end := directiveEnd(lines, i, len(m[1]))
			out = append(out, &Node{Kind: Directive, Info: m[2], Args: strings.TrimSpace(m[3]), Children: parse(lines[i+1:min(end, len(lines))], ln+1), Line: ln})
			i = end + 1
			continue
		}
		if m := leafRe.FindStringSubmatch(l); m != nil {
			out = append(out, &Node{Kind: Leaf, Info: m[1], Args: strings.TrimSpace(m[2]), Line: ln})
			i++
			continue
		}
		if m := headRe.FindStringSubmatch(l); m != nil {
			n := &Node{Kind: Heading, Level: len(m[1]), Text: m[2], Line: ln}
			if id := headIDRe.FindStringSubmatch(n.Text); id != nil {
				n.ID = id[1]
				n.Text = strings.TrimSuffix(n.Text, id[0])
			}
			out = append(out, n)
			i++
			continue
		}
		if hrRe.MatchString(l) {
			out = append(out, &Node{Kind: HR, Line: ln})
			i++
			continue
		}
		// blockquote
		if strings.HasPrefix(strings.TrimLeft(l, " "), ">") && indentOf(l) < 4 {
			var body []string
			j := i
			for ; j < len(lines); j++ {
				t := strings.TrimLeft(lines[j], " ")
				if strings.HasPrefix(t, ">") {
					body = append(body, strings.TrimPrefix(strings.TrimPrefix(t, ">"), " "))
				} else if !blank(lines[j]) && !startsBlock(lines[j]) && len(body) > 0 && !blank(body[len(body)-1]) {
					body = append(body, lines[j]) // lazy continuation
				} else {
					break
				}
			}
			out = append(out, &Node{Kind: Quote, Children: parse(body, ln), Line: ln})
			i = j
			continue
		}
		if m := listRe.FindStringSubmatch(l); m != nil {
			n, next := parseList(lines, i, base)
			out = append(out, n)
			i = next
			continue
		}
		if isHTMLStart(l) {
			j := htmlEnd(lines, i)
			out = append(out, &Node{Kind: HTML, Text: strings.Join(lines[i:j], "\n"), Line: ln})
			i = j
			continue
		}
		// table
		if strings.Contains(l, "|") && i+1 < len(lines) && delimRe.MatchString(lines[i+1]) && strings.Contains(lines[i+1], "-") {
			head := splitRow(l)
			delims := splitRow(lines[i+1])
			if len(head) == len(delims) {
				t := &Node{Kind: Table, Line: ln}
				for _, d := range delims {
					a := ""
					switch l, r := strings.HasPrefix(d, ":"), strings.HasSuffix(d, ":"); {
					case l && r:
						a = "center"
					case r:
						a = "right"
					case l:
						a = "left"
					}
					t.Align = append(t.Align, a)
				}
				t.Children = append(t.Children, row(head, len(head)))
				j := i + 2
				for ; j < len(lines) && !blank(lines[j]) && strings.Contains(lines[j], "|"); j++ {
					t.Children = append(t.Children, row(splitRow(lines[j]), len(head)))
				}
				out = append(out, t)
				i = j
				continue
			}
		}
		// paragraph
		j := i + 1
		for j < len(lines) && !blank(lines[j]) && !startsBlock(lines[j]) {
			j++
		}
		para := make([]string, 0, j-i)
		for _, pl := range lines[i:j] {
			para = append(para, strings.TrimLeft(pl, " "))
		}
		out = append(out, &Node{Kind: Para, Text: strings.TrimRight(strings.Join(para, "\n"), " \n"), Line: ln})
		i = j
	}
	return out
}

func cut(s string, n int) string {
	for n > 0 && strings.HasPrefix(s, " ") {
		s = s[1:]
		n--
	}
	return s
}

// directiveEnd finds the line closing the directive opened at lines[i].
// Nested directives are balanced with a stack; fenced code is skipped.
func directiveEnd(lines []string, i, colons int) int {
	stack := []int{colons}
	fence := ""
	for j := i + 1; j < len(lines); j++ {
		l := lines[j]
		if m := fenceRe.FindStringSubmatch(l); m != nil {
			t := strings.TrimSpace(l)
			if fence == "" {
				fence = m[2]
			} else if strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == "" {
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		t := strings.TrimSpace(l)
		if m := dirEndRe.FindStringSubmatch(t); m != nil {
			n := len(m[1])
			for len(stack) > 1 && stack[len(stack)-1] != n {
				stack = stack[:len(stack)-1]
			}
			if stack[len(stack)-1] == n {
				stack = stack[:len(stack)-1]
			}
			if len(stack) == 0 {
				return j
			}
		} else if m := dirOpenRe.FindStringSubmatch(t); m != nil {
			stack = append(stack, len(m[1]))
		}
	}
	return len(lines)
}

func htmlEnd(lines []string, i int) int {
	m := htmlRe.FindStringSubmatch(lines[i])
	tag := strings.ToLower(m[1])
	until := ""
	switch {
	case m[1] == "" && strings.Contains(lines[i], "<!--"):
		until = "-->"
	case tag == "script" || tag == "style" || tag == "pre" || tag == "textarea":
		until = "</" + tag + ">"
	}
	for j := i; j < len(lines); j++ {
		if until != "" {
			if strings.Contains(strings.ToLower(lines[j]), until) {
				return j + 1
			}
		} else if blank(lines[j]) {
			return j
		}
	}
	return len(lines)
}

func splitRow(l string) []string {
	l = strings.TrimSpace(l)
	l = strings.TrimPrefix(l, "|")
	if strings.HasSuffix(l, "|") && !strings.HasSuffix(l, `\|`) {
		l = l[:len(l)-1]
	}
	var cells []string
	var cur strings.Builder
	code := false
	for i := 0; i < len(l); i++ {
		c := l[i]
		switch {
		case c == '\\' && i+1 < len(l) && l[i+1] == '|':
			cur.WriteByte('|')
			i++
		case c == '`':
			code = !code
			cur.WriteByte(c)
		case c == '|' && !code:
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	return append(cells, strings.TrimSpace(cur.String()))
}

func row(cells []string, n int) *Node {
	r := &Node{Kind: Row}
	for i := 0; i < n; i++ {
		c := &Node{Kind: Para}
		if i < len(cells) {
			c.Text = cells[i]
		}
		r.Children = append(r.Children, c)
	}
	return r
}

func parseList(lines []string, i, base int) (*Node, int) {
	first := listRe.FindStringSubmatch(lines[i])
	ordered := len(first[2]) > 1
	marker := first[2][len(first[2])-1] // bullet char, or . / ) for ordered
	list := &Node{Kind: List, Ordered: ordered, Tight: true, Line: base + i}
	if ordered {
		list.Level, _ = strconv.Atoi(first[2][:len(first[2])-1])
	}
	sibling := func(l string) []string {
		m := listRe.FindStringSubmatch(l)
		if m == nil || (len(m[2]) > 1) != ordered || m[2][len(m[2])-1] != marker || hrRe.MatchString(l) {
			return nil
		}
		return m
	}
	for i < len(lines) {
		m := sibling(lines[i])
		if m == nil {
			break
		}
		pad := len(m[3])
		if pad > 4 || pad == 0 {
			pad = 1
		}
		w := len(m[1]) + len(m[2]) + pad
		body := []string{""}
		if len(lines[i]) > w {
			body[0] = lines[i][w:]
		}
		start := i
		i++
		for i < len(lines) {
			l := lines[i]
			if blank(l) {
				k := i
				for k < len(lines) && blank(lines[k]) {
					k++
				}
				if k < len(lines) && indentOf(lines[k]) >= w {
					for ; i < k; i++ {
						body = append(body, "")
					}
					continue
				}
				if k < len(lines) && sibling(lines[k]) != nil {
					list.Tight = false
				}
				i = k
				break
			}
			if indentOf(l) >= w {
				body = append(body, l[w:])
				i++
				continue
			}
			if listRe.MatchString(l) || startsBlock(l) || blank(body[len(body)-1]) {
				break
			}
			body = append(body, strings.TrimLeft(l, " ")) // lazy continuation
			i++
		}
		item := &Node{Kind: Item, Line: base + start}
		if t := taskRe.FindStringSubmatch(body[0]); t != nil {
			item.Task = 1
			if t[1] != " " {
				item.Task = 2
			}
			body[0] = body[0][len(t[0]):]
		}
		item.Children = parse(body, base+start)
		paras := 0
		for _, c := range item.Children {
			if c.Kind == Para {
				paras++
			}
		}
		if paras > 1 {
			list.Tight = false
		}
		list.Children = append(list.Children, item)
	}
	return list, i
}
