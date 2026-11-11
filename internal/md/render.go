package md

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"
)

// Renderer turns a block tree into HTML. The hooks are where the host
// (package site) plugs in expressions, live bindings and components.
type Renderer struct {
	Eval      func(src string) string                   // {{ src }} → HTML
	Live      func(src string) (string, bool)           // { src } → HTML, or false to keep literal
	Action    func(labelHTML, code string) string       // [label](@code)
	Input     func(spec, onEnter string) (string, bool) // [? spec] or [? spec](@code)
	Link      func(href string) string                  // rewrite link targets
	Code      func(info, code string) string            // fenced code
	Directive func(n *Node) string                      // ::: and :: blocks
	Heading   func(level int, id, innerHTML string)     // observe headings (toc)
	Line      int                                       // source line of the block being rendered, for error messages
	ids       map[string]int
}

func (r *Renderer) Blocks(nodes []*Node) string {
	var b strings.Builder
	for _, n := range nodes {
		r.block(&b, n, false)
	}
	return b.String()
}

func esc(s string) string { return html.EscapeString(s) }

func (r *Renderer) block(b *strings.Builder, n *Node, tight bool) {
	if n.Line > 0 {
		r.Line = n.Line
	}
	switch n.Kind {
	case Para:
		if tight {
			b.WriteString(r.Inline(n.Text))
			b.WriteString("\n")
		} else {
			fmt.Fprintf(b, "<p>%s</p>\n", r.Inline(n.Text))
		}
	case Heading:
		inner := r.Inline(n.Text)
		id := n.ID
		if id == "" {
			id = Slug(stripTags(inner))
		}
		if id == "" {
			id = "section"
		}
		if r.ids == nil {
			r.ids = map[string]int{}
		}
		r.ids[id]++
		if c := r.ids[id]; c > 1 {
			id = fmt.Sprintf("%s-%d", id, c)
		}
		if r.Heading != nil {
			r.Heading(n.Level, id, inner)
		}
		anchor := ""
		if n.Level > 1 {
			anchor = fmt.Sprintf(`<a class="hj-anchor" href="#%s" aria-label="이 섹션 링크">#</a>`, esc(id))
		}
		fmt.Fprintf(b, "<h%d id=\"%s\">%s%s</h%d>\n", n.Level, esc(id), inner, anchor, n.Level)
	case Code:
		if r.Code != nil {
			b.WriteString(r.Code(n.Info, n.Text))
		} else {
			fmt.Fprintf(b, "<pre><code>%s</code></pre>\n", esc(n.Text))
		}
	case Quote:
		b.WriteString("<blockquote>\n")
		b.WriteString(r.Blocks(n.Children))
		b.WriteString("</blockquote>\n")
	case List:
		tag := "ul"
		if n.Ordered {
			tag = "ol"
		}
		attrs := ""
		if n.Ordered && n.Level != 1 {
			attrs = fmt.Sprintf(` start="%d"`, n.Level)
		}
		for _, it := range n.Children {
			if it.Task > 0 {
				attrs += ` class="hj-tasks"`
				break
			}
		}
		fmt.Fprintf(b, "<%s%s>\n", tag, attrs)
		for _, it := range n.Children {
			b.WriteString("<li>")
			if it.Task > 0 {
				checked := ""
				if it.Task == 2 {
					checked = " checked"
				}
				fmt.Fprintf(b, `<input type="checkbox" disabled%s> `, checked)
			}
			for _, c := range it.Children {
				r.block(b, c, n.Tight)
			}
			// trim the newline a tight paragraph leaves before </li>
			s := b.String()
			if strings.HasSuffix(s, "\n") {
				b.Reset()
				b.WriteString(s[:len(s)-1])
			}
			b.WriteString("</li>\n")
		}
		fmt.Fprintf(b, "</%s>\n", tag)
	case HR:
		b.WriteString("<hr>\n")
	case HTML:
		b.WriteString(r.interpolate(n.Text))
		b.WriteString("\n")
	case Table:
		b.WriteString("<div class=\"hj-table\"><table>\n")
		for i, row := range n.Children {
			cell := "td"
			if i == 0 {
				cell = "th"
				b.WriteString("<thead>\n")
			} else if i == 1 {
				b.WriteString("<tbody>\n")
			}
			b.WriteString("<tr>")
			for j, c := range row.Children {
				al := ""
				if j < len(n.Align) && n.Align[j] != "" {
					al = fmt.Sprintf(` style="text-align:%s"`, n.Align[j])
				}
				fmt.Fprintf(b, "<%s%s>%s</%s>", cell, al, r.Inline(c.Text), cell)
			}
			b.WriteString("</tr>\n")
			if i == 0 {
				b.WriteString("</thead>\n")
			}
		}
		if len(n.Children) > 1 {
			b.WriteString("</tbody>\n")
		}
		b.WriteString("</table></div>\n")
	case Directive, Leaf:
		if r.Directive != nil {
			b.WriteString(r.Directive(n))
		} else {
			b.WriteString(r.Blocks(n.Children))
		}
	}
}

// interpolate evaluates {{ }} inside raw HTML blocks.
func (r *Renderer) interpolate(s string) string {
	if r.Eval == nil || !strings.Contains(s, "{{") {
		return s
	}
	var b strings.Builder
	for {
		i := strings.Index(s, "{{")
		if i < 0 {
			break
		}
		if i > 0 && s[i-1] == '\\' {
			b.WriteString(s[:i-1])
			b.WriteString("{{")
			s = s[i+2:]
			continue
		}
		j := strings.Index(s[i:], "}}")
		if j < 0 {
			break
		}
		b.WriteString(s[:i])
		b.WriteString(r.Eval(strings.TrimSpace(s[i+2 : i+j])))
		s = s[i+j+2:]
	}
	b.WriteString(s)
	return b.String()
}

var (
	autoRe   = regexp.MustCompile(`^<((?:https?://|mailto:)[^\s<>]+)>`)
	rawTagRe = regexp.MustCompile(`^(?:</?[A-Za-z][A-Za-z0-9-]*(?:\s[^<>]*)?/?>|<!--[\s\S]*?-->)`)
	entityRe = regexp.MustCompile(`^&(?:[A-Za-z][A-Za-z0-9]{1,31}|#[0-9]{1,7}|#[xX][0-9A-Fa-f]{1,6});`)
	tagsRe   = regexp.MustCompile(`<[^>]*>`)
)

func stripTags(s string) string { return html.UnescapeString(tagsRe.ReplaceAllString(s, "")) }

// Slug makes a URL fragment from text, keeping letters of any script.
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, c := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(c)
		case unicode.IsSpace(c) || c == '-' || c == '/' || c == '.':
			dash = true
		}
	}
	return b.String()
}

func isPunct(c byte) bool {
	return c < 0x80 && (c >= '!' && c <= '/' || c >= ':' && c <= '@' || c >= '[' && c <= '`' || c >= '{' && c <= '~')
}

func isSpace(c byte) bool { return c == ' ' || c == '\n' || c == '\t' }

func isWord(c byte) bool {
	return c >= 0x80 || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// Inline renders inline markdown to HTML.
func (r *Renderer) Inline(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		switch c {
		case '\\':
			if i+1 < len(s) && s[i+1] == '\n' {
				b.WriteString("<br>\n")
				i += 2
				continue
			}
			if i+1 < len(s) && isPunct(s[i+1]) {
				b.WriteString(esc(s[i+1 : i+2]))
				i += 2
				continue
			}
			b.WriteByte(c)
			i++
		case '`':
			n := run(s, i)
			if j := codeEnd(s, i+n, n); j >= 0 {
				code := strings.ReplaceAll(s[i+n:j], "\n", " ")
				if len(code) > 2 && code[0] == ' ' && code[len(code)-1] == ' ' && strings.TrimSpace(code) != "" {
					code = code[1 : len(code)-1]
				}
				fmt.Fprintf(&b, "<code>%s</code>", esc(code))
				i = j + n
				continue
			}
			b.WriteString(s[i : i+n])
			i += n
		case '{':
			if strings.HasPrefix(s[i:], "{{") && r.Eval != nil {
				if j := strings.Index(s[i:], "}}"); j > 0 {
					b.WriteString(r.Eval(strings.TrimSpace(s[i+2 : i+j])))
					i += j + 2
					continue
				}
			} else if r.Live != nil {
				if j := match(s, i, '{', '}'); j > 0 {
					if h, ok := r.Live(strings.TrimSpace(s[i+1 : j])); ok {
						b.WriteString(h)
						i = j + 1
						continue
					}
				}
			}
			b.WriteByte(c)
			i++
		case '!', '[':
			img := c == '!'
			if img && (i+1 >= len(s) || s[i+1] != '[') {
				b.WriteByte(c)
				i++
				continue
			}
			if h, next, ok := r.bracket(s, i, img); ok {
				b.WriteString(h)
				i = next
				continue
			}
			b.WriteByte(c)
			i++
		case '<':
			if m := autoRe.FindStringSubmatch(s[i:]); m != nil {
				fmt.Fprintf(&b, `<a href="%s">%s</a>`, esc(m[1]), esc(strings.TrimPrefix(m[1], "mailto:")))
				i += len(m[0])
			} else if m := rawTagRe.FindString(s[i:]); m != "" {
				b.WriteString(m)
				i += len(m)
			} else {
				b.WriteString("&lt;")
				i++
			}
		case '>':
			b.WriteString("&gt;")
			i++
		case '&':
			if m := entityRe.FindString(s[i:]); m != "" {
				b.WriteString(m)
				i += len(m)
			} else {
				b.WriteString("&amp;")
				i++
			}
		case '*', '_', '~', '=':
			n := run(s, i)
			max, min := 3, 1
			if c == '~' || c == '=' {
				max, min = 2, 2
			}
			if n > max {
				b.WriteString(s[i : i+n])
				i += n
				continue
			}
			open := n >= min && i+n < len(s) && !isSpace(s[i+n]) && (c != '_' || i == 0 || !isWord(s[i-1]))
			if open {
				if j := closer(s, i+n, s[i:i+n]); j > 0 {
					inner := r.Inline(s[i+n : j])
					switch {
					case c == '~':
						fmt.Fprintf(&b, "<del>%s</del>", inner)
					case c == '=':
						fmt.Fprintf(&b, "<mark>%s</mark>", inner)
					case n == 1:
						fmt.Fprintf(&b, "<em>%s</em>", inner)
					case n == 2:
						fmt.Fprintf(&b, "<strong>%s</strong>", inner)
					default:
						fmt.Fprintf(&b, "<em><strong>%s</strong></em>", inner)
					}
					i = j + n
					continue
				}
			}
			b.WriteString(s[i : i+n])
			i += n
		case '\n':
			if cur := b.String(); strings.HasSuffix(cur, "  ") {
				b.Reset()
				b.WriteString(strings.TrimRight(cur, " "))
				b.WriteString("<br>")
			}
			b.WriteByte('\n')
			i++
		case 'h':
			if (i == 0 || isSpace(s[i-1]) || s[i-1] == '(') && (strings.HasPrefix(s[i:], "http://") || strings.HasPrefix(s[i:], "https://")) {
				j := i
				for j < len(s) && !isSpace(s[j]) && s[j] != '<' {
					j++
				}
				for j > i && strings.ContainsRune(".,;:!?)", rune(s[j-1])) {
					j--
				}
				fmt.Fprintf(&b, `<a href="%s">%s</a>`, esc(s[i:j]), esc(s[i:j]))
				i = j
				continue
			}
			b.WriteByte(c)
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

func run(s string, i int) int {
	n := 0
	for i+n < len(s) && s[i+n] == s[i] {
		n++
	}
	return n
}

// codeEnd finds a closing backtick run of exactly n ticks.
func codeEnd(s string, from, n int) int {
	for j := from; j < len(s); j++ {
		if s[j] == '`' {
			m := run(s, j)
			if m == n {
				return j
			}
			j += m - 1
		}
	}
	return -1
}

// match finds the bracket closing the one at s[i], skipping escapes, code
// spans and (for braces and parens) quoted strings.
func match(s string, i int, open, close byte) int {
	depth := 0
	var q byte
	for j := i; j < len(s); j++ {
		c := s[j]
		switch {
		case q != 0:
			if c == '\\' {
				j++
			} else if c == q {
				q = 0
			}
		case c == '\\':
			j++
		case c == '`' && open == '[':
			n := run(s, j)
			if e := codeEnd(s, j+n, n); e >= 0 {
				j = e + n - 1
			} else {
				j += n - 1
			}
		case (c == '"' || c == '\'' || c == '`') && open != '[':
			// an apostrophe inside a word (don't) is not a string quote
			if c == '\'' && j > 0 && isWord(s[j-1]) && j+1 < len(s) && isWord(s[j+1]) {
				continue
			}
			q = c
		case c == open:
			depth++
		case c == close:
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

func closer(s string, from int, delim string) int {
	ch, n := delim[0], len(delim)
	for k := from; k < len(s); k++ {
		c := s[k]
		switch {
		case c == '\\':
			k++
		case c == '`':
			m := run(s, k)
			if e := codeEnd(s, k+m, m); e >= 0 {
				k = e + m - 1
			} else {
				k += m - 1
			}
		case c == ch:
			m := run(s, k)
			switch {
			case m < n:
				k += m - 1
			case m == 2 && n == 1:
				// a nested ** … ** pair; hop over it
				if e := closer(s, k+2, s[k:k+2]); e > 0 {
					k = e + 1
				} else {
					k++
				}
			case k > from && !isSpace(s[k-1]) && (ch != '_' || k+m >= len(s) || !isWord(s[k+m])):
				return k + m - n
			default:
				k += m - 1
			}
		}
	}
	return -1
}

// bracket handles [text](href), ![alt](src), [label](@action) and [? input].
func (r *Renderer) bracket(s string, i int, img bool) (string, int, bool) {
	start := i
	if img {
		start++
	}
	end := match(s, start, '[', ']')
	if end < 0 {
		return "", 0, false
	}
	label := s[start+1 : end]
	if !img && strings.HasPrefix(label, "?") && r.Input != nil {
		next, enter := end+1, ""
		if next < len(s) && s[next] == '(' {
			if pe := match(s, next, '(', ')'); pe > 0 && strings.HasPrefix(strings.TrimSpace(s[next+1:pe]), "@") {
				enter, next = strings.TrimSpace(strings.TrimSpace(s[next+1 : pe])[1:]), pe+1
			}
		}
		if h, ok := r.Input(strings.TrimSpace(label[1:]), enter); ok {
			return h, next, true
		}
	}
	if end+1 >= len(s) || s[end+1] != '(' {
		return "", 0, false
	}
	pend := match(s, end+1, '(', ')')
	if pend < 0 {
		return "", 0, false
	}
	dest := strings.TrimSpace(s[end+2 : pend])
	if !img && strings.HasPrefix(dest, "@") && r.Action != nil {
		return r.Action(r.Inline(label), strings.TrimSpace(dest[1:])), pend + 1, true
	}
	title := ""
	if strings.HasSuffix(dest, `"`) {
		if q := strings.Index(dest, ` "`); q > 0 {
			title = fmt.Sprintf(` title="%s"`, esc(dest[q+2:len(dest)-1]))
			dest = strings.TrimSpace(dest[:q])
		}
	}
	dest = strings.Trim(dest, "<>")
	if strings.Contains(dest, "{{") {
		dest = html.UnescapeString(r.interpolate(dest))
	}
	if r.Link != nil {
		dest = r.Link(dest)
	}
	if img {
		return fmt.Sprintf(`<img src="%s" alt="%s"%s loading="lazy">`, esc(dest), esc(stripTags(r.Inline(label))), title), pend + 1, true
	}
	return fmt.Sprintf(`<a href="%s"%s>%s</a>`, esc(dest), title, r.Inline(label)), pend + 1, true
}
