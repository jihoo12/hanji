---
title: 시작하기
---

## 설치

한지는 Go로 쓴 실행 파일 하나입니다. 소스에서 빌드하려면:

```bash
git clone https://github.com/jihoo12/hanji.git && cd hanji
nix develop -c go build -o bin/hanji ./cmd/hanji   # Nix를 쓴다면
go build -o bin/hanji ./cmd/hanji                  # Go 1.26+ 가 있다면
```

## 첫 사이트

::: steps
1. **만들기**

   ```bash
   hanji new my-site
   cd my-site
   ```

2. **개발 서버 띄우기**

   ```bash
   hanji dev
   ```

   `http://localhost:5151` 을 엽니다. 파일을 저장하면 브라우저가 스스로 새로고침됩니다.
   빌드 오류가 나면 어느 파일 몇 번째 줄인지 화면에 보여 줍니다.

3. **빌드하기**

   ```bash
   hanji build
   ```

   `dist/` 에 정적 파일이 만들어집니다. 아무 정적 호스팅에나 올리면 됩니다.
:::

## 폴더 구조

```text
my-site/
├── hanji.yml          사이트 설정
├── pages/             페이지 (파일 경로 = 주소)
│   ├── index.md         →  /
│   ├── docs/
│   │   ├── index.md     →  /docs/
│   │   └── 01-start.md  →  /docs/start/
│   └── blog/
├── components/        내 컴포넌트 (선택)
├── layouts/           내 레이아웃 (선택)
└── public/            그대로 복사되는 파일 (이미지, 폰트 …)
```

`components/` 와 `layouts/` 가 비어 있어도 됩니다. 한지에 내장된 테마가 같은 이름의 파일을
이미 갖고 있고, 여러분이 같은 이름으로 파일을 만들면 **그 파일이 내장 것을 덮어씁니다**.

## 주소 규칙

| 파일 | 주소 |
| --- | --- |
| `pages/index.md` | `/` |
| `pages/about.md` | `/about/` |
| `pages/docs/index.md` | `/docs/` |
| `pages/docs/01-start.md` | `/docs/start/` — 숫자 접두사는 순서만 정하고 주소에서는 빠집니다 |
| `pages/404.md` | `/404.html` |
| `pages/_draft.md` | 만들어지지 않음 — `_` 로 시작하면 무시합니다 |
| `pages/blog/photo.png` | `/blog/photo.png` — 마크다운이 아닌 파일은 그대로 복사됩니다 |

## 머리말

파일 맨 위 `---` 사이에 YAML로 페이지 정보를 적습니다. 전부 선택 사항입니다.

```yaml
---
title: 시작하기        # 없으면 본문의 첫 "# 제목"을 씁니다
description: 검색과 목록에 쓰일 요약
date: 2026-11-11       # 날짜가 있으면 목록에서 최신순으로 정렬됩니다
tags: [소식, 한지]
layout: doc            # 없으면 hanji.yml 의 layouts, 그것도 없으면 page
order: 3               # 사이드바 순서 (파일 이름 접두사 대신)
draft: true            # dev 에서만 보이고 build 에서는 빠집니다
unlisted: true         # 만들어지지만 목록·검색·사이드바에는 안 나옵니다
---
```

여기에 적은 값은 무엇이든 템플릿과 본문에서 `page.이름` 으로 읽을 수 있습니다.
