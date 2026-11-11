package yaml

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	src := `
# comment
title: "한지: 종이"   # trailing
count: 3
ok: true
tags: [a, "b c", 3]
nav:
  - text: 문서
    href: /docs/
  - text: 블로그
    href: /blog/
state:
  todos:
    - { text: "하나", done: false }
    - plain
  nested:
    deep: 'it''s'
derive:
  left: todos.filter(t => !t.done).length
body: |
  줄 하나
    들여쓴 줄
empty:
`
	got, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(got)
	b := strings.TrimSpace(buf.String())
	want := `{"body":"줄 하나\n  들여쓴 줄","count":3,"derive":{"left":"todos.filter(t => !t.done).length"},"empty":null,"nav":[{"href":"/docs/","text":"문서"},{"href":"/blog/","text":"블로그"}],"ok":true,"state":{"nested":{"deep":"it's"},"todos":[{"done":false,"text":"하나"},"plain"]},"tags":["a","b c",3],"title":"한지: 종이"}`
	if b != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
	for _, bad := range []string{"just text", "a: 1\n   b: 2", "a: [1, 2"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}
