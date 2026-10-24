// Fluxer Forge wizard — vanilla state + render.

const STEPS = ['Connection', 'Modules', 'Review']

const params = new URLSearchParams(window.location.search)
const initialStep = Math.min(2, Math.max(0, Number(params.get('step') ?? 0) || 0))

const state = {
  step: initialStep,
  config: createConfig(MODULES),
  expanded: MODULES[0].id,
}

const dl = {
  manifest: undefined, // undefined = loading, null = none, object = loaded
  target: detectTarget(),
  token: '',
  busy: false,
  status: null,
}

const exampleToml = buildExampleToml(createConfig(MODULES), MODULES)

const $stepper = document.getElementById('stepper')
const $content = document.getElementById('card-content')
const $back = document.getElementById('btn-back')
const $next = document.getElementById('btn-next')
const $spacer = document.getElementById('btn-spacer')
const $footNote = document.getElementById('foot-note')

function esc(value) {
  return String(value).replace(
    /[&<>"]/g,
    (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c],
  )
}

function instanceValid() {
  return /^https?:\/\/[^\s]+$/.test(state.config.instanceUrl.trim())
}

// ---------- state mutations ----------

function patchModule(id, update) {
  state.config.modules[id] = update(state.config.modules[id])
}

function toggleModule(id, enabled) {
  patchModule(id, (m) => ({ ...m, enabled }))
  if (enabled) state.expanded = id
}

function setField(moduleId, sectionKey, fieldKey, value) {
  patchModule(moduleId, (m) => ({
    ...m,
    sections: {
      ...m.sections,
      [sectionKey]: { ...m.sections[sectionKey], values: { ...m.sections[sectionKey].values, [fieldKey]: value } },
    },
  }))
}

function setSectionEnabled(moduleId, sectionKey, enabled) {
  patchModule(moduleId, (m) => ({
    ...m,
    sections: { ...m.sections, [sectionKey]: { ...m.sections[sectionKey], enabled } },
  }))
}

function addItem(moduleId, listKey) {
  const def = MODULES.find((m) => m.id === moduleId)
  if (!def) return
  patchModule(moduleId, (m) => ({
    ...m,
    lists: { ...m.lists, [listKey]: [...(m.lists[listKey] ?? []), emptyListItem(def, listKey)] },
  }))
}

function removeItem(moduleId, listKey, index) {
  patchModule(moduleId, (m) => ({
    ...m,
    lists: { ...m.lists, [listKey]: (m.lists[listKey] ?? []).filter((_, i) => i !== index) },
  }))
}

function setItemField(moduleId, listKey, index, fieldKey, value) {
  patchModule(moduleId, (m) => ({
    ...m,
    lists: {
      ...m.lists,
      [listKey]: (m.lists[listKey] ?? []).map((item, i) => (i === index ? { ...item, [fieldKey]: value } : item)),
    },
  }))
}

// ---------- field rendering ----------

function fieldHtml(field, value, ctx) {
  const attrs = [
    `data-mod="${esc(ctx.mod)}"`,
    `data-field="${esc(field.key)}"`,
    `data-kind="${esc(field.kind)}"`,
  ]
  if (ctx.sec) attrs.push(`data-sec="${esc(ctx.sec)}"`)
  if (ctx.list) attrs.push(`data-list="${esc(ctx.list)}"`, `data-idx="${ctx.idx}"`)
  const data = attrs.join(' ')

  if (field.kind === 'bool') {
    return `<div class="field full">
      <label class="check" for="f-${esc(ctx.uid)}">
        <input id="f-${esc(ctx.uid)}" type="checkbox" ${data} ${value ? 'checked' : ''} />
        <span>${esc(field.label)}</span>
      </label>
      ${field.help ? `<p class="hint">${esc(field.help)}</p>` : ''}
    </div>`
  }

  const full = field.kind === 'textarea' ? ' full' : ''
  let control
  if (field.kind === 'textarea') {
    control = `<textarea id="f-${esc(ctx.uid)}" rows="3" ${data}${
      field.placeholder ? ` placeholder="${esc(field.placeholder)}"` : ''
    }>${esc(String(value))}</textarea>`
  } else {
    const type = field.kind === 'number' ? 'number' : 'text'
    const inputMode = field.kind === 'id' ? ' inputmode="numeric"' : ''
    const cls = field.kind === 'id' ? ' class="mono"' : ''
    control = `<input id="f-${esc(ctx.uid)}" type="${type}"${cls}${inputMode} ${data} value="${esc(
      String(value),
    )}"${field.placeholder ? ` placeholder="${esc(field.placeholder)}"` : ''} />`
  }

  return `<div class="field${full}">
    <label for="f-${esc(ctx.uid)}">${esc(field.label)}</label>
    ${control}
    ${field.help ? `<p class="hint">${esc(field.help)}</p>` : ''}
  </div>`
}

function switchHtml(checked, label, act, extra) {
  return `<button type="button" role="switch" aria-checked="${checked}" aria-label="${esc(
    label,
  )}" class="sw" data-act="${act}" ${extra}></button>`
}

function listHtml(list, items, modId) {
  const count = `${items.length} ${items.length === 1 ? 'entry' : 'entries'}`
  let body = ''
  if (list.note) body += `<p class="hint wide">${esc(list.note)}</p>`
  if (items.length === 0) body += `<p class="empty">Nothing here yet.</p>`

  items.forEach((item, index) => {
    const fields = list.fields
      .map((field) =>
        fieldHtml(
          { ...field, label: list.scalar ? list.itemLabel : field.label },
          item[field.key] ?? '',
          { mod: modId, list: list.key, idx: index, field, uid: `${modId}-${list.key}-${index}-${field.key}` },
        ),
      )
      .join('')
    body += `<div class="list-item">
      <div class="list-item-head">
        <span class="mono dim">${esc(list.itemLabel)} ${index + 1}</span>
        <button type="button" class="link" data-act="remove-item" data-mod="${esc(modId)}" data-list="${esc(
          list.key,
        )}" data-idx="${index}">Remove</button>
      </div>
      <div class="${list.scalar ? 'list-grid one' : 'list-grid'}">${fields}</div>
    </div>`
  })

  return `<div class="sect">
    <div class="sect-head">
      <h3>${esc(list.label)}</h3>
      <span class="count">${count}</span>
    </div>
    <div class="sect-body single">
      ${body}
      <button type="button" class="btn add" data-act="add-item" data-mod="${esc(modId)}" data-list="${esc(
        list.key,
      )}">+ Add ${esc(list.itemLabel.toLowerCase())}</button>
    </div>
  </div>`
}

function moduleCardHtml(def, index) {
  const mstate = state.config.modules[def.id]
  const expanded = state.expanded === def.id
  const cls = `mcard${mstate.enabled ? ' on' : ''}${expanded ? ' open' : ''}`

  let body = ''
  if (expanded) {
    if (!mstate.enabled) {
      body += `<p class="banner">This module is off. Settings are kept but left out of <code>config.toml</code>.</p>`
    }

    for (const section of def.sections) {
      const sstate = mstate.sections[section.key]
      if (!sstate) continue
      const dimmed = section.toggle && !sstate.enabled
      const fields = section.fields
        .map((field) =>
          fieldHtml(field, sstate.values[field.key] ?? '', {
            mod: def.id,
            sec: section.key,
            field,
            uid: `${def.id}-${section.key}-${field.key}`,
          }),
        )
        .join('')
      body += `<div class="sect">
        <div class="sect-head">
          <h3>${esc(section.label)}</h3>
          ${
            section.toggle
              ? switchHtml(sstate.enabled, `Enable ${section.label}`, 'section-toggle',
                  `data-mod="${esc(def.id)}" data-sec="${esc(section.key)}"`)
              : ''
          }
        </div>
        <div class="sect-body${dimmed ? ' dimmed' : ''}">${fields}</div>
      </div>`
    }

    for (const list of def.lists ?? []) {
      body += listHtml(list, mstate.lists[list.key] ?? [], def.id)
    }
  }

  return `<div class="${cls}" data-card="${esc(def.id)}">
    <div class="mcard-head">
      ${switchHtml(mstate.enabled, `Enable ${def.name}`, 'toggle-module', `data-mod="${esc(def.id)}"`)}
      <div class="mcard-text" role="button" tabindex="0" data-act="expand" data-mod="${esc(def.id)}">
        <span class="mcard-name">
          <span class="idx">${String(index + 1).padStart(2, '0')}</span>
          ${esc(def.name)}
          <span class="stage ${esc(def.stage)}">${esc(def.stage)}</span>
        </span>
        <span class="mcard-blurb">${esc(def.blurb)}</span>
      </div>
      <button type="button" class="chev" aria-expanded="${expanded}" aria-label="${
        expanded ? `Collapse ${esc(def.name)}` : `Expand ${esc(def.name)}`
      }" data-act="expand" data-mod="${esc(def.id)}">${expanded ? '−' : '+'}</button>
    </div>
    ${expanded ? `<div class="mcard-body">${body}</div>` : ''}
  </div>`
}

// ---------- steps ----------

function step0Html() {
  const cfg = state.config
  const valid = instanceValid()
  return `<div class="card-head">
      <span class="kicker">Step 1 of 3</span>
      <h1>Connection</h1>
      <p>Point the bot at the Fluxer instance it will run against. The site never contacts it — only the binary does, from your machine.</p>
    </div>
    <div class="card-body">
      <div class="form-grid">
        <div class="field full">
          <label for="instance">Instance URL</label>
          <input id="instance" type="text" class="mono" spellcheck="false" data-bind="instance" value="${esc(
            cfg.instanceUrl,
          )}" />
          <p class="hint${valid ? '' : ' bad'}" id="instance-hint">${
            valid
              ? 'Discovery is read from /.well-known/fluxer by the bot at startup.'
              : 'Needs to start with http:// or https://'
          }</p>
        </div>
        <div class="field">
          <label for="prefix">Command prefix</label>
          <input id="prefix" type="text" class="mono" maxlength="4" data-bind="prefix" value="${esc(cfg.prefix)}" />
          <p class="hint">Fluxer has no slash commands.</p>
        </div>
        <div class="field">
          <label class="check" for="dry-run">
            <input id="dry-run" type="checkbox" data-bind="dryRun" ${cfg.dryRun ? 'checked' : ''} />
            <span>Dry run</span>
          </label>
          <p class="hint">Log actions instead of sending them. Keep this on first.</p>
        </div>
      </div>
    </div>`
}

function step1Html() {
  const included = enabledModuleCount(state.config)
  const cards = MODULES.map((def, index) => moduleCardHtml(def, index)).join('')
  return `<div class="card-head">
      <span class="kicker">Step 2 of 3</span>
      <h1>Modules</h1>
      <p>Turn on what you need and fill in the details. Everything stays in the browser until you download.</p>
      <span class="counter">${included} of ${MODULES.length} enabled</span>
    </div>
    <div class="card-body">${cards}</div>`
}

function previewLineHtml(text) {
  if (!text.trim()) return '​'
  const trimmed = text.trimStart()
  if (trimmed.startsWith('#')) return `<span class="t-com">${esc(text)}</span>`
  if (trimmed.startsWith('[')) return `<span class="t-head">${esc(text)}</span>`

  const eq = text.indexOf(' = ')
  if (eq > -1) {
    const key = text.slice(0, eq)
    const value = text.slice(eq + 3)
    const cls = value.startsWith('"') || value.startsWith('[') ? 't-str' : 't-num'
    return `<span class="t-key">${esc(key)}</span><span class="t-eq"> = </span><span class="${cls}">${esc(
      value,
    )}</span>`
  }
  return esc(text)
}

function webBase(instanceUrl) {
  try {
    const url = new URL(instanceUrl)
    if (url.host === 'fluxer.app' || url.host === 'www.fluxer.app') return 'https://web.fluxer.app'
    return instanceUrl.replace(/\/+$/, '')
  } catch {
    return instanceUrl.replace(/\/+$/, '')
  }
}

function clientIdFromToken(token) {
  const first = token.trim().split('.')[0]
  return /^\d{5,}$/.test(first) ? first : null
}

function inviteUrl() {
  const clientId = clientIdFromToken(dl.token)
  if (!clientId) return null
  return `${webBase(state.config.instanceUrl)}/oauth2/authorize?client_id=${clientId}&scope=bot`
}

function step2Html() {
  const cfg = state.config
  const toml = buildToml(cfg, MODULES)
  const lines = toml.trimEnd().split('\n')

  const summary = `<div class="summary">
    <span class="tag">${enabledModuleCount(cfg)} modules</span>
    <span class="tag">${cfg.dryRun ? 'dry run on' : 'live'}</span>
    <span class="tag mono">${esc(cfg.instanceUrl)}</span>
  </div>`

  const preview = `<div class="preview">
    <div class="pre-head">
      <span>config.toml</span>
      <span>${lines.length} lines</span>
    </div>
    <pre class="code">${lines
      .map((line, index) => `<div class="code-line"><span class="ln">${index + 1}</span><span class="lc">${previewLineHtml(
        line,
      )}</span></div>`)
      .join('')}</pre>
    <button type="button" class="btn ghost copy" id="btn-copy">${dl.copied ? 'Copied' : 'Copy config'}</button>
  </div>`

  const available = dl.manifest ? Object.keys(dl.manifest.targets) : [...TARGETS]
  if (!available.includes(dl.target)) dl.target = available[0]
  const invite = inviteUrl()

  const download = `<div class="dl">
    <div class="dl-row">
      <label class="field-inline">
        <span>Bot token</span>
        <input type="password" data-bind="token" value="${esc(dl.token)}" placeholder="optional — writes .env" autocomplete="off" spellcheck="false" />
      </label>
      <label class="field-inline narrow">
        <span>Target</span>
        <select data-bind="target">${available
          .map((name) => `<option value="${esc(name)}"${name === dl.target ? ' selected' : ''}>${esc(name)}</option>`)
          .join('')}</select>
      </label>
    </div>
    <div class="dl-actions">
      <button type="button" class="btn primary" id="btn-zip" ${dl.busy ? 'disabled' : ''}>${
        dl.busy ? 'Packing…' : 'Download ZIP'
      }</button>
      <button type="button" class="btn ghost" id="btn-toml">config.toml only</button>
    </div>
    ${
      invite
        ? `<a class="invite" id="invite-link" href="${esc(invite)}" target="_blank" rel="noreferrer">Open bot invite →</a>`
        : `<a class="invite" id="invite-link" hidden></a>`
    }
    ${dl.status ? `<p class="status">${esc(dl.status)}</p>` : ''}
    <p class="fine">${
      dl.manifest === undefined
        ? 'Checking for a published build…'
        : dl.manifest
          ? `Binary ${dl.manifest.version ?? 'unknown'} · schema ${dl.manifest.schema_version ?? '?'}`
          : 'No build published yet — the ZIP will carry config, scripts and Dockerfile.'
    }</p>
    <p class="fine">Nothing is uploaded. The token stays in this tab.</p>
  </div>`

  return `<div class="card-head">
      <span class="kicker">Step 3 of 3</span>
      <h1>Review &amp; download</h1>
      <p>This is exactly what goes in the archive. Edit anything by going back a step; the file updates live.</p>
    </div>
    <div class="card-body">
      ${summary}
      ${preview}
      ${download}
    </div>`
}

// ---------- render ----------

function renderStepper() {
  $stepper.innerHTML = STEPS.map(
    (name, index) => `<button type="button" class="step${index < state.step ? ' done' : ''}"${
      index === state.step ? ' aria-current="step"' : ''
    } data-step="${index}">
      <span class="n">${index < state.step ? '✓' : index + 1}</span>${name}
    </button>`,
  ).join('')
}

function renderFoot() {
  $back.disabled = state.step === 0
  $footNote.textContent =
    state.step === 0
      ? 'No account, no upload.'
      : state.step === 1
        ? 'Ids are strings. Unknown keys are ignored.'
        : 'Check the binary target before downloading.'

  const last = state.step === STEPS.length - 1
  $next.hidden = last
  $spacer.hidden = !last
  $next.disabled = state.step === 0 && !instanceValid()
}

function render() {
  renderStepper()
  renderCard()
  renderFoot()
}

function renderCard() {
  $content.innerHTML =
    state.step === 0 ? step0Html() : state.step === 1 ? step1Html() : step2Html()
  if (state.step === 2 && dl.manifest === undefined) loadManifestAndMaybeRender()
}

function goStep(step) {
  state.step = Math.min(STEPS.length - 1, Math.max(0, step))
  render()
  window.scrollTo({ top: 0 })
}

let manifestPromise = null
function loadManifestAndMaybeRender() {
  if (manifestPromise) return
  manifestPromise = loadManifest().then((data) => {
    dl.manifest = data
    if (state.step === 2) renderCard()
  })
}

// ---------- events ----------

$content.addEventListener('keydown', (event) => {
  if ((event.key === 'Enter' || event.key === ' ') && event.target.matches('.mcard-text')) {
    event.preventDefault()
    event.target.click()
  }
})

$back.addEventListener('click', () => goStep(state.step - 1))
$next.addEventListener('click', () => {
  if (state.step === 0 && !instanceValid()) return
  goStep(state.step + 1)
})

$stepper.addEventListener('click', (event) => {
  const btn = event.target.closest('[data-step]')
  if (btn) goStep(Number(btn.dataset.step))
})

$content.addEventListener('input', (event) => {
  const el = event.target
  const d = el.dataset

  if (d.bind === 'instance') {
    state.config.instanceUrl = el.value
    const valid = instanceValid()
    const hint = document.getElementById('instance-hint')
    if (hint) {
      hint.classList.toggle('bad', !valid)
      hint.textContent = valid
        ? 'Discovery is read from /.well-known/fluxer by the bot at startup.'
        : 'Needs to start with http:// or https://'
    }
    $next.disabled = state.step === 0 && !valid
    return
  }
  if (d.bind === 'prefix') {
    state.config.prefix = el.value
    return
  }
  if (d.bind === 'dryRun') {
    state.config.dryRun = el.checked
    return
  }
  if (d.bind === 'token') {
    dl.token = el.value
    updateInvite()
    return
  }

  if (!d.mod || !d.field) return
  const kind = d.kind
  let value
  if (kind === 'bool') value = el.checked
  else if (kind === 'number') value = el.value === '' ? '' : Number(el.value)
  else value = el.value

  if (d.sec) setField(d.mod, d.sec, d.field, value)
  else if (d.list) setItemField(d.mod, d.list, Number(d.idx), d.field, value)
})

$content.addEventListener('change', (event) => {
  if (event.target.dataset.bind === 'target') dl.target = event.target.value
})

function updateInvite() {
  const link = document.getElementById('invite-link')
  if (!link) return
  const url = inviteUrl()
  if (url) {
    link.href = url
    link.target = '_blank'
    link.rel = 'noreferrer'
    link.textContent = 'Open bot invite →'
    link.hidden = false
  } else {
    link.hidden = true
  }
}

$content.addEventListener('click', async (event) => {
  const actEl = event.target.closest('[data-act]')
  if (actEl) {
    const d = actEl.dataset
    if (d.act === 'expand') {
      state.expanded = state.expanded === d.mod ? null : d.mod
      renderCard()
      return
    }
    if (d.act === 'toggle-module') {
      const current = state.config.modules[d.mod].enabled
      toggleModule(d.mod, !current)
      renderCard()
      renderFoot()
      return
    }
    if (d.act === 'section-toggle') {
      const current = state.config.modules[d.mod].sections[d.sec].enabled
      setSectionEnabled(d.mod, d.sec, !current)
      actEl.setAttribute('aria-checked', String(!current))
      const sect = actEl.closest('.sect')
      if (sect) {
        const def = MODULES.find((m) => m.id === d.mod)
        const section = def.sections.find((s) => s.key === d.sec)
        const body = sect.querySelector('.sect-body')
        if (section?.toggle && body) body.classList.toggle('dimmed', current)
      }
      return
    }
    if (d.act === 'add-item') {
      addItem(d.mod, d.list)
      renderCard()
      return
    }
    if (d.act === 'remove-item') {
      removeItem(d.mod, d.list, Number(d.idx))
      renderCard()
      return
    }
  }

  if (event.target.id === 'btn-copy') {
    try {
      await navigator.clipboard.writeText(buildToml(state.config, MODULES))
      dl.copied = true
      event.target.textContent = 'Copied'
      setTimeout(() => {
        dl.copied = false
        const btn = document.getElementById('btn-copy')
        if (btn) btn.textContent = 'Copy config'
      }, 1400)
    } catch {
      /* clipboard unavailable */
    }
    return
  }

  if (event.target.id === 'btn-toml') {
    downloadBlob(new Blob([buildToml(state.config, MODULES)], { type: 'text/plain' }), 'config.toml')
    return
  }

  if (event.target.id === 'btn-zip') {
    const tokenInput = $content.querySelector('[data-bind="token"]')
    dl.token = tokenInput ? tokenInput.value : dl.token
    dl.busy = true
    dl.status = null
    renderCard()
    try {
      const result = await buildZip(buildToml(state.config, MODULES), exampleToml, {
        token: dl.token,
        target: dl.target,
        manifest: dl.manifest ?? null,
      })
      downloadBlob(result.blob, 'fluxer-bot.zip')
      const kb = Math.max(1, Math.round(result.blob.size / 1024))
      dl.status = `${result.names.length} files · ${kb} KB${result.binaryIncluded ? '' : ' · binary not bundled'}`
    } catch (error) {
      dl.status = error instanceof Error ? error.message : 'Could not build the archive'
    } finally {
      dl.busy = false
      if (state.step === 2) renderCard()
    }
  }
})

render()
