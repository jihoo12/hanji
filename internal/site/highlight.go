package site

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// A deliberately small build-time highlighter: one generic lexer driven by a
// per-language table. It knows comments, strings, numbers, keywords, calls
// and Capitalised type names — enough to make code readable with no client JS.

type langDef struct {
	kw    map[string]bool
	line  []string  // line comment starters
	block [2]string // block comment delimiters
	strs  string    // string delimiters
	multi string    // delimiters that may span lines
}

func words(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

var cLike = [2]string{"/*", "*/"}

var langs = map[string]*langDef{
	"go":     {kw: words("break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var nil true false iota"), line: []string{"//"}, block: cLike, strs: "\"'`", multi: "`"},
	"js":     {kw: words("async await break case catch class const continue default delete do else export extends finally for from function if import in instanceof let new of return static super switch this throw try typeof var void while yield null undefined true false interface type enum implements readonly as"), line: []string{"//"}, block: cLike, strs: "\"'`", multi: "`"},
	"rust":   {kw: words("as async await break const continue crate dyn else enum extern fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait type unsafe use where while true false"), line: []string{"//"}, block: cLike, strs: "\""},
	"python": {kw: words("and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield None True False self"), line: []string{"#"}, strs: "\"'"},
	"bash":   {kw: words("if then else elif fi for while do done case esac function in return export local cd echo exit set source"), line: []string{"#"}, strs: "\"'"},
	"json":   {kw: words("true false null"), strs: "\""},
	"yaml":   {kw: words("true false null"), line: []string{"#"}, strs: "\"'"},
	"css":    {kw: words("important media supports keyframes import from to"), block: cLike, strs: "\"'"},
	"c":      {kw: words("auto break case char const continue default do double else enum extern float for goto if inline int long register return short signed sizeof static struct switch typedef union unsigned void volatile while class namespace template public private protected virtual new delete this nullptr true false using"), line: []string{"//"}, block: cLike, strs: "\"'"},
	"java":   {kw: words("abstract boolean break byte case catch char class const continue default do double else enum extends final finally float for if implements import instanceof int interface long new package private protected public return short static super switch this throw throws try void volatile while var val fun when object null true false"), line: []string{"//"}, block: cLike, strs: "\"'"},
	"nix":    {kw: words("let in with rec inherit if then else assert import true false null"), line: []string{"#"}, block: cLike, strs: "\""},
}

var langAlias = map[string]string{
	"javascript": "js", "ts": "js", "typescript": "js", "jsx": "js", "tsx": "js", "mjs": "js",
	"golang": "go", "rs": "rust", "py": "python", "sh": "bash", "shell": "bash", "zsh": "bash",
	"yml": "yaml", "toml": "yaml", "scss": "css", "cpp": "c", "h": "c", "kotlin": "java", "kt": "java",
	"xml": "html", "svg": "html", "vue": "html", "svelte": "html",
}

var (
	titleRe = regexp.MustCompile(`title="([^"]*)"`)
	linesRe = regexp.MustCompile(`\{([\d,\s-]+)\}`)
)

// Highlight renders a fenced code block. The info string may carry a title
// and highlighted lines:  ```go title="main.go" {2,4-6}
func Highlight(info, code string) string {
	lang := ""
	if f := strings.Fields(info); len(f) > 0 && !strings.ContainsAny(f[0], `={"`) {
		lang = strings.ToLower(f[0])
	}
	marked := map[int]bool{}
	if m := linesRe.FindStringSubmatch(info); m != nil {
		for _, part := range strings.Split(m[1], ",") {
			a, b, isRange := strings.Cut(strings.TrimSpace(part), "-")
			from, err := strconv.Atoi(strings.TrimSpace(a))
			if err != nil {
				continue
			}
			to := from
			if isRange {
				if n, err := strconv.Atoi(strings.TrimSpace(b)); err == nil {
					to = n
				}
			}
			for i := from; i <= to && i < from+10000; i++ {
				marked[i] = true
			}
		}
	}
	key := lang
	if a, ok := langAlias[key]; ok {
		key = a
	}
	var body string
	switch def := langs[key]; {
	case key == "html":
		body = highlightHTML(code)
	case def != nil:
		body = highlightGeneric(code, def)
	default:
		body = html.EscapeString(code)
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<figure class="hj-code" data-lang="%s">`, html.EscapeString(lang))
	head := lang
	if m := titleRe.FindStringSubmatch(info); m != nil {
		head = m[1]
	}
	fmt.Fprintf(&b, `<figcaption><span>%s</span><button type="button" class="hj-copy" aria-label="코드 복사">복사</button></figcaption>`, html.EscapeString(head))
	b.WriteString("<pre><code>")
	for i, l := range strings.Split(body, "\n") {
		cls := "line"
		if marked[i+1] {
			cls = "line hl"
		}
		fmt.Fprintf(&b, "<span class=\"%s\">%s</span>\n", cls, l)
	}
	b.WriteString("</code></pre></figure>\n")
	return b.String()
}

// emit writes a token, closing and reopening its span at line breaks so the
// output can be split into lines safely.
func emit(b *strings.Builder, class, text string) {
	if text == "" {
		return
	}
	if class == "" {
		b.WriteString(html.EscapeString(text))
		return
	}
	for i, l := range strings.Split(text, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		if l != "" {
			fmt.Fprintf(b, `<span class="tk-%s">%s</span>`, class, html.EscapeString(l))
		}
	}
}

func isWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func highlightGeneric(code string, d *langDef) string {
	var b strings.Builder
	plain := 0
	flush := func(to int) {
		emit(&b, "", code[plain:to])
	}
	for i := 0; i < len(code); {
		c := code[i]
		rest := code[i:]
		end := -1
		class := ""
		switch {
		case d.block[0] != "" && strings.HasPrefix(rest, d.block[0]):
			class, end = "c", len(code)
			if j := strings.Index(rest[len(d.block[0]):], d.block[1]); j >= 0 {
				end = i + len(d.block[0]) + j + len(d.block[1])
			}
		case startsAny(rest, d.line) && (c != '#' || i == 0 || !isWordByte(code[i-1])):
			class, end = "c", len(code)
			if j := strings.IndexByte(rest, '\n'); j >= 0 {
				end = i + j
			}
		case strings.IndexByte(d.strs, c) >= 0:
			j := i + 1
			multi := strings.IndexByte(d.multi, c) >= 0
			for j < len(code) && code[j] != c && (multi || code[j] != '\n') {
				if code[j] == '\\' {
					j++
				}
				j++
			}
			if j < len(code) && code[j] == c {
				class, end = "s", j+1
			}
		case c >= '0' && c <= '9' && (i == 0 || !isWordByte(code[i-1])):
			j := i
			for j < len(code) && (isWordByte(code[j]) || code[j] == '.') {
				j++
			}
			class, end = "n", j
		case isWordByte(c) && (i == 0 || !isWordByte(code[i-1])):
			j := i
			for j < len(code) && (isWordByte(code[j]) || code[j] == '-' && d == langs["css"]) {
				j++
			}
			w := code[i:j]
			switch {
			case d.kw[w]:
				class = "k"
			case j < len(code) && code[j] == '(':
				class = "f"
			case c >= 'A' && c <= 'Z':
				class = "t"
			case (d == langs["json"] || d == langs["yaml"]) && j < len(code) && code[j] == ':':
				class = "p"
			}
			if class == "" {
				i = j
				continue
			}
			end = j
		}
		if end < 0 {
			i++
			continue
		}
		// JSON/YAML keys: a string directly followed by a colon
		if class == "s" && d == langs["json"] && strings.HasPrefix(strings.TrimLeft(code[end:], " "), ":") {
			class = "p"
		}
		flush(i)
		emit(&b, class, code[i:end])
		i, plain = end, end
	}
	flush(len(code))
	return b.String()
}

func startsAny(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

var htmlTokRe = regexp.MustCompile(`(?s)(<!--.*?-->)|(</?[A-Za-z][\w:-]*)|("[^"\n]*"|'[^'\n]*')|(\{\{.*?\}\})|(/?>)|([A-Za-z_:@-][\w:.-]*)(=)`)

func highlightHTML(code string) string {
	var b strings.Builder
	last := 0
	inTag := false
	for _, m := range htmlTokRe.FindAllStringSubmatchIndex(code, -1) {
		tok := code[m[0]:m[1]]
		class := ""
		switch {
		case m[2] >= 0:
			class = "c"
		case m[4] >= 0:
			class, inTag = "k", true
		case m[6] >= 0:
			if inTag {
				class = "s"
			}
		case m[8] >= 0:
			class = "f"
		case m[10] >= 0:
			if inTag {
				class = "k"
			}
			inTag = false
		case m[12] >= 0:
			if inTag {
				emit(&b, "", code[last:m[0]])
				emit(&b, "p", code[m[12]:m[13]])
				emit(&b, "", "=")
				last = m[1]
			}
			continue
		}
		emit(&b, "", code[last:m[0]])
		emit(&b, class, tok)
		last = m[1]
	}
	emit(&b, "", code[last:])
	return b.String()
}
