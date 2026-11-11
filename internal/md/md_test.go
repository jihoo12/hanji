package md

import (
	"strings"
	"testing"
)

func render(src string) string {
	var r *Renderer
	r = &Renderer{
		Eval: func(s string) string { return "<" + s + ">" },
		Live: func(s string) (string, bool) {
			if strings.Contains(s, "count") {
				return "[live:" + s + "]", true
			}
			return "", false
		},
		Action: func(label, code string) string { return "[act:" + label + "|" + code + "]" },
		Input: func(spec, enter string) (string, bool) {
			return "[in:" + spec + "|" + enter + "]", true
		},
		Directive: func(n *Node) string {
			return "<" + n.Info + " " + n.Args + ">" + r.Blocks(n.Children) + "</" + n.Info + ">"
		},
	}
	return strings.TrimSpace(r.Blocks(Parse(src)))
}

func TestInline(t *testing.T) {
	cases := []struct{ in, want string }{
		{"**굵게**는 *기울임* ~~취소~~ ==표시==", "<p><strong>굵게</strong>는 <em>기울임</em> <del>취소</del> <mark>표시</mark></p>"},
		{"*a **b** c*", "<p><em>a <strong>b</strong> c</em></p>"},
		{"snake_case_name and 2 * 3 * 4", "<p>snake_case_name and 2 * 3 * 4</p>"},
		{"`a < b` & <b>raw</b> <3", "<p><code>a &lt; b</code> &amp; <b>raw</b> &lt;3</p>"},
		{"[링크](/a \"제목\") ![그림](b.png)", `<p><a href="/a" title="제목">링크</a> <img src="b.png" alt="그림" loading="lazy"></p>`},
		{"{{ page.title }} 과 `{{ 코드 }}`", "<p><page.title> 과 <code>{{ 코드 }}</code></p>"},
		{"{count + 1} 과 {그냥 글자}", "<p>[live:count + 1] 과 {그냥 글자}</p>"},
		{"[+1](@count += f(1, \")\"))", `<p>[act:+1|count += f(1, ")")]</p>`},
		{"[? name | 힌트] [? draft](@add())", "<p>[in:name | 힌트|] [in:draft|add()]</p>"},
		{"줄  \n바꿈", "<p>줄<br>\n바꿈</p>"},
		{"see https://example.com/a.", `<p>see <a href="https://example.com/a">https://example.com/a</a>.</p>`},
		{`\*별\* \{{ x }}`, "<p>*별* {{ x }}</p>"},
	}
	for _, c := range cases {
		if got := render(c.in); got != c.want {
			t.Errorf("%q\n got  %s\n want %s", c.in, got, c.want)
		}
	}
}

func TestBlocks(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"heading ids", "## 안녕 World\n## 안녕 World\n### 직접 {#custom}",
			`<h2 id="안녕-world">안녕 World<a class="hj-anchor" href="#안녕-world" aria-label="이 섹션 링크">#</a></h2>` + "\n" +
				`<h2 id="안녕-world-2">안녕 World<a class="hj-anchor" href="#안녕-world-2" aria-label="이 섹션 링크">#</a></h2>` + "\n" +
				`<h3 id="custom">직접<a class="hj-anchor" href="#custom" aria-label="이 섹션 링크">#</a></h3>`},
		{"tight nested list", "- a\n  - b\n- c", "<ul>\n<li>a\n<ul>\n<li>b</li>\n</ul></li>\n<li>c</li>\n</ul>"},
		{"loose ordered list", "3. a\n\n4. b", "<ol start=\"3\">\n<li><p>a</p></li>\n<li><p>b</p></li>\n</ol>"},
		{"tasks", "- [x] 한 일\n- [ ] 할 일", "<ul class=\"hj-tasks\">\n<li><input type=\"checkbox\" disabled checked> 한 일</li>\n<li><input type=\"checkbox\" disabled> 할 일</li>\n</ul>"},
		{"quote", "> 인용\n계속", "<blockquote>\n<p>인용\n계속</p>\n</blockquote>"},
		{"table", "| a | b |\n|:--|--:|\n| `x|y` | 2 |", "<div class=\"hj-table\"><table>\n<thead>\n<tr><th style=\"text-align:left\">a</th><th style=\"text-align:right\">b</th></tr>\n</thead>\n<tbody>\n<tr><td style=\"text-align:left\"><code>x|y</code></td><td style=\"text-align:right\">2</td></tr>\n</tbody>\n</table></div>"},
		{"code keeps markup", "```\n**x** {{ y }}\n:::\n```", "<pre><code>**x** {{ y }}\n:::</code></pre>"},
		{"directive", "::: tip 제목 k=\"v\"\n본문\n:::\n뒤", "<tip 제목 k=\"v\"><p>본문</p>\n</tip><p>뒤</p>"},
		{"nested same colons", "::: if x\n::: tip\n안\n:::\n:::\n밖", "<if x><tip ><p>안</p>\n</tip></if><p>밖</p>"},
		{"nested longer colons", ":::: grid\n::: card\n1\n:::\n::: card\n2\n:::\n::::", "<grid ><card ><p>1</p>\n</card><card ><p>2</p>\n</card></grid>"},
		{"fence inside directive", "::: a\n```\n:::\n```\n:::", "<a ><pre><code>:::</code></pre>\n</a>"},
		{"leaf", "::postlist from=\"blog/*\"", `<postlist from="blog/*"></postlist>`},
		{"html block", "<div class=\"x\">\n{{ site.title }}\n</div>\n\n글", "<div class=\"x\">\n<site.title>\n</div>\n<p>글</p>"},
		{"hr vs list", "a\n\n---\n\n* * *", "<p>a</p>\n<hr>\n<hr>"},
	}
	for _, c := range cases {
		if got := render(c.in); got != c.want {
			t.Errorf("%s\n got  %q\n want %q", c.name, got, c.want)
		}
	}
}
