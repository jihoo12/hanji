---
title: 컴포넌트
---

컴포넌트는 `components/` 폴더의 **HTML 파일 하나**입니다. 마크다운에서는 디렉티브로,
레이아웃에서는 `{{> 이름}}` 으로 씁니다.

## 내장 컴포넌트

### 강조 상자 — `note` `tip` `warn` `danger`

::: danger 되돌릴 수 없습니다
`hanji build` 는 출력 폴더를 비우고 다시 씁니다. 한지가 만든 폴더가 아니면 건드리지 않습니다.
:::

### 카드와 격자 — `card` `grid`

`card` 는 `title`, `icon`, `href` 를, `grid` 는 `cols` 를 받습니다.

:::: grid cols=3
::: card title="문서" icon="📚" href="/docs/"
처음부터 읽기
:::
::: card title="블로그" icon="✍️" href="/blog/"
소식과 이야기
:::
::: card title="그냥 상자" icon="📦"
`href` 가 없으면 링크가 아닙니다
:::
::::

### 단계 — `steps`

번호 목록을 감싸면 단계별 안내가 됩니다. [시작하기](01-start.md)에서 쓰고 있습니다.

### 탭 — `tabs` `tab`

`````md
:::: tabs
::: tab title="Nix"
```bash
nix run . -- dev
```
:::
::: tab title="Go"
```bash
go run ./cmd/hanji dev
```
:::
::::
`````

:::: tabs
::: tab title="Nix"
```bash
nix run . -- dev
```
:::
::: tab title="Go"
```bash
go run ./cmd/hanji dev
```
:::
::::

### 접기 — `details`

::: details 눌러서 펼치기
`open=true` 를 주면 처음부터 펼쳐져 있습니다.
:::

### 그 밖에

| 이름 | 쓰임 | 속성 |
| --- | --- | --- |
| `hero` | 첫 화면의 큰 제목. 본문의 링크는 버튼이 됩니다 | `title` `tagline` `eyebrow` |
| `postlist` | 글 목록 | `from` (예: `"blog/*"`) `limit` |
| `figure` | 설명이 달린 그림 | `src` `alt`, 제목이 설명이 됩니다 |

## 직접 만들기

이 스타터에는 `components/quote.html` 이 들어 있습니다. 전체 내용은 이렇습니다.

```html title="components/quote.html"
<figure class="quote">
  <blockquote>{{slot}}</blockquote>
  {{#if by}}<figcaption>— {{ by }}</figcaption>{{/if}}
</figure>

<style>
.quote { margin: 2rem 0; padding: 1.4rem 1.6rem; background: var(--paper-sunk); }
</style>
```

```md
::: quote by="어느 종이 장인"
좋은 종이는 **물**과 기다림이 만든다.
:::
```

::: quote by="어느 종이 장인"
좋은 종이는 **물**과 기다림이 만든다.
:::

규칙은 이게 전부입니다.

- 디렉티브의 속성은 그 이름 그대로 변수가 됩니다 (`by`). 주지 않은 속성은 비어 있습니다.
- 디렉티브의 본문은 마크다운으로 변환되어 `{{slot}}` 자리에 들어갑니다.
- `page`, `site` 와 모든 [함수](06-data.md)를 쓸 수 있습니다.
- 속성에 식을 넘기려면 중괄호를 씁니다: `::: card title={page.title}`
- `<style>` 과 `<script>` 는 떼어 내서, **이 컴포넌트를 쓴 페이지에만** 한 번씩 넣습니다.
  스크립트는 모듈로 실행되고, 페이지를 이동할 때마다 다시 실행됩니다.
- 내장 컴포넌트와 같은 이름으로 만들면 내 것이 우선합니다. 예를 들어 `components/callout.html`
  을 만들면 모든 `::: tip` 의 모양이 바뀝니다.

::: tip 스타일은 테마 변수로
`var(--paper)`, `var(--ink)`, `var(--accent)`, `var(--line)`, `var(--radius)` 같은 변수를 쓰면
어두운 테마에서도 저절로 어울립니다.
:::
