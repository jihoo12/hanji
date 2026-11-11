// Hanji site runtime: instant navigation, search, theme, toc, copy.
// Everything is delegated from `document`, because navigation swaps <body>.

const base = document.querySelector('meta[name="hanji-base"]')?.content ?? "/"
const root = document.documentElement

// ---- theme ----
function toggleTheme() {
  const dark = root.dataset.theme ? root.dataset.theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches
  root.dataset.theme = dark ? "light" : "dark"
  try {
    localStorage.setItem("hanji-theme", root.dataset.theme)
  } catch {}
}

// ---- per-page setup ----
let spy
function init() {
  root.classList.remove("hj-menu-open")
  // mark the current section in the top nav
  const here = location.pathname
  let best = null
  for (const a of document.querySelectorAll(".hj-nav a")) {
    a.removeAttribute("aria-current")
    const p = new URL(a.href).pathname
    if (p !== base && here.startsWith(p) && (!best || p.length > new URL(best.href).pathname.length)) best = a
  }
  best?.setAttribute("aria-current", "true")
  document.querySelector(".hj-sidenav a[aria-current]")?.scrollIntoView({ block: "nearest" })

  // table of contents follows the scroll position
  spy?.disconnect()
  const links = new Map([...document.querySelectorAll(".hj-toc a")].map((a) => [decodeURIComponent(a.hash.slice(1)), a]))
  if (links.size) {
    const visible = new Set()
    spy = new IntersectionObserver(
      (entries) => {
        for (const e of entries) e.isIntersecting ? visible.add(e.target.id) : visible.delete(e.target.id)
        const first = [...links.keys()].find((id) => visible.has(id))
        if (first) for (const [id, a] of links) a.classList.toggle("active", id === first)
      },
      { rootMargin: "-64px 0px -60% 0px" },
    )
    for (const id of links.keys()) {
      const h = document.getElementById(id)
      if (h) spy.observe(h)
    }
  }
}

// ---- instant navigation ----
const cache = new Map()
const pageKey = (url) => new URL(url, location.href).href.split("#")[0]
function load(url) {
  const k = pageKey(url)
  if (!cache.has(k)) {
    cache.set(
      k,
      fetch(k)
        .then((r) => (r.ok && r.headers.get("content-type")?.includes("text/html") ? r.text() : null))
        .catch(() => null),
    )
  }
  return cache.get(k)
}

function internal(a) {
  if (!a || a.target || a.hasAttribute("download") || a.origin !== location.origin) return false
  if (!a.pathname.startsWith(base) || /\.[a-z0-9]+$/i.test(a.pathname)) return false
  return true
}

let current = pageKey(location.href)
async function navigate(href, push) {
  const html = await load(href)
  if (!html) return void (location.href = href)
  const doc = new DOMParser().parseFromString(html, "text/html")
  const swap = () => {
    document.title = doc.title
    for (const s of document.querySelectorAll("style[data-hj-components]")) s.remove()
    for (const s of doc.querySelectorAll("style[data-hj-components]")) document.head.append(s)
    document.body.replaceChildren(...doc.body.childNodes)
    // scripts inserted by a swap don't run; recreate them so they do
    for (const old of document.body.querySelectorAll("script")) {
      const s = document.createElement("script")
      for (const at of old.attributes) s.setAttribute(at.name, at.value)
      s.textContent = old.textContent
      old.replaceWith(s)
    }
  }
  if (push) history.pushState({}, "", href)
  current = pageKey(href)
  if (document.startViewTransition) await document.startViewTransition(swap).updateCallbackDone
  else swap()
  const hash = new URL(href, location.href).hash
  const target = hash && document.getElementById(decodeURIComponent(hash.slice(1)))
  if (target) target.scrollIntoView()
  else scrollTo({ top: 0, behavior: "instant" })
  init()
  document.dispatchEvent(new CustomEvent("hanji:page"))
}

addEventListener("popstate", () => {
  if (pageKey(location.href) !== current) navigate(location.href, false)
})
document.addEventListener("pointerover", (e) => {
  const a = e.target.closest?.("a[href]")
  if (internal(a)) load(a.href)
})

// ---- search ----
let index
let selected = 0
const dialog = () => document.getElementById("hj-search")
const escapeHTML = (s) => s.replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c])

async function openSearch() {
  const d = dialog()
  if (!d || d.open) return
  d.showModal()
  const input = d.querySelector("input")
  input.value = ""
  renderResults("")
  input.focus()
  index ??= await fetch(base + "_hanji/search.json")
    .then((r) => r.json())
    .catch(() => [])
}

function renderResults(q) {
  const list = dialog().querySelector("ul")
  const terms = q.toLowerCase().split(/\s+/).filter(Boolean)
  if (!terms.length || !index) return void (list.innerHTML = "")
  const mark = (s) => {
    let out = escapeHTML(s)
    for (const t of terms) out = out.replace(new RegExp(escapeHTML(t).replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "gi"), "<mark>$&</mark>")
    return out
  }
  const hits = []
  for (const p of index) {
    const title = p.title.toLowerCase()
    const heads = (p.headings ?? []).join(" ").toLowerCase()
    const text = p.text.toLowerCase()
    let score = 0
    for (const t of terms) {
      const s = (title.includes(t) ? 10 : 0) + (heads.includes(t) ? 4 : 0) + (text.includes(t) ? 1 : 0)
      if (!s) {
        score = 0
        break
      }
      score += s
    }
    if (!score) continue
    const at = text.indexOf(terms[0])
    const from = Math.max(0, at - 30)
    const snippet = at < 0 ? p.text.slice(0, 90) : (from ? "…" : "") + p.text.slice(from, from + 110)
    hits.push({ score, p, snippet })
  }
  hits.sort((a, b) => b.score - a.score)
  selected = 0
  list.innerHTML = hits.length
    ? hits
        .slice(0, 8)
        .map((h, i) => `<li><a href="${escapeHTML(h.p.url)}" aria-selected="${i === 0}"><strong>${mark(h.p.title)}</strong><span>${mark(h.snippet)}</span></a></li>`)
        .join("")
    : `<li class="empty">${escapeHTML((root.dataset.hjNoResults ?? "%s").replace("%s", q))}</li>`
}

function moveSelection(delta) {
  const items = [...dialog().querySelectorAll("li a")]
  if (!items.length) return
  selected = (selected + delta + items.length) % items.length
  items.forEach((a, i) => a.setAttribute("aria-selected", i === selected))
  items[selected].scrollIntoView({ block: "nearest" })
}

// ---- delegated events ----
document.addEventListener("click", (e) => {
  const t = e.target
  if (t.closest("[data-hj-theme]")) return toggleTheme()
  if (t.closest("[data-hj-menu]")) return root.classList.toggle("hj-menu-open")
  if (t.closest("[data-hj-search]")) return openSearch()
  if (t === dialog()) return dialog().close() // click on the backdrop

  const copy = t.closest(".hj-copy")
  if (copy) {
    const code = copy.closest(".hj-code")?.querySelector("code")
    navigator.clipboard?.writeText(code.innerText.replace(/\n$/, "")).then(() => {
      copy.textContent = root.dataset.hjCopied ?? "Copied"
      setTimeout(() => (copy.textContent = root.dataset.hjCopy ?? "Copy"), 1500)
    })
    return
  }

  const a = t.closest("a[href]")
  if (!a || e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
  if (!internal(a)) return
  if (pageKey(a.href) === current) {
    if (!a.hash) e.preventDefault() // already here
    dialog()?.close()
    return // let the browser handle in-page #anchors
  }
  e.preventDefault()
  navigate(a.href, true)
})

document.addEventListener("input", (e) => {
  if (e.target.closest("#hj-search")) renderResults(e.target.value)
})

document.addEventListener("keydown", (e) => {
  const d = dialog()
  if (d?.open) {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault()
      moveSelection(e.key === "ArrowDown" ? 1 : -1)
    } else if (e.key === "Enter" && !e.isComposing) {
      e.preventDefault()
      d.querySelector('li a[aria-selected="true"]')?.click()
    }
    return
  }
  const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName) || e.target.isContentEditable
  if ((e.key === "/" && !typing) || (e.key.toLowerCase() === "k" && (e.metaKey || e.ctrlKey))) {
    e.preventDefault()
    openSearch()
  }
})

init()
