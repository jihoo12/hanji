package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hanji"
)

func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuild(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"hanji.yml":           "title: 시험\nbase: /sub/\nurl: https://example.com\nlayouts:\n  docs: doc\n",
		"pages/index.md":      "# 집\n\n[문서](docs/02-b.md) [절대](/docs/) ![그림](pic.png)\n",
		"pages/pic.png":       "png",
		"pages/docs/index.md": "# 문서\n\n::: box title={page.title}\n**본문**\n:::\n",
		"pages/docs/02-b.md":  "---\ntitle: 둘째\n---\n## 절\n내용\n",
		"pages/docs/01-a.md":  "---\ntitle: 첫째\nstate:\n  n: 1\n---\n{n} [더](@n += 1)\n",
		"pages/docs/_skip.md": "숨김",
		"pages/draft.md":      "---\ndraft: true\n---\n초안",
		"pages/t/[x].md":      "---\ntitle: \"T {{ x }}\"\nexpand: x in [\"가 나\", \"b\"]\n---\n값 {{ x }}\n",
		"components/box.html": "<div class=\"box\">{{ title }}{{slot}}</div>\n<style>.box{color:red}</style>\n<script>console.log(1)</script>",
		"public/robots.txt":   "ok",
	})
	res, err := Build(Options{Dir: dir, Theme: hanji.Theme()})
	if err != nil {
		t.Fatal(err)
	}
	page := func(name string) string {
		f, ok := res.Files[name]
		if !ok {
			t.Fatalf("missing output %s (have %d files)", name, len(res.Files))
		}
		return string(f.Data)
	}
	has := func(name, want string) {
		t.Helper()
		if !strings.Contains(page(name), want) {
			t.Errorf("%s: missing %q", name, want)
		}
	}
	has("index.html", `<a href="/sub/docs/b/">문서</a> <a href="/sub/docs/">절대</a> <img src="/sub/pic.png"`)
	has("index.html", `<h1 id="집">집</h1>`)
	has("docs/index.html", `<div class="box">문서<p><strong>본문</strong></p>`)
	has("docs/index.html", `.box{color:red}`)
	has("docs/index.html", `console.log(1)`)
	has("docs/b/index.html", `<a class="prev" href="/sub/docs/a/"><small>이전</small>첫째</a>`)
	has("docs/b/index.html", `<li class="l2"><a href="#절">절</a></li>`)
	has("docs/a/index.html", `<span data-hj-text="0">1</span> <button type="button" class="hj-action" data-hj-on="1">더</button>`)
	has("docs/a/index.html", `import{mount}from"/sub/_hanji/live.js";mount({state:{"n":1},derive:{},persist:false,fns:[$=>($.n),($,$event)=>{$.n += 1}]})`)
	has("t/가-나/index.html", `<h1>T 가 나</h1>`)
	has("t/b/index.html", `값 b`)
	has("sitemap.xml", `<loc>https://example.com/sub/docs/a/</loc>`)
	if strings.Contains(page("docs/b/index.html"), "live.js") {
		t.Error("a page without live expressions must not load live.js")
	}
	if strings.Contains(page("docs/b/index.html"), ".box{") {
		t.Error("component css leaked into a page that does not use it")
	}
	for _, name := range []string{"docs/skip/index.html", "draft/index.html"} {
		if _, ok := res.Files[name]; ok {
			t.Errorf("%s should not be built", name)
		}
	}
	for _, name := range []string{"pic.png", "robots.txt", "_hanji/theme.css", "_hanji/search.json"} {
		if _, ok := res.Files[name]; !ok {
			t.Errorf("missing %s", name)
		}
	}
	// the sidebar follows file-name order: index, 01-a, 02-b
	side := page("docs/b/index.html")
	if a, b := strings.Index(side, ">첫째</a>"), strings.Index(side, `aria-current="page">둘째</a>`); a < 0 || b < a {
		t.Errorf("sidebar order wrong: %d %d", a, b)
	}
}

func TestBuildErrors(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"pages/index.md": "# 집\n\n[깨진](nope.md)\n\n::: nothing\n:::\n\n{{ 1 + }}\n",
	})
	_, err := Build(Options{Dir: dir, Theme: hanji.Theme()})
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{"pages/index.md:3: broken link: nope.md", `pages/index.md:5: unknown component "nothing"`, "pages/index.md:8:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}

func TestStarterBuilds(t *testing.T) {
	dir := t.TempDir()
	if err := os.CopyFS(dir, hanji.Starter()); err != nil {
		t.Fatal(err)
	}
	res, err := Build(Options{Dir: dir, Theme: hanji.Theme()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Pages < 10 {
		t.Errorf("starter built only %d pages", res.Pages)
	}
}
