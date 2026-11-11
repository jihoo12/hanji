// Package site loads a Hanji project from disk and builds it into files.
package site

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"hanji/internal/expr"
	"hanji/internal/md"
	"hanji/internal/tmpl"
	"hanji/internal/yaml"
)

const Version = "0.1.0"

// Options control a build.
type Options struct {
	Dir   string // project root
	Theme fs.FS  // built-in theme: layouts/, components/, assets/
	Dev   bool   // include drafts, inject live reload
}

// File is one output file: either bytes in memory or a path to copy from.
type File struct {
	Data []byte
	Path string
}

// Result is a finished build, keyed by output path ("docs/index.html").
type Result struct {
	Files map[string]File
	Pages int
	Base  string // base path the site was built for
}

type Page struct {
	Src    string // path under pages/, slash separated
	ID     string // "docs/intro"; "" for the home page
	URL    string // public URL including the base path
	Out    string // output file
	FM     map[string]any
	Data   map[string]any // what templates see as `page`
	Vars   map[string]any // variables from `expand`
	IsHTML bool
	index  bool
	body   string
	nodes  []*md.Node
	h1     *md.Node // the leading "# Title", lifted out of the body
	order  float64
}

type Component struct {
	Name   string
	Tmpl   *tmpl.Template
	Style  string
	Script string
	Layout string // parent layout, for layouts
	// OwnTitle is set by `title: false` in a layout's front matter: the layout
	// does not print page.title, so the document keeps its leading "# Title".
	OwnTitle bool
}

type Site struct {
	opt        Options
	Config     map[string]any
	Base       string // "/" or "/sub/"
	Pages      []*Page
	bySrc      map[string]*Page
	layouts    map[string]*Component
	components map[string]*Component
	dirOrder   map[string]float64
	sidebars   map[string][]*navNode
	static     map[string]string // output path → source file
	strings    map[string]any    // interface text of the theme, for site.lang
	errs       []error
}

func (s *Site) errf(format string, a ...any) {
	s.errs = append(s.errs, fmt.Errorf(format, a...))
}

// Build loads the project at opt.Dir and renders every page.
func Build(opt Options) (*Result, error) {
	s := &Site{opt: opt, bySrc: map[string]*Page{}, layouts: map[string]*Component{}, components: map[string]*Component{},
		dirOrder: map[string]float64{}, static: map[string]string{}}
	if err := s.loadConfig(); err != nil {
		return nil, err
	}
	s.loadStrings()
	s.loadTemplates()
	s.loadPages()
	res := &Result{Files: map[string]File{}, Pages: len(s.Pages), Base: s.Base}
	if len(s.errs) == 0 {
		s.render(res)
	}
	if len(s.errs) > 0 {
		return nil, errors.Join(s.errs...)
	}
	return res, nil
}

func (s *Site) loadConfig() error {
	s.Config = map[string]any{}
	for _, name := range []string{"hanji.yml", "hanji.yaml"} {
		b, err := os.ReadFile(filepath.Join(s.opt.Dir, name))
		if err != nil {
			continue
		}
		cfg, err := yaml.Parse(string(b))
		if err != nil {
			return fmt.Errorf("%s: %v", name, err)
		}
		s.Config = cfg
		break
	}
	def := func(k string, v any) {
		if s.Config[k] == nil {
			s.Config[k] = v
		}
	}
	def("title", "Hanji")
	def("lang", "en")
	def("description", "")
	base := "/" + strings.Trim(expr.Str(s.Config["base"]), "/") + "/"
	if base == "//" {
		base = "/"
	}
	s.Base = base
	s.Config["base"] = base
	s.Config["url"] = strings.TrimRight(expr.Str(s.Config["url"]), "/")
	s.Config["dev"] = s.opt.Dev
	s.Config["generator"] = "Hanji " + Version
	// nav links get the base path applied once, here.
	for _, it := range expr.List(s.Config["nav"]) {
		if m, ok := it.(map[string]any); ok {
			m["href"] = s.url(expr.Str(m["href"]))
		}
	}
	return nil
}

// loadStrings picks the theme's interface text for site.lang (en if there
// is none), then applies overrides from `strings:` in hanji.yml.
func (s *Site) loadStrings() {
	s.strings = map[string]any{}
	if b, err := fs.ReadFile(s.opt.Theme, "strings.yml"); err == nil {
		all, err := yaml.Parse(string(b))
		if err != nil {
			s.errf("theme strings.yml: %v", err)
		}
		lang, _, _ := strings.Cut(strings.ToLower(expr.Str(s.Config["lang"])), "-")
		for _, l := range []string{"en", lang} {
			if m, ok := all[l].(map[string]any); ok {
				for k, v := range m {
					s.strings[k] = v
				}
			}
		}
	}
	if m, ok := s.Config["strings"].(map[string]any); ok {
		for k, v := range m {
			s.strings[k] = v
		}
	}
}

// T returns one piece of interface text.
func (s *Site) T(key string) string { return expr.Str(s.strings[key]) }

// url applies the base path to a root-relative URL.
func (s *Site) url(p string) string {
	if strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") {
		if s.Base != "/" && strings.HasPrefix(p, s.Base) {
			return p // already carries the base, e.g. a page.url
		}
		return s.Base + p[1:]
	}
	return p
}

var (
	styleRe  = regexp.MustCompile(`(?s)<style>(.*?)</style>\s*`)
	scriptRe = regexp.MustCompile(`(?s)<script>(.*?)</script>\s*`)
)

func splitFrontMatter(src string) (map[string]any, string, error) {
	src = strings.TrimPrefix(strings.ReplaceAll(src, "\r\n", "\n"), "\uFEFF")
	if !strings.HasPrefix(src, "---\n") {
		return map[string]any{}, src, nil
	}
	end := strings.Index(src[4:], "\n---")
	if end < 0 {
		return map[string]any{}, src, nil
	}
	fm, err := yaml.Parse(src[4 : 4+end])
	rest := src[4+end+4:]
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		rest = rest[i+1:]
	} else {
		rest = ""
	}
	if err != nil {
		return nil, "", fmt.Errorf("front matter: %v", err)
	}
	// keep line numbers honest for error messages
	return fm, strings.Repeat("\n", strings.Count(src[:len(src)-len(rest)], "\n")) + rest, nil
}

func (s *Site) loadTemplates() {
	load := func(fsys fs.FS, dir string, into map[string]*Component, extract bool) {
		entries, err := fs.ReadDir(fsys, dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
				continue
			}
			b, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
			if err != nil {
				s.errf("%s/%s: %v", dir, e.Name(), err)
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".html")
			fm, src, err := splitFrontMatter(string(b))
			if err != nil {
				s.errf("%s/%s: %v", dir, e.Name(), err)
				continue
			}
			c := &Component{Name: name, Layout: expr.Str(fm["layout"]), OwnTitle: fm["title"] == false}
			if extract {
				for _, m := range styleRe.FindAllStringSubmatch(src, -1) {
					c.Style += strings.TrimSpace(m[1]) + "\n"
				}
				for _, m := range scriptRe.FindAllStringSubmatch(src, -1) {
					c.Script += strings.TrimSpace(m[1]) + "\n"
				}
				src = scriptRe.ReplaceAllString(styleRe.ReplaceAllString(src, ""), "")
			}
			c.Tmpl, err = tmpl.Parse(dir+"/"+e.Name(), strings.TrimSpace(src))
			if err != nil {
				s.errf("%v", err)
				continue
			}
			into[name] = c
		}
	}
	local := os.DirFS(s.opt.Dir)
	for _, fsys := range []fs.FS{s.opt.Theme, local} { // project files override the theme
		load(fsys, "layouts", s.layouts, false)
		load(fsys, "components", s.components, true)
	}
}

func init() {
	expr.Filters["slug"] = func(in any, _ []any) (any, error) { return md.Slug(expr.Str(in)), nil }
}

var orderRe = regexp.MustCompile(`^(\d+)[-_.](.+)$`)

func humanize(s string) string {
	s = strings.NewReplacer("-", " ", "_", " ").Replace(s)
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[:1])) + string(r[1:])
}

func (s *Site) loadPages() {
	root := filepath.Join(s.opt.Dir, "pages")
	var expand []*Page
	err := filepath.WalkDir(root, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if file != root && (strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, file)
		rel = filepath.ToSlash(rel)
		ext := path.Ext(rel)
		// strip ordering prefixes ("01-intro") from every segment
		segs := strings.Split(strings.TrimSuffix(rel, ext), "/")
		order := math.Inf(1)
		for i, seg := range segs {
			if m := orderRe.FindStringSubmatch(seg); m != nil {
				n, _ := strconv.ParseFloat(m[1], 64)
				segs[i] = m[2]
				if i == len(segs)-1 {
					order = n
				} else {
					s.dirOrder[strings.Join(segs[:i+1], "/")] = n
				}
			}
		}
		if ext != ".md" && ext != ".html" {
			s.static[strings.Join(segs, "/")+ext] = file
			return nil
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		fm, body, err := splitFrontMatter(string(b))
		if err != nil {
			s.errf("pages/%s: %v", rel, err)
			return nil
		}
		if expr.Truthy(fm["draft"]) && !s.opt.Dev {
			return nil
		}
		p := &Page{Src: rel, FM: fm, body: body, IsHTML: ext == ".html", order: order}
		if segs[len(segs)-1] == "index" {
			p.index = true
			segs = segs[:len(segs)-1]
			if len(segs) > 0 {
				if o, ok := s.dirOrder[strings.Join(segs, "/")]; ok {
					p.order = o
				}
			}
		}
		p.ID = strings.Join(segs, "/")
		if o, ok := fm["order"].(float64); ok {
			p.order = o
		}
		if !p.IsHTML {
			p.nodes = md.Parse(body)
			// A leading "# Title" is the page title; layouts print it.
			if len(p.nodes) > 0 && p.nodes[0].Kind == md.Heading && p.nodes[0].Level == 1 {
				if fm["title"] == nil {
					fm["title"] = expr.StripTags((&md.Renderer{}).Inline(p.nodes[0].Text))
				}
				p.h1, p.nodes = p.nodes[0], p.nodes[1:]
			}
		}
		s.bySrc[rel] = p
		if fm["expand"] != nil {
			expand = append(expand, p)
		} else {
			s.addPage(p)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		s.errf("pages: %v", err)
	}
	if len(s.Pages) == 0 && len(expand) == 0 && len(s.errs) == 0 {
		s.errf("no pages found in %s — create pages/index.md (or run `hanji new`)", root)
	}
	s.sortPages()
	// One file → many pages: `expand: tag in tags()` with [tag] in the filename.
	for _, p := range expand {
		v, _, listSrc, ok := tmpl.SplitEach(expr.Str(p.FM["expand"]))
		if !ok {
			s.errf("pages/%s: expand must look like `name in expression`", p.Src)
			continue
		}
		list, err := expr.Eval(listSrc, s.env(nil))
		if err != nil {
			s.errf("pages/%s: expand: %v", p.Src, err)
			continue
		}
		if !strings.Contains(p.ID, "["+v+"]") {
			s.errf("pages/%s: expand needs [%s] somewhere in the file name", p.Src, v)
			continue
		}
		for _, item := range expr.List(list) {
			q := *p
			q.FM = map[string]any{}
			for k, val := range p.FM {
				q.FM[k] = val
			}
			q.Vars = map[string]any{v: item}
			q.ID = strings.ReplaceAll(p.ID, "["+v+"]", md.Slug(expr.Str(item)))
			s.addPage(&q)
		}
	}
	s.sortPages()
	s.buildNav()
}

func (s *Site) sortPages() {
	sort.SliceStable(s.Pages, func(i, j int) bool { return pageLess(s.Pages[i], s.Pages[j]) })
}

// pageLess orders newest first, then by explicit order, then by title.
func pageLess(a, b *Page) bool {
	ad, aok := a.Data["date"].(time.Time)
	bd, bok := b.Data["date"].(time.Time)
	if aok != bok {
		return aok
	}
	if aok && !ad.Equal(bd) {
		return ad.After(bd)
	}
	if a.order != b.order {
		return a.order < b.order
	}
	return expr.Str(a.Data["title"]) < expr.Str(b.Data["title"])
}

func (s *Site) addPage(p *Page) {
	route := "/"
	p.Out = "index.html"
	if p.ID != "" {
		route = "/" + p.ID + "/"
		p.Out = p.ID + "/index.html"
	}
	if p.ID == "404" {
		p.Out = "404.html"
	}
	p.URL = s.url(route)
	fm := p.FM
	d := map[string]any{}
	for k, v := range fm {
		d[k] = v
	}
	title := expr.Str(fm["title"])
	if title == "" {
		title = humanize(path.Base(p.ID))
		if p.ID == "" {
			title = expr.Str(s.Config["title"])
		}
	}
	if strings.Contains(title, "{{") { // titles of expanded pages can use their variable
		if t, err := tmpl.Parse("title", title); err == nil {
			if out, err := t.Render(expr.NewEnv(p.Vars), &tmpl.Ctx{}); err == nil {
				title = expr.StripTags(out)
			}
		}
	}
	section := ""
	if i := strings.IndexByte(p.ID, '/'); i > 0 {
		section = p.ID[:i]
	} else if p.index && p.ID != "" {
		section = p.ID
	}
	// The summary is the first paragraph, rendered with only the page's own
	// data in scope (the site's functions are not ready while pages load).
	text := ""
	plain := &md.Renderer{Eval: func(src string) string {
		vars := map[string]any{"page": fm, "site": s.Config}
		for k, v := range p.Vars {
			vars[k] = v
		}
		v, err := expr.Eval(src, expr.NewEnv(vars))
		if err != nil {
			return ""
		}
		return tmpl.Escape(v)
	}}
	for _, n := range p.nodes {
		if n.Kind == md.Para && text == "" {
			text = strings.Join(strings.Fields(expr.StripTags(plain.Inline(n.Text))), " ")
		}
	}
	desc := expr.Str(fm["description"])
	if desc == "" {
		if r := []rune(text); len(r) > 160 {
			desc = string(r[:160]) + "…"
		} else {
			desc = text
		}
	}
	chars := 0
	for _, c := range p.body {
		if c > ' ' {
			chars++
		}
	}
	layout := expr.Str(fm["layout"])
	if layout == "" {
		if m, ok := s.Config["layouts"].(map[string]any); ok && section != "" {
			layout = expr.Str(m[section])
		}
	}
	if layout == "" {
		layout = "page"
		if p.ID == "" {
			layout = "home"
		}
	}
	tags := []any{}
	for _, t := range expr.List(fm["tags"]) {
		tags = append(tags, expr.Str(t))
	}
	d["title"] = title
	d["description"] = desc
	d["excerpt"] = desc
	d["url"] = p.URL
	d["id"] = p.ID
	d["src"] = "pages/" + p.Src
	d["section"] = section
	d["layout"] = layout
	d["tags"] = tags
	d["readingTime"] = math.Max(1, math.Ceil(float64(chars)/700))
	if p.order != math.Inf(1) {
		d["order"] = p.order
	}
	for k, v := range p.Vars {
		d[k] = v
	}
	p.Data = d
	s.Pages = append(s.Pages, p)
}

// env is the variable scope a page renders in. p may be nil.
func (s *Site) env(p *Page) *expr.Env {
	vars := map[string]any{
		"site":  s.Config,
		"t":     s.strings,
		"now":   time.Now(),
		"pages": expr.Func(s.fnPages),
		"tags":  expr.Func(s.fnTags),
		"url": expr.Func(func(a []any) (any, error) {
			if len(a) == 0 {
				return s.Base, nil
			}
			return s.url(expr.Str(a[0])), nil
		}),
		"len":   expr.Func(func(a []any) (any, error) { return expr.Filters["len"](first(a), nil) }),
		"range": expr.Func(fnRange),
	}
	if p != nil {
		vars["page"] = p.Data
		vars["sidebar"] = expr.Func(func(a []any) (any, error) {
			sec := expr.Str(p.Data["section"])
			if len(a) > 0 {
				sec = strings.Trim(expr.Str(a[0]), "/")
			}
			return s.sidebar(sec, p), nil
		})
		for k, v := range p.Vars {
			vars[k] = v
		}
	}
	return expr.NewEnv(vars)
}

func first(a []any) any {
	if len(a) > 0 {
		return a[0]
	}
	return nil
}

func fnRange(a []any) (any, error) {
	from, to := 0.0, 0.0
	switch len(a) {
	case 1:
		to, _ = expr.Num(a[0])
	case 2:
		from, _ = expr.Num(a[0])
		to, _ = expr.Num(a[1])
	default:
		return nil, fmt.Errorf("range(n) or range(from, to)")
	}
	out := []any{}
	for i := from; i < to && len(out) < 10000; i++ {
		out = append(out, i)
	}
	return out, nil
}

func globRe(pat string) (*regexp.Regexp, error) {
	pat = strings.Trim(pat, "/")
	q := regexp.QuoteMeta(pat)
	q = strings.ReplaceAll(q, `\*\*`, `.+`)
	q = strings.ReplaceAll(q, `\*`, `[^/]+`)
	return regexp.Compile("^" + q + "$")
}

// fnPages implements pages("blog/*"): pages whose id matches the glob,
// newest first. * matches one path segment, ** any depth.
func (s *Site) fnPages(a []any) (any, error) {
	var re *regexp.Regexp
	if len(a) > 0 {
		var err error
		if re, err = globRe(expr.Str(a[0])); err != nil {
			return nil, err
		}
	}
	out := []any{}
	for _, p := range s.Pages {
		if p.ID == "404" || expr.Truthy(p.FM["unlisted"]) || (re != nil && !re.MatchString(p.ID)) {
			continue
		}
		out = append(out, p.Data)
	}
	return out, nil
}

func (s *Site) fnTags([]any) (any, error) {
	seen := map[string]bool{}
	for _, p := range s.Pages {
		for _, t := range expr.List(p.Data["tags"]) {
			seen[expr.Str(t)] = true
		}
	}
	names := make([]string, 0, len(seen))
	for t := range seen {
		names = append(names, t)
	}
	sort.Strings(names)
	out := make([]any, len(names))
	for i, t := range names {
		out[i] = t
	}
	return out, nil
}

// ---- section navigation ----

type navNode struct {
	title    string
	page     *Page
	order    float64
	children []*navNode
}

// buildNav derives a sidebar tree per top-level section from the file tree,
// and links each page to its previous and next neighbour.
func (s *Site) buildNav() {
	s.sidebars = map[string][]*navNode{}
	byID := map[string]*Page{}
	sections := map[string]bool{}
	for _, p := range s.Pages {
		byID[p.ID] = p
		if sec := expr.Str(p.Data["section"]); sec != "" && !expr.Truthy(p.FM["unlisted"]) {
			sections[sec] = true
		}
	}
	for sec := range sections {
		dirs := map[string]*navNode{}
		var get func(id string) *navNode
		get = func(id string) *navNode {
			if n, ok := dirs[id]; ok {
				return n
			}
			n := &navNode{title: humanize(path.Base(id)), order: math.Inf(1)}
			if o, ok := s.dirOrder[id]; ok {
				n.order = o
			}
			if p := byID[id]; p != nil {
				n.title, n.page, n.order = expr.Str(p.Data["title"]), p, p.order
			}
			dirs[id] = n
			if id != sec {
				parent := get(path.Dir(id))
				parent.children = append(parent.children, n)
			}
			return n
		}
		for _, p := range s.Pages {
			if (p.ID == sec || strings.HasPrefix(p.ID, sec+"/")) && !expr.Truthy(p.FM["unlisted"]) {
				get(p.ID)
			}
		}
		var sortTree func(n *navNode)
		sortTree = func(n *navNode) {
			sort.SliceStable(n.children, func(i, j int) bool {
				a, b := n.children[i], n.children[j]
				if a.order != b.order {
					return a.order < b.order
				}
				return a.title < b.title
			})
			for _, c := range n.children {
				sortTree(c)
			}
		}
		root := dirs[sec]
		sortTree(root)
		tree := root.children
		if root.page != nil { // the section index leads its own sidebar
			tree = append([]*navNode{{title: root.title, page: root.page}}, tree...)
		}
		s.sidebars[sec] = tree
		var flat []*Page
		var walk func(ns []*navNode)
		walk = func(ns []*navNode) {
			for _, n := range ns {
				if n.page != nil {
					flat = append(flat, n.page)
				}
				walk(n.children)
			}
		}
		walk(tree)
		link := func(p *Page) any { return map[string]any{"title": p.Data["title"], "url": p.URL} }
		for i, p := range flat {
			if i > 0 {
				p.Data["prev"] = link(flat[i-1])
			}
			if i+1 < len(flat) {
				p.Data["next"] = link(flat[i+1])
			}
		}
	}
}

func (s *Site) sidebar(sec string, cur *Page) []any {
	var conv func(ns []*navNode) ([]any, bool)
	conv = func(ns []*navNode) ([]any, bool) {
		out, anyActive := []any{}, false
		for _, n := range ns {
			kids, active := conv(n.children)
			m := map[string]any{"title": n.title, "children": kids, "current": n.page != nil && n.page == cur}
			if n.page != nil {
				m["url"] = n.page.URL
			}
			active = active || n.page == cur
			m["active"] = active
			anyActive = anyActive || active
			out = append(out, m)
		}
		return out, anyActive
	}
	out, _ := conv(s.sidebars[sec])
	return out
}
