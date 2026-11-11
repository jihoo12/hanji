---
title: 살아있는 문서
persist: true
state:
  count: 0
  name: ""
  price: 12000
  people: 3
  tip: 10
  plan: 기본
  draft: ""
  todos:
    - { text: "한지 설치하기", done: true }
    - { text: "첫 페이지 쓰기", done: false }
derive:
  total: Math.round(price * people * (1 + tip / 100))
  left: todos.filter(t => !t.done).length
---

머리말에 `state` 를 적는 순간, 그 페이지의 마크다운은 **반응하는 문서**가 됩니다.
문법은 네 가지뿐입니다.

| 문법 | 뜻 |
| --- | --- |
| `{식}` | 값을 보여 주고, 바뀌면 따라 바뀝니다 |
| `[글자](@코드)` | 누르면 코드를 실행하는 버튼 |
| `[? 이름]` | 상태에 묶인 입력 칸 |
| `::: if 식` · `::: each x in 목록` | 조건부로 보이기, 되풀이하기 |

식과 코드는 평범한 자바스크립트입니다. 상태 이름을 변수처럼 쓰면 됩니다.

## 값과 버튼

```md
---
state:
  count: 0
---

지금 **{count}** 입니다. 두 배는 {count * 2}.

[+1](@count += 1) [-1](@count -= 1) [0으로](@count = 0)
```

지금 **{count}** 입니다. 두 배는 {count * 2}.

[+1](@count += 1) [-1](@count -= 1) [0으로](@count = 0)

`{…}` 는 그 안에 **상태 이름이 들어 있을 때만** 살아있는 값으로 바뀝니다. 그래서 글에 그냥
{이런 중괄호}를 써도 아무 일도 일어나지 않습니다.

## 입력

`[? 이름 | 힌트]` 는 상태의 처음 값을 보고 알맞은 입력 칸이 됩니다.

| 처음 값 / 힌트 | 만들어지는 것 |
| --- | --- |
| 글자 (`name: ""`) | 글 입력 칸. 힌트는 안내 문구가 됩니다 |
| 숫자 (`price: 0`) | 숫자 입력 칸 |
| 참/거짓 (`ok: false`) 또는 힌트 `check` | 체크박스 |
| 힌트 `0..100` · `0..100 step 5` | 슬라이더 |
| 힌트 `가 / 나 / 다` | 선택 상자 |

```md
이름: [? name | 이름을 적어 보세요]

::: if name
반갑습니다, **{name}**님. 이름이 {name.length}글자네요.
:::
```

이름: [? name | 이름을 적어 보세요]

::: if name
반갑습니다, **{name}**님. 이름이 {name.length}글자네요.
:::

## 계산하는 문서

`derive` 에 식을 적어 두면 다른 값에서 **따라 나오는 값**이 됩니다. 견적서나 요금표를 문서 그대로 만들 수 있습니다.

```md
---
state:
  price: 12000
  people: 3
  tip: 10
  plan: 기본
derive:
  total: Math.round(price * people * (1 + tip / 100))
---

1인당 [? price]원 × [? people | 1..10]  {people}명, 봉사료 [? tip | 0..30 step 5] {tip}%

요금제: [? plan | 기본 / 넉넉 / 잔치]

**합계 {total.toLocaleString()}원**
```

1인당 [? price]원 × [? people | 1..10]  {people}명, 봉사료 [? tip | 0..30 step 5] {tip}%

요금제: [? plan | 기본 / 넉넉 / 잔치]

**합계 {total.toLocaleString()}원** — {plan} 요금제

## 되풀이

`::: each` 의 목록이 상태라면, 목록이 바뀔 때마다 줄이 늘고 줄어듭니다.
입력 칸 뒤에 `(@코드)` 를 붙이면 Enter 를 눌렀을 때 실행됩니다.

```md
---
state:
  draft: ""
  todos:
    - { text: "한지 설치하기", done: true }
    - { text: "첫 페이지 쓰기", done: false }
derive:
  left: todos.filter(t => !t.done).length
persist: true
---

[? draft | 할 일을 적고 Enter](@draft && (todos.push({ text: draft, done: false }), draft = ""))

::: each todo, i in todos
[? todo.done | check] {todo.text} [지우기](@todos.splice(i, 1))
:::

남은 일 **{left}개** / 전체 {todos.length}개
```

[? draft | 할 일을 적고 Enter](@draft && (todos.push({ text: draft, done: false }), draft = ""))

::: each todo, i in todos
[? todo.done | check] {todo.text} [지우기](@todos.splice(i, 1))
:::

남은 일 **{left}개** / 전체 {todos.length}개

::: if left == 0
::: tip 다 했습니다
남은 일이 없을 때만 보이는 상자입니다.
:::
:::

## 기억하기

머리말에 `persist: true` 를 적으면 상태를 브라우저에 저장합니다. 이 페이지가 그렇게 되어 있으니,
위에서 값을 바꾸고 새로고침해 보세요. [전부 처음으로](@localStorage.clear(); location.reload())

## 어떻게 동작하나요

컴파일러는 살아있는 표현을 평범한 HTML과 작은 함수 목록으로 바꿉니다.

```md
지금 **{count}** 입니다. [+1](@count += 1)
```

```html
<p>지금 <strong><span data-hj-text="0">0</span></strong> 입니다.
<button data-hj-on="1">+1</button></p>
<script type="module">
  import { mount } from "/_hanji/live.js"
  mount({ state: { count: 0 }, fns: [$ => ($.count), ($, $event) => { $.count += 1 }] })
</script>
```

런타임(`live.js`)은 압축하면 2KB 남짓이고, 가상 DOM도 의존성 그래프도 없습니다. 무언가 바뀌면 페이지의
모든 바인딩을 다시 계산해서 **값이 달라진 곳만** 고칩니다. 문서 한 장 크기에서는 이 방법이 가장
단순하고 충분히 빠르며, `todos.push(…)` 같은 깊은 변경도 그대로 동작합니다.

::: note 알아 둘 점
- 살아있는 표현을 쓰지 않은 페이지에는 `live.js` 가 실리지 않습니다.
- 화살표 함수의 매개변수 이름은 상태 이름과 다르게 지으세요 (`todos.filter(t => …)` 의 `t`).
- 자바스크립트가 꺼진 브라우저에서는 처음 값이 그대로 보입니다.
- 더 복잡한 동작이 필요하면 [컴포넌트](04-components.md)의 `<script>` 를 쓰세요.
  `hanji:live` 이벤트로 `state` 와 `update()` 를 받을 수 있습니다.
:::
