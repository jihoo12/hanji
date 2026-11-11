---
title: 설정과 배포
---

## hanji.yml

```yaml title="hanji.yml"
title: 내 사이트
description: 검색 엔진과 RSS에 쓰일 설명
lang: ko
seal: 紙                 # 로고 도장에 들어갈 글자
url: https://example.com # 있으면 sitemap.xml, feed.xml 을 만듭니다
base: /                  # 하위 경로에 올릴 때: /my-repo/
accent: "#b5362a"
css: [/custom.css]
footer: © 2026 나
tags: /tags/             # 태그 페이지 경로

nav:
  - text: 문서
    href: /docs/

layouts:                 # 섹션별 기본 레이아웃
  docs: doc
  blog: post
```

`lang` 은 내장 테마의 화면 문구(검색, 복사, 이전/다음 …)를 정합니다. 지금은 `ko` 와 `en` 이 있고,
없는 언어는 영어로 나옵니다. 문구 하나하나는 `strings` 로 바꿀 수 있습니다.

```yaml title="hanji.yml"
lang: ko
strings:
  madeWith: 손으로 만들었습니다
  onThisPage: 목차
```

템플릿에서는 `t.이름` 으로 읽습니다 (예: `{{ t.search }}`).

정해진 이름 말고도 무엇이든 적을 수 있고, 템플릿에서 `site.이름` 으로 읽습니다.

## 명령

| 명령 | 하는 일 |
| --- | --- |
| `hanji new <폴더>` | 스타터 사이트 만들기 |
| `hanji dev [폴더] --port 5151` | 개발 서버. 초안(`draft`)도 보입니다 |
| `hanji build [폴더] --out dist` | 정적 사이트 빌드 |
| `hanji preview [폴더]` | 빌드 결과를 그대로 띄워 보기 |

## 빌드 결과

```text
dist/
├── index.html
├── docs/start/index.html
├── 404.html
├── sitemap.xml · feed.xml      (url 을 설정했을 때)
└── _hanji/
    ├── theme.css   테마
    ├── app.js      즉시 이동, 검색, 테마 전환
    ├── live.js     살아있는 문서 런타임 (쓰는 페이지에서만 불러옵니다)
    └── search.json 검색 색인
```

서버가 필요 없는 정적 파일이므로 GitHub Pages, Cloudflare Pages, Netlify, S3, 혹은 nginx 어디든
`dist/` 를 그대로 올리면 됩니다.

::: warn 하위 경로에 올린다면
`https://이름.github.io/my-repo/` 처럼 하위 경로에 배포할 때는 `base: /my-repo/` 를 꼭 적으세요.
마크다운의 `/` 로 시작하는 링크와 내장 테마의 모든 주소에 자동으로 붙습니다.
:::
