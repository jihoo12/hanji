package expr

import (
	"testing"
	"time"
)

func TestEval(t *testing.T) {
	posts := []any{
		map[string]any{"title": "b", "tags": []any{"go", "web"}, "n": 2.0},
		map[string]any{"title": "a", "tags": []any{"web"}, "n": 1.0},
	}
	env := NewEnv(map[string]any{
		"page":  map[string]any{"title": "한지", "draft": false, "date": time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)},
		"posts": posts,
		"add":   Func(func(a []any) (any, error) { x, _ := Num(a[0]); y, _ := Num(a[1]); return x + y, nil }),
	})
	cases := []struct{ src, want string }{
		{`1 + 2 * 3`, "7"},
		{`(1 + 2) * 3 % 4`, "1"},
		{`"a" + 1 + 'b'`, "a1b"},
		{`page.title + " · " + page["title"]`, "한지 · 한지"},
		{`page.draft ? "초안" : "발행"`, "발행"},
		{`page.missing.deep ?? "없음"`, "없음"},
		{`!page.draft && 3 > 2 && "ok"`, "ok"},
		{`page.missing || "기본"`, "기본"},
		{`page.date | date("YYYY년 M월 D일")`, "2026년 11월 11일"},
		{`page.date.year`, "2026"},
		{`"Hello" | upper | truncate(3)`, "HEL…"},
		{`posts | where("tags", "go") | map("title") | join("/")`, "b"},
		{`posts | sort("n") | map("title") | join("")`, "ab"},
		{`posts | sort("title", "desc") | first | json`, `{"n":2,"tags":["go","web"],"title":"b"}`},
		{`posts.length + (posts | limit(1) | len)`, "3"},
		{`posts[-1].tags[0]`, "web"},
		{`add(1, 2) == 3`, "true"},
		{`2 | add(5)`, "7"},
		{`[1, 2] + [3] | len`, "3"},
		{`"x" | default("y")`, "x"},
		{`"" | default("y")`, "y"},
	}
	for _, c := range cases {
		v, err := Eval(c.src, env)
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
		} else if got := Str(v); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
	}
	for _, bad := range []string{`1 +`, `"open`, `x | nope`, `page.title(1)`, `a ? b`} {
		if _, err := Eval(bad, env); err == nil {
			t.Errorf("%s: expected an error", bad)
		}
	}
}
