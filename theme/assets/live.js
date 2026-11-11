// Hanji live runtime.
//
// The compiler turns live markdown into plain HTML with data-hj-* markers and
// a list of tiny functions; this file wires the two together. There is no
// virtual DOM and no dependency graph: after anything changes, every binding
// on the page re-runs and touches the DOM only if its value differs. Pages
// are documents, so this stays cheap — and deep mutations like
// `todos.push(x)` just work.

export function mount(def) {
  const root = document.querySelector("[data-hj-live]:not([data-hj-mounted])")
  if (!root) return
  root.setAttribute("data-hj-mounted", "")

  const fns = def.fns
  const key = "hanji:" + location.pathname
  const state = structuredClone(def.state)
  if (def.persist) {
    try {
      Object.assign(state, JSON.parse(localStorage.getItem(key)) ?? {})
    } catch {}
  }
  // derived values are getters, so they are always current and never saved
  for (const [name, fn] of Object.entries(def.derive)) {
    Object.defineProperty(state, name, { get: () => call(fn, state), enumerable: false })
  }

  const bindings = []
  let queued = false
  const update = () => {
    queued = false
    for (const b of bindings) b()
    if (def.persist) {
      try {
        localStorage.setItem(key, JSON.stringify(state))
      } catch {}
    }
  }
  const schedule = () => {
    if (!queued) {
      queued = true
      queueMicrotask(update)
    }
  }

  function call(fn, scope, arg) {
    try {
      return fn(scope, arg)
    } catch (err) {
      console.error("[hanji]", err.message, "\n  in", fn.toString())
    }
  }

  const show = (v) => (v == null || v === false ? "" : typeof v === "object" ? JSON.stringify(v) : String(v))

  // A row scope: loop variables live here, everything else falls through.
  const child = (parent, locals) =>
    new Proxy(locals, {
      get: (t, k) => (k in t ? t[k] : parent[k]),
      set: (t, k, v) => (k in t ? (t[k] = v) : (parent[k] = v), true),
      has: (t, k) => k in t || k in parent,
    })

  function scan(el, scope, out) {
    if (el.nodeType !== 1) return
    const d = el.dataset
    if (d.hjEach !== undefined) return void out.push(each(el, scope))
    if (d.hjIf !== undefined) {
      const fn = fns[d.hjIf]
      out.push(() => (el.hidden = !call(fn, scope)))
    }
    if (d.hjText !== undefined) {
      const fn = fns[d.hjText]
      let last = el.textContent
      out.push(() => {
        const v = show(call(fn, scope))
        if (v !== last) el.textContent = last = v
      })
    }
    if (d.hjOn !== undefined) {
      el.addEventListener("click", (e) => {
        call(fns[d.hjOn], scope, e)
        schedule()
      })
    }
    if (d.hjModel !== undefined) model(el, scope, out)
    for (const c of el.children) scan(c, scope, out)
  }

  function model(el, scope, out) {
    const get = fns[el.dataset.hjModel]
    const set = fns[el.dataset.hjSet]
    const box = el.type === "checkbox"
    const num = el.type === "number" || el.type === "range"
    const read = () => (box ? el.checked : num ? Number(el.value) : el.value)
    el.addEventListener(box || el.tagName === "SELECT" ? "change" : "input", () => {
      call(set, scope, read())
      schedule()
    })
    if (el.dataset.hjEnter !== undefined) {
      el.addEventListener("keydown", (e) => {
        if (e.key !== "Enter" || e.isComposing) return
        call(fns[el.dataset.hjEnter], scope, e)
        schedule()
      })
    }
    out.push(() => {
      const v = call(get, scope)
      if (box) el.checked = !!v
      else if (num ? Number(el.value) !== v || el.value === "" : el.value !== show(v)) el.value = show(v)
    })
  }

  // Rows are reused by position: growing appends, shrinking removes from the
  // end, and surviving rows just get their item swapped — so focus is kept.
  function each(el, scope) {
    const tpl = el.querySelector(":scope > template")
    const list = fns[el.dataset.hjEach]
    const as = el.dataset.hjAs
    const index = el.dataset.hjIndex
    const rows = []
    return () => {
      const items = Array.from(call(list, scope) ?? [])
      while (rows.length > items.length) for (const n of rows.pop().nodes) n.remove()
      while (rows.length < items.length) {
        const locals = { [as]: undefined }
        if (index) locals[index] = 0
        const frag = tpl.content.cloneNode(true)
        const row = { locals, nodes: [...frag.childNodes], run: [] }
        const rowScope = child(scope, locals)
        for (const n of row.nodes) scan(n, rowScope, row.run)
        el.append(frag)
        rows.push(row)
      }
      rows.forEach((row, i) => {
        row.locals[as] = items[i]
        if (index) row.locals[index] = i
        for (const b of row.run) b()
      })
    }
  }

  scan(root, state, bindings)
  update()
  // escape hatch for custom scripts: change `state`, then call `update()`
  root.dispatchEvent(new CustomEvent("hanji:live", { bubbles: true, detail: { state, update } }))
}
