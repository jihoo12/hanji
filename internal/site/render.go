package site

import (
	"encoding/json"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"hanji/internal/expr"
	"hanji/internal/md"
	"hanji/internal/tmpl"
)

// rctx is the state of rendering one page.
type rctx struct {
	s    *Site
	p    *Page
	root *expr.Env // page scope
	env  *expr.Env // current scope (root, or inside a build-time each)
	live *live
	r    *md.Renderer
	used []string // components used, in first-use order
	seen map[string]bool
	toc  []any
}

func (rc *rctx) errf(format string, a ...any) {
	loc := "pages/" + rc.p.Src
	if rc.r != nil && rc.r.Line > 0 {
		loc += ":" + strconv.Itoa(rc.r.Line)
	}
	rc.s.errf("%s: %s", loc, fmt.Sprintf(format, a...))
}

var callouts = map[string]string{"note": "note", "info": "note", "tip": "tip", "warn": "warn", "warning": "warn", "danger": "danger"}

func (s *Site) render(res *Result) {
	type entry struct {
		URL      string   `json:"url"`
		Title    string   `json:"title"`
		Section  string   `json:"section,omitempty"`
		Headings []string `json:"headings,omitempty"`
		Text     string   `json:"text"`
	}
	var index []entry
	for _, p := range s.Pages {
		out, text, toc := s.renderPage(p)
		res.Files[p.Out] = File{Data: []byte(out)}
		if p.ID == "404" || expr.Truthy(p.FM["unlisted"]) {
			continue
		}
		e := entry{URL: p.URL, Title: expr.Str(p.Data["title"]), Section: expr.Str(p.Data["section"])}
		for _, h := range toc {
			e.Headings = append(e.Headings, expr.Str(h.(map[string]any)["text"]))
		}
		if r := []rune(text); len(r) > 4000 {
			text = string(r[:4000])
		}
		e.Text = text
		index = append(index, e)
	}
	if b, err := json.Marshal(index); err == nil {
		res.Files["_hanji/search.json"] = File{Data: b}
	}
	// theme assets, then the project's own files (which may override them)
	if entries, err := fs.ReadDir(s.opt.Theme, "assets"); err == nil {
		for _, e := range entries {
			if b, err := fs.ReadFile(s.opt.Theme, "assets/"+e.Name()); err == nil {
				res.Files["_hanji/"+e.Name()] = File{Data: b}
			}
		}
	}
	for out, src := range s.static {
		res.Files[out] = File{Path: src}
	}
	pub := filepath.Join(s.opt.Dir, "public")
	filepath.WalkDir(pub, func(file string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(pub, file)
			res.Files[filepath.ToSlash(rel)] = File{Path: file}
		}
		return nil
	})
	if site := expr.Str(s.Config["url"]); site != "" {
		s.writeFeeds(res, site)
	}
}

func (s *Site) writeFeeds(res *Result, site string) {
	var sm, rss strings.Builder
	sm.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	fmt.Fprintf(&rss, `<?xml version="1.0" encoding="UTF-8"?>`+"\n"+`<rss version="2.0"><channel><title>%s</title><link>%s%s</link><description>%s</description>`+"\n",
		html.EscapeString(expr.Str(s.Config["title"])), site, s.Base, html.EscapeString(expr.Str(s.Config["description"])))
	items := 0
	for _, p := range s.Pages {
		if p.ID == "404" || expr.Truthy(p.FM["unlisted"]) {
			continue
		}
		fmt.Fprintf(&sm, "<url><loc>%s%s</loc></url>\n", site, html.EscapeString(p.URL))
		if d, ok := p.Data["date"].(time.Time); ok && items < 50 {
			items++
			fmt.Fprintf(&rss, "<item><title>%s</title><link>%s%s</link><guid>%s%s</guid><pubDate>%s</pubDate><description>%s</description></item>\n",
				html.EscapeString(expr.Str(p.Data["title"])), site, p.URL, site, p.URL, d.Format(time.RFC1123Z), html.EscapeString(expr.Str(p.Data["description"])))
		}
	}
	sm.WriteString("</urlset>\n")
	rss.WriteString("</channel></rss>\n")
	res.Files["sitemap.xml"] = File{Data: []byte(sm.String())}
	if items > 0 {
		res.Files["feed.xml"] = File{Data: []byte(rss.String())}
	}
}

// renderPage returns the full HTML document, its plain text, and its toc.
func (s *Site) renderPage(p *Page) (string, string, []any) {
	rc := &rctx{s: s, p: p, seen: map[string]bool{}, toc: []any{}}
	rc.root = s.env(p)
	rc.env = rc.root
	rc.live = &live{state: map[string]any{}, derive: map[string]string{}, persist: expr.Truthy(p.FM["persist"])}
	if m, ok := p.FM["state"].(map[string]any); ok {
		rc.live.state = m
	}
	if m, ok := p.FM["derive"].(map[string]any); ok {
		for k, v := range m {
			rc.live.derive[k] = expr.Str(v)
		}
	}
	ctx := &tmpl.Ctx{Component: rc.component}

	var body string
	if p.IsHTML {
		t, err := tmpl.Parse("pages/"+p.Src, p.body)
		if err == nil {
			body, err = t.Render(rc.root, ctx)
		}
		if err != nil {
			s.errf("%v", err)
		}
	} else {
		rc.r = &md.Renderer{
			Eval: rc.eval, Live: rc.liveText, Action: rc.liveAction, Input: rc.liveInput,
			Link: rc.link, Code: Highlight, Directive: rc.directive,
			Heading: func(level int, id, inner string) {
				if level == 2 || level == 3 {
					rc.toc = append(rc.toc, map[string]any{"id": id, "level": float64(level), "text": expr.StripTags(inner)})
				}
			},
		}
		nodes := p.nodes
		if l := s.layouts[expr.Str(p.Data["layout"])]; p.h1 != nil && (l == nil || l.OwnTitle) {
			nodes = append([]*md.Node{p.h1}, nodes...)
		}
		body = rc.r.Blocks(nodes)
		rc.r.Line = 0
	}
	// plain text for the search index: drop code captions, keep word breaks
	text := captionRe.ReplaceAllString(body, "")
	text = strings.Join(strings.Fields(expr.StripTags(strings.ReplaceAll(text, "<", " <"))), " ")
	if rc.live.used {
		state, _ := json.Marshal(rc.live.state)
		fns := strings.ReplaceAll(strings.Join(rc.live.fns, ","), "</", `<\/`)
		body = fmt.Sprintf("<div data-hj-live>\n%s</div>\n<script type=\"module\">import{mount}from%q;mount({state:%s,derive:%s,persist:%t,fns:[%s]})</script>\n",
			body, s.Base+"_hanji/live.js", state, strings.ReplaceAll(rc.live.deriveJS(), "</", `<\/`), rc.live.persist, fns)
	}
	p.Data["toc"] = rc.toc

	out := body
	name := expr.Str(p.Data["layout"])
	for depth := 0; name != "" && name != "none"; depth++ {
		l := s.layouts[name]
		if l == nil || depth > 8 {
			s.errf("pages/%s: layout %q not found (looked in layouts/%s.html and the built-in theme)", p.Src, name, name)
			break
		}
		ctx.Slot = out
		var err error
		if out, err = l.Tmpl.Render(rc.root, ctx); err != nil {
			s.errf("pages/%s: %v", p.Src, err)
			break
		}
		name = l.Layout
	}

	var css, js strings.Builder
	for _, n := range rc.used {
		c := s.components[n]
		if c.Style != "" {
			css.WriteString(c.Style)
		}
		if c.Script != "" {
			fmt.Fprintf(&js, "{\n%s}\n", c.Script)
		}
	}
	if css.Len() > 0 {
		out = insertBefore(out, "</head>", "<style data-hj-components>\n"+css.String()+"</style>\n")
	}
	tail := ""
	if js.Len() > 0 {
		tail += "<script type=\"module\">\n" + js.String() + "</script>\n"
	}
	if s.opt.Dev {
		tail += fmt.Sprintf("<script>window.__hjReload||(window.__hjReload=new EventSource(%q)).addEventListener(\"message\",()=>location.reload())</script>\n", s.Base+"_hanji/reload")
	}
	if tail != "" {
		out = insertBefore(out, "</body>", tail)
	}
	return out, text, rc.toc
}

func insertBefore(doc, marker, s string) string {
	if i := strings.LastIndex(doc, marker); i >= 0 {
		return doc[:i] + s + doc[i:]
	}
	return doc + s
}

// component renders components/<name>.html with props and a body.
func (rc *rctx) component(name string, props map[string]any, body string, _ *expr.Env) (string, error) {
	c := rc.s.components[name]
	if c == nil {
		return "", fmt.Errorf("unknown component %q — create components/%s.html", name, name)
	}
	if !rc.seen[name] {
		rc.seen[name] = true
		rc.used = append(rc.used, name)
	}
	vars := map[string]any{"props": props, "slot": expr.HTML(body)}
	for k, v := range props {
		vars[k] = v
	}
	// Components see the page scope plus their own props — never the caller's
	// locals — so a missing prop is empty instead of leaking from outside.
	return c.Tmpl.Render(rc.root.Child(vars), &tmpl.Ctx{Slot: body, Component: rc.component})
}

func (rc *rctx) eval(src string) string {
	v, err := expr.Eval(src, rc.env)
	if err != nil {
		rc.errf("%v", err)
		return ""
	}
	return tmpl.Escape(v)
}

var captionRe = regexp.MustCompile(`(?s)<figcaption>.*?</figcaption>|<template>|</template>`)

var identRe = regexp.MustCompile(`^[A-Za-z_$][\w$]*$`)

func (rc *rctx) liveText(src string) (string, bool) {
	if src == "" || !rc.live.refs(src) {
		return "", false
	}
	initial := ""
	if v, ok := rc.live.state[src]; ok && identRe.MatchString(src) {
		initial = html.EscapeString(expr.Str(v))
	}
	return fmt.Sprintf(`<span data-hj-text="%d">%s</span>`, rc.live.getter(src), initial), true
}

func (rc *rctx) liveAction(label, code string) string {
	return fmt.Sprintf(`<button type="button" class="hj-action" data-hj-on="%d">%s</button>`, rc.live.action(code), label)
}

var rangeRe = regexp.MustCompile(`^(-?[\d.]+)\s*\.\.\s*(-?[\d.]+)(?:\s+step\s+([\d.]+))?$`)

// liveInput renders [? name | hint] as a control bound to live state.
// [? name](@code) also runs code when Enter is pressed.
func (rc *rctx) liveInput(spec, onEnter string) (string, bool) {
	name, hint, _ := strings.Cut(spec, "|")
	name, hint = strings.TrimSpace(name), strings.TrimSpace(hint)
	if name == "" || !rc.live.refs(name) {
		return "", false
	}
	bind := fmt.Sprintf(`class="hj-input" data-hj-model="%d" data-hj-set="%d"`, rc.live.getter(name), rc.live.setter(name))
	if onEnter != "" {
		bind += fmt.Sprintf(` data-hj-enter="%d"`, rc.live.action(onEnter))
	}
	init := rc.live.state[name]
	_, isBool := init.(bool)
	_, isNum := init.(float64)
	switch m := rangeRe.FindStringSubmatch(hint); {
	case isBool || hint == "check":
		return fmt.Sprintf(`<input type="checkbox" %s>`, bind), true
	case m != nil:
		step := ""
		if m[3] != "" {
			step = ` step="` + m[3] + `"`
		}
		return fmt.Sprintf(`<input type="range" min="%s" max="%s"%s %s>`, m[1], m[2], step, bind), true
	case strings.Contains(hint, " / "):
		var b strings.Builder
		fmt.Fprintf(&b, `<select %s>`, bind)
		for _, o := range strings.Split(hint, " / ") {
			fmt.Fprintf(&b, `<option>%s</option>`, html.EscapeString(strings.TrimSpace(o)))
		}
		b.WriteString(`</select>`)
		return b.String(), true
	case isNum:
		return fmt.Sprintf(`<input type="number" %s placeholder="%s">`, bind, html.EscapeString(hint)), true
	}
	return fmt.Sprintf(`<input type="text" %s placeholder="%s">`, bind, html.EscapeString(hint)), true
}

var schemeRe = regexp.MustCompile(`^(?:[a-zA-Z][a-zA-Z0-9+.-]*:|//|#)`)

// link resolves markdown link targets: /abs gets the base path, relative
// .md files become page URLs, and other relative files follow their source.
func (rc *rctx) link(href string) string {
	if href == "" || schemeRe.MatchString(href) {
		return href
	}
	if strings.HasPrefix(href, "/") {
		return rc.s.url(href)
	}
	target, frag, _ := strings.Cut(href, "#")
	if frag != "" {
		frag = "#" + frag
	}
	rel := path.Join(path.Dir(rc.p.Src), target)
	if strings.HasSuffix(target, ".md") {
		if p := rc.s.bySrc[rel]; p != nil && p.Data != nil {
			return p.URL + frag
		}
		rc.errf("broken link: %s (no such page pages/%s)", href, rel)
		return href
	}
	// strip ordering prefixes the same way routes do
	segs := strings.Split(rel, "/")
	for i, seg := range segs {
		if m := orderRe.FindStringSubmatch(seg); m != nil {
			segs[i] = m[2]
		}
	}
	return rc.s.url("/"+strings.Join(segs, "/")) + frag
}

// props parses directive arguments: key="str" key={expr} key=bare, and any
// other words become the `title` prop.
func (rc *rctx) props(args string) map[string]any {
	props := map[string]any{}
	var title []string
	for _, tok := range tmpl.SplitArgs(args) {
		k, v, ok := strings.Cut(tok, "=")
		if !ok || !identRe.MatchString(strings.ReplaceAll(k, "-", "_")) {
			title = append(title, tok)
			continue
		}
		switch {
		case len(v) >= 2 && v[0] == '{' && v[len(v)-1] == '}':
			val, err := expr.Eval(v[1:len(v)-1], rc.env)
			if err != nil {
				rc.errf("%v", err)
			}
			props[k] = val
		case len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0]:
			props[k] = v[1 : len(v)-1]
		case v == "true" || v == "false":
			props[k] = v == "true"
		default:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				props[k] = f
			} else {
				props[k] = v
			}
		}
	}
	if len(title) > 0 {
		t := strings.Join(title, " ")
		if len(t) >= 2 && (t[0] == '"' || t[0] == '\'') && t[len(t)-1] == t[0] {
			t = t[1 : len(t)-1]
		}
		props["title"] = t
	}
	return props
}

func (rc *rctx) directive(n *md.Node) string {
	line := rc.r.Line
	switch n.Info {
	case "if":
		if rc.live.refs(n.Args) {
			return fmt.Sprintf("<div data-hj-if=\"%d\" hidden>\n%s</div>\n", rc.live.getter(n.Args), rc.r.Blocks(n.Children))
		}
		v, err := expr.Eval(n.Args, rc.env)
		if err != nil {
			rc.errf("%v", err)
		}
		if expr.Truthy(v) {
			return rc.r.Blocks(n.Children)
		}
		return ""
	case "each":
		v, idx, listSrc, ok := tmpl.SplitEach(n.Args)
		if !ok {
			rc.errf("expected `::: each item in list`")
			return ""
		}
		if rc.live.refs(listSrc) {
			i := rc.live.getter(listSrc)
			depth := len(rc.live.scope)
			rc.live.scope = append(rc.live.scope, v)
			if idx != "" {
				rc.live.scope = append(rc.live.scope, idx)
			}
			body := rc.r.Blocks(n.Children)
			rc.live.scope = rc.live.scope[:depth]
			return fmt.Sprintf("<div data-hj-each=\"%d\" data-hj-as=\"%s\" data-hj-index=\"%s\"><template>%s</template></div>\n", i, v, idx, body)
		}
		list, err := expr.Eval(listSrc, rc.env)
		if err != nil {
			rc.errf("%v", err)
			return ""
		}
		var b strings.Builder
		outer := rc.env
		for i, item := range expr.List(list) {
			vars := map[string]any{v: item}
			if idx != "" {
				vars[idx] = float64(i)
			}
			rc.env = outer.Child(vars)
			b.WriteString(rc.r.Blocks(n.Children))
		}
		rc.env = outer
		// one list item per iteration reads as a single list
		return strings.NewReplacer("</ul>\n<ul>\n", "", "</ol>\n<ol>\n", "").Replace(b.String())
	}
	props := rc.props(n.Args)
	name := n.Info
	if kind, ok := callouts[name]; ok {
		name = "callout"
		props["kind"] = kind
	}
	body := rc.r.Blocks(n.Children)
	out, err := rc.component(name, props, body, nil)
	if err != nil {
		rc.r.Line = line
		rc.errf("%v", err)
		return ""
	}
	return out + "\n"
}

const outMarker = ".hanji-out"

// Write puts a build on disk, replacing dir. It refuses to replace a
// non-empty directory that an earlier build did not create.
func (r *Result) Write(dir string) error {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		if _, err := os.Stat(filepath.Join(dir, outMarker)); err != nil {
			return fmt.Errorf("%s is not empty and was not created by hanji; refusing to overwrite it", dir)
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, outMarker), []byte("built by hanji\n"), 0o644); err != nil {
		return err
	}
	for name, f := range r.Files {
		dst := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		data := f.Data
		if f.Path != "" {
			var err error
			if data, err = os.ReadFile(f.Path); err != nil {
				return err
			}
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
