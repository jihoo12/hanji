# 한지 Hanji

[English](README.md) · **한국어**

**문서가 곧 앱이 되는 마크다운 웹 프레임워크.**

마크다운을 쓰면 사이트의 틀(메뉴, 사이드바, 목차, 검색, 다크 모드, RSS)이 잡히고,
중괄호 하나면 글이 살아 움직입니다. 컴파일러는 Go로 쓴 실행 파일 하나이고 의존성이 없습니다.

```md
---
state:
  count: 0
---

# 안녕하세요

지금까지 **{count}번** 눌렀습니다. [하나 더](@count += 1)

::: if count >= 5
::: tip 다섯 번을 넘겼습니다
이 상자는 조건이 참일 때만 보입니다.
:::
:::
```

이 파일 하나가 버튼이 동작하는 페이지가 됩니다. 자바스크립트 파일도, 컴포넌트 파일도 없습니다.

## 빠르게 시작하기

```bash
nix develop -c go build -o bin/hanji ./cmd/hanji    # 또는: go build -o bin/hanji ./cmd/hanji (Go 1.26+)

bin/hanji new my-site     # 스타터 사이트 만들기 (문서 겸 예제)
bin/hanji dev my-site     # http://localhost:5151 — 저장하면 자동 새로고침
bin/hanji build my-site   # my-site/dist 에 정적 사이트 생성
```

Nix 만 있다면 설치 없이 `nix run . -- dev starter` 로 바로 띄워 볼 수 있습니다.
자세한 사용법은 스타터 사이트의 문서(`starter/pages/docs/`)에 있고, 개발 서버로 띄우면 그대로 읽을 수 있습니다.

## 무엇이 다른가

| | |
| --- | --- |
| **살아있는 마크다운** | `{식}` 값, `[글자](@코드)` 버튼, `[? 이름]` 입력, `::: if` / `::: each`. 상태는 머리말의 `state`, 파생 값은 `derive`, `persist: true` 로 저장 |
| **디렉티브 = 컴포넌트** | `::: card title="…"` 은 `components/card.html` 한 파일. 스타일과 스크립트는 그 컴포넌트를 쓴 페이지에만 실립니다 |
| **빌드 타임 표현식** | `{{ pages("blog/*") \| where("tags", "go") \| limit(3) }}` — 파이프가 있는 작은 식 언어 |
| **파일이 곧 구조** | `pages/docs/01-start.md` → `/docs/start/`. 폴더가 사이드바, 제목이 목차, 숫자 접두사가 순서 |
| **한 파일로 여러 페이지** | `pages/tags/[tag].md` + `expand: tag in tags()` |
| **갖춰진 틀** | 내장 테마(레이아웃 5종, 컴포넌트 11종), 검색, 즉시 이동(View Transitions), 다크 모드, 코드 강조, 사이트맵, RSS |
| **쓴 만큼만** | 살아있는 표현이 없는 페이지에는 런타임이 실리지 않습니다 |

## 구조

```text
cmd/hanji/          CLI (new · dev · build · preview)
internal/
  md/               마크다운 파서와 렌더러 (디렉티브, 살아있는 표현 포함)
  expr/             {{ }} 표현식 언어와 필터
  tmpl/             레이아웃·컴포넌트용 HTML 템플릿 엔진
  yaml/             머리말과 hanji.yml 을 읽는 YAML 부분집합 파서
  site/             프로젝트 로딩, 라우팅, 렌더링, 살아있는 표현 컴파일, 코드 강조
  server/           개발 서버 (변경 감지, 메모리에서 서빙, 자동 새로고침, 오류 화면)
theme/              내장 테마 — 실행 파일에 포함됩니다
  layouts/          base · page · home · doc · post
  components/       callout · card · grid · hero · steps · tabs · details · postlist …
  assets/           theme.css · app.js (이동/검색/테마) · live.js (살아있는 문서 런타임)
starter/            `hanji new` 가 만들어 주는 사이트 (문서 겸 예제)
```

표준 라이브러리만 사용합니다. 마크다운 파서, 표현식 언어, 템플릿 엔진, YAML 파서를 모두 직접 구현했습니다.

## 개발

```bash
nix develop -c go test ./...
nix develop -c go vet ./...
nix build            # result/bin/hanji
```

## 알려진 한계

- 마크다운은 CommonMark 전체가 아닙니다. 들여쓰기 코드 블록, 참조 링크(`[a][b]`), Setext 제목, 각주는 지원하지 않습니다.
- YAML 은 부분집합입니다 (맵, 목록, 흐름 표기 `[]` `{}`, `|` 블록, 따옴표 문자열). 앵커나 여러 줄 흐름 표기는 없습니다.
- 살아있는 식은 자바스크립트를 이름 단위로만 분석합니다. 화살표 함수의 매개변수 이름이 상태 이름과 같으면 상태로 취급됩니다.
- 코드 강조는 가벼운 범용 렉서입니다 (Go, JS/TS, Rust, Python, Bash, JSON, YAML, CSS, HTML, C, Java, Nix).
- 변경 감지는 300ms 간격의 폴링이고, 변경이 있으면 사이트 전체를 다시 빌드합니다 (스타터 16쪽 기준 약 6ms).

## 라이선스

[Apache License 2.0](LICENSE)
