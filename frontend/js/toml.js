// config.toml serialization.

function tomlQuote(value) {
  return JSON.stringify(value)
}

function tomlAsNumber(value) {
  const n = Number(value)
  return Number.isFinite(n) ? String(n) : '0'
}

function tomlSerialize(kind, value) {
  switch (kind) {
    case 'bool':
      return value ? 'true' : 'false'
    case 'number':
      return tomlAsNumber(value)
    case 'csv': {
      const parts = String(value)
        .split(',')
        .map((p) => p.trim())
        .filter(Boolean)
        .map(tomlQuote)
      return `[${parts.join(', ')}]`
    }
    default:
      return tomlQuote(String(value))
  }
}

function tomlFieldDefault(field) {
  if (field.default !== undefined) return field.default
  return field.kind === 'bool' ? false : field.kind === 'number' ? 0 : ''
}

function tomlScalarItem(list, item) {
  const field = list.fields[0]
  return tomlSerialize(field?.kind ?? 'text', item[field?.key ?? 'value'] ?? '')
}

function buildToml(config, modules, options = {}) {
  const lines = []

  if (options.header !== false) {
    lines.push('# Fluxer Forge — generated configuration')
    lines.push('# Start with: ./bot --config config.toml')
    lines.push('')
  }

  lines.push('schema_version = 1')
  lines.push(`instance_url = ${tomlQuote(config.instanceUrl)}`)
  lines.push('')
  lines.push('[bot]')
  lines.push(`prefix = ${tomlQuote(config.prefix)}`)
  lines.push(`dry_run = ${config.dryRun ? 'true' : 'false'}`)

  for (const def of modules) {
    const state = config.modules[def.id]
    if (!state) continue
    if (!state.enabled && !options.includeDisabled) continue

    lines.push('')
    lines.push(`[modules.${def.id}]`)
    lines.push(`enabled = ${state.enabled ? 'true' : 'false'}`)

    for (const list of def.lists ?? []) {
      if (!list.scalar) continue
      const items = state.lists[list.key] ?? []
      const values = items.map((item) => tomlScalarItem(list, item)).filter((v) => v !== '""' && v !== '[]')
      lines.push(`${list.key} = [${values.join(', ')}]`)
    }

    for (const section of def.sections) {
      const sectionState = state.sections[section.key]
      if (!sectionState) continue
      lines.push('')
      lines.push(`[modules.${def.id}.${section.key}]`)
      if (section.toggle) lines.push(`enabled = ${sectionState.enabled ? 'true' : 'false'}`)
      for (const field of section.fields) {
        const value = sectionState.values[field.key] ?? tomlFieldDefault(field)
        if (field.kind === 'id' && String(value).trim() === '') continue
        lines.push(`${field.key} = ${tomlSerialize(field.kind, value)}`)
      }
    }

    for (const list of def.lists ?? []) {
      if (list.scalar) continue
      const items = state.lists[list.key] ?? []
      for (const item of items) {
        lines.push('')
        lines.push(`[[modules.${def.id}.${list.key}]]`)
        for (const field of list.fields) {
          const value = item[field.key] ?? tomlFieldDefault(field)
          lines.push(`${field.key} = ${tomlSerialize(field.kind, value)}`)
        }
      }
    }
  }

  return lines.join('\n') + '\n'
}

function buildExampleToml(config, modules) {
  return buildToml(config, modules, { includeDisabled: true })
}
