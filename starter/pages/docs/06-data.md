---
title: 데이터와 표현식
---

`{{ }}` 안에 쓰는 식은 빌드할 때 계산됩니다. 자바스크립트와 닮았지만 훨씬 작고, 없는 값을 읽어도
오류 대신 빈 값이 됩니다.

## 쓸 수 있는 값

### `page` — 지금 페이지

| 이름 | 내용 |
| --- | --- |
| `page.title` `page.description` | 제목과 요약. 요약이 없으면 첫 문단에서 만듭니다 |
| `page.url` `page.id` | 주소(`/docs/start/`)와 식별자(`docs/start`) |
| `page.date` `page.tags` | 머리말의 날짜와 태그 |
| `page.section` | 맨 위 폴더 이름 (`docs`) |
| `page.toc` | 제목 목록: `id` `text` `level` |
| `page.prev` `page.next` | 같은 섹션의 앞뒤 문서: `title` `url` |
| `page.readingTime` | 읽는 데 걸리는 시간(분) |
| `page.아무거나` | 머리말에 적은 모든 값 |

### `site` — 사이트

`hanji.yml` 의 모든 값입니다. `site.title`, `site.nav`, 그리고 직접 추가한 값도 읽을 수 있습니다.
`site.dev` 는 개발 서버에서만 참입니다.

### 함수

| 함수 | 결과 |
| --- | --- |
| `pages("blog/*")` | 주소가 무늬에 맞는 페이지들. 날짜가 있으면 최신순, 없으면 순서·제목순 |
| `pages("docs/**")` | `*` 는 한 단계, `**` 는 모든 깊이 |
| `tags()` | 사이트에 쓰인 모든 태그 |
| `sidebar()` | 지금 섹션의 메뉴 나무: `title` `url` `current` `children` |
| `url("/logo.svg")` | `base` 설정을 반영한 주소 |
| `range(3)` | `[0, 1, 2]` |
| `now` | 빌드한 시각 (`now.year`) |

## 연산

```text
page.title + " · " + site.title        이어 붙이기
page.order > 3 && !page.draft          비교와 논리
page.date ? "글" : "문서"              조건
page.cover ?? "/default.png"           없으면 대신
page.tags[0]   page.tags.length        목록
```

## 필터

`|` 로 값을 다음 필터에 넘깁니다. 여러 개를 이어 쓸 수 있습니다.

```text
pages("blog/*") | where("tags", "한지") | limit(5)
page.date | date("YYYY년 M월 D일")
page.description | truncate(80)
```

| 필터 | 하는 일 |
| --- | --- |
| `upper` `lower` `trim` | 글자 바꾸기 |
| `truncate(n)` | n글자에서 자르고 … 붙이기 |
| `default(x)` | 비어 있으면 x |
| `date("YYYY-MM-DD")` | 날짜 모양. `YYYY` `MM` `M` `DD` `D` `HH` `mm` |
| `slug` | 주소에 쓸 수 있는 모양으로 |
| `raw` | HTML을 이스케이프하지 않고 출력 |
| `strip` | HTML 태그 지우기 |
| `json` | JSON 으로 |
| `len` | 길이 |
| `first` `last` `reverse` | 목록 다루기 |
| `limit(n)` `offset(n)` | 앞에서 n개 / n개 건너뛰기 |
| `sort("key")` `sort("key", "desc")` | 정렬 |
| `where("key")` `where("key", 값)` | 걸러 내기. 목록인 값(태그)은 "들어 있는지"를 봅니다 |
| `map("key")` | 각 항목에서 한 값만 뽑기 |
| `join(", ")` `split(",")` | 붙이기 / 나누기 |
| `contains(x)` | 들어 있는지 |

## 한 파일로 여러 페이지 만들기

파일 이름에 `[이름]` 을 넣고 머리말에 `expand` 를 적으면, 목록의 항목마다 페이지가 하나씩 생깁니다.
이 스타터의 태그 페이지가 그렇게 만들어집니다.

```md title="pages/tags/[tag].md"
---
title: "#{{ tag }}"
expand: tag in tags()
---

::: each p in pages("blog/*") | where("tags", tag)
- [{{ p.title }}]({{ p.url }})
:::
```

지금 이 사이트의 태그: {{ tags() | join(", ") }}
