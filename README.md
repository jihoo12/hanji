# Hanji 한지

**English** · [한국어](README.ko.md)

**A markdown-first web framework where the document is the app.**

Write markdown and you get the whole frame of a website: navigation, sidebar, table of
contents, search, dark mode, RSS. Add a pair of braces and the text comes alive. The
compiler is a single Go binary with no dependencies.

```md
---
state:
  count: 0
---

# Hello

You have clicked **{count}** times. [One more](@count += 1)

::: if count >= 5
::: tip Past five
This box is only visible while the condition holds.
:::
:::
```

That one file becomes a page with a working button. No JavaScript file, no component file.

*Hanji (韓紙) is traditional Korean paper made from mulberry bark: thin, tough, and said to
last a thousand years.*

## Quick start

```bash
git clone https://github.com/jihoo12/hanji.git && cd hanji
nix develop -c go build -o bin/hanji ./cmd/hanji    # or: go build -o bin/hanji ./cmd/hanji (Go 1.26+)

bin/hanji new my-site     # scaffold the starter site (it doubles as the docs)
bin/hanji dev my-site     # http://localhost:5151, reloads on save
bin/hanji build my-site   # static site in my-site/dist
```

With Nix you can try it without installing anything: `nix run . -- dev starter`.

The full guide lives in the starter site (`starter/pages/docs/`) and is readable in the
browser once the dev server is running. The guide itself is currently written in Korean. The
built-in theme's interface text follows `lang:` in `hanji.yml` (`en` by default, `ko`
available) and single strings can be overridden under `strings:`.

## What makes it different

| | |
| --- | --- |
| **Live markdown** | `{expr}` shows a value, `[label](@code)` is a button, `[? name]` is a bound input, `::: if` / `::: each` for conditions and loops. State goes in front matter under `state`, derived values under `derive`, and `persist: true` saves it in the browser |
| **Directives are components** | `::: card title="…"` is one file, `components/card.html`. Its styles and scripts ship only on pages that use it |
| **Build-time expressions** | `{{ pages("blog/*") \| where("tags", "go") \| limit(3) }}`, a small expression language with pipes |
| **Files are the structure** | `pages/docs/01-start.md` → `/docs/start/`. Folders become the sidebar, headings the table of contents, numeric prefixes the order |
| **One file, many pages** | `pages/tags/[tag].md` with `expand: tag in tags()` |
| **A frame out of the box** | Built-in theme (5 layouts, 11 components), search, instant navigation with View Transitions, dark mode, syntax highlighting, sitemap, RSS |
| **Pay for what you use** | Pages without live expressions ship no runtime |

## The syntax in one screen

```md
{{ page.title }}                      computed at build time
{count * 2}                           live value (only when it mentions state)
[+1](@count += 1)                     button that runs code
[? name | Your name]                  text input bound to state
[? volume | 0..100 step 5]            slider
[? plan | Basic / Pro / Team]         select
[? draft](@todos.push(draft))         input that runs code on Enter

::: tip Title                         callout (note, tip, warn, danger)
Body is **markdown**.
:::

::postlist from="blog/*" limit=3      a component with no body

::: each p in pages("blog/*")         build-time loop
- [{{ p.title }}]({{ p.url }})
:::
```

Layouts and components are HTML files using the same expressions:

```html
{{#if page.date}} … {{:else}} … {{/if}}
{{#each p, i in pages("blog/*")}} … {{/each}}
{{> card title="Hi" href=page.url}}
{{slot}}
```

## Layout of the repository

```text
cmd/hanji/          CLI (new · dev · build · preview)
internal/
  md/               markdown parser and renderer (directives, live expressions)
  expr/             the {{ }} expression language and its filters
  tmpl/             HTML template engine for layouts and components
  yaml/             YAML-subset parser for front matter and hanji.yml
  site/             project loading, routing, rendering, live compilation, highlighting
  server/           dev server (change detection, in-memory serving, reload, error page)
theme/              built-in theme, embedded in the binary
  layouts/          base · page · home · doc · post
  components/       callout · card · grid · hero · steps · tabs · details · postlist …
  assets/           theme.css · app.js (navigation/search/theme) · live.js (live runtime)
starter/            the site `hanji new` writes out (docs and example in one)
```

Everything uses the Go standard library only. The markdown parser, expression language,
template engine and YAML parser are all implemented here.

## Development

```bash
nix develop -c go test ./...
nix develop -c go vet ./...
nix build            # result/bin/hanji
```

## Known limits

- Markdown is not full CommonMark. Indented code blocks, reference links (`[a][b]`), Setext headings and footnotes are not supported.
- YAML is a subset: maps, lists, flow notation (`[]`, `{}`), `|` blocks and quoted strings. No anchors or multi-line flow notation.
- Live expressions are analysed by identifier only. An arrow-function parameter with the same name as a state variable is treated as that state.
- Syntax highlighting is a light generic lexer (Go, JS/TS, Rust, Python, Bash, JSON, YAML, CSS, HTML, C, Java, Nix).
- Change detection polls every 300ms and rebuilds the whole site (about 6ms for the 16-page starter).

## License

[Apache License 2.0](LICENSE)
