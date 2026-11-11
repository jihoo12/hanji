---
title: 레이아웃과 템플릿
---

레이아웃은 페이지 본문을 감싸는 틀입니다. `layouts/` 폴더의 HTML 파일이고, 컴포넌트와 같은
템플릿 문법을 씁니다.

## 내장 레이아웃

| 이름 | 모양 |
| --- | --- |
| `base` | `<html>` 뼈대, 머리글, 바닥글, 검색 창. 다른 레이아웃이 모두 이것을 감쌉니다 |
| `page` | 가운데 한 단. 기본값 |
| `home` | 넓은 한 단, 제목 없음. `pages/index.md` 의 기본값 |
| `doc` | 왼쪽 사이드바 + 본문 + 오른쪽 목차 + 이전/다음 |
| `post` | 날짜, 읽는 시간, 태그가 붙은 글 |
| `none` | 레이아웃 없이 본문만 |

어떤 레이아웃을 쓸지는 이 순서로 정해집니다: 머리말의 `layout` → `hanji.yml` 의
`layouts.<섹션>` → `page`.

## 레이아웃 만들기

```html title="layouts/wide.html"
---
layout: base
---
<main id="content" class="wide">
  <h1>{{ page.title }}</h1>
  {{slot}}
</main>
```

머리말의 `layout: base` 는 "이 레이아웃을 다시 `base` 로 감싼다"는 뜻입니다. 이렇게 겹쳐 쌓을 수
있어서, 머리글과 바닥글을 되풀이해 쓸 필요가 없습니다. 레이아웃이 제목을 직접 찍지 않는다면
머리말에 `title: false` 를 적으세요. 그러면 본문의 `# 제목` 이 지워지지 않고 그대로 남습니다
(`home` 이 그렇게 되어 있습니다). 내장 `base.html` 을 통째로 바꾸고 싶으면
같은 이름으로 `layouts/base.html` 을 만드세요.

## 템플릿 문법

```html
\{{ 식 }}                       값 출력 (HTML은 자동으로 이스케이프됩니다)
\{{ page.html | raw }}          이스케이프 없이 출력

\{{#if page.date}} … \{{:else if page.draft}} … \{{:else}} … \{{/if}}

\{{#each p, i in pages("blog/*")}}
  <li>\{{ i + 1 }}. \{{ p.title }}</li>
\{{:else}}
  <li>글이 없습니다</li>
\{{/each}}

\{{> card title="안녕" href=page.url}}        본문 없는 컴포넌트
\{{#> card title="안녕"}} 본문 \{{/>}}         본문 있는 컴포넌트
\{{slot}}                        감싸고 있는 본문
\{{! 주석 }}
```

중괄호 두 겹을 글자 그대로 쓰고 싶으면 앞에 역슬래시를 붙입니다: `\\{{`.

## HTML 페이지

마크다운으로 표현하기 어려운 페이지는 `pages/` 에 `.html` 로 만들 수 있습니다. 머리말을 쓸 수
있고, 본문은 위의 템플릿 문법으로 처리됩니다.

## 테마 손보기

색과 글꼴은 CSS 변수입니다. `public/custom.css` 를 만들고 `hanji.yml` 에 등록하세요.

```yaml title="hanji.yml"
css: [/custom.css]
accent: "#2f628f"     # 강조 색만 바꾸려면 이 한 줄로 충분합니다
```

```css title="public/custom.css"
:root {
  --paper: #ffffff;
  --serif: "Gowun Batang", serif;
  --content: 48rem;      /* 본문 너비 */
}
```
