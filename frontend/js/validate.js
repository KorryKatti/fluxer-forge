// Config validation, mirroring runtime/internal/config as closely as a
// 100-line browser check can. Errors block download; warnings do not.

const SNOWFLAKE = /^\d{1,25}$/

function validateConfig(config, modules) {
  const errors = []
  const warnings = []
  const error = (path, message) => errors.push({ path, message })
  const warn = (path, message) => warnings.push({ path, message })
  const idOk = (path, value) => {
    if (!String(value ?? '').trim()) error(path, 'required')
    else if (!SNOWFLAKE.test(String(value).trim())) error(path, 'not a snowflake (digits only)')
  }
  const numberOk = (path, value, label) => {
    const n = Number(value ?? 0)
    if (!Number.isFinite(n) || n < 0) error(path, `${label} cannot be negative`)
  }

  if (!/^https?:\/\/[^\s]+$/.test(String(config.instanceUrl ?? '').trim())) {
    error('instance_url', 'needs to start with http:// or https://')
  }
  if (String(config.prefix ?? '').length > 4) {
    error('bot.prefix', 'longer than 4 characters')
  }

  for (const def of modules) {
    const state = config.modules[def.id]
    if (!state?.enabled) continue
    const at = `modules.${def.id}`

    for (const section of def.sections) {
      const sectionState = state.sections[section.key]
      if (!sectionState) continue
      if (section.toggle && !sectionState.enabled) continue
      for (const field of section.fields) {
        const value = sectionState.values[field.key]
        const path = `${at}.${section.key}.${field.key}`
        if (field.kind === 'id') idOk(path, value)
        if (field.kind === 'number') numberOk(path, value, field.label)
      }
    }

    for (const list of def.lists ?? []) {
      const items = state.lists[list.key] ?? []
      if (!list.scalar && items.length === 0) {
        error(`${at}.${list.key}`, 'at least one entry is required when this module is on')
        continue
      }
      items.forEach((item, index) => {
        const prefix = `${at}.${list.key}[${index}]`
        for (const field of list.fields) {
          const value = item[field.key]
          const path = `${prefix}.${field.key}`
          if (field.kind === 'id') idOk(path, value)
          if (field.kind === 'number') numberOk(path, value, field.label)
          if (field.kind === 'csv') {
            String(value ?? '')
              .split(',')
              .map((part) => part.trim())
              .forEach((alias) => {
                if (!alias || /\s/.test(alias)) error(path, `alias ${JSON.stringify(alias)} cannot be empty or contain spaces`)
              })
          }
        }
      })
    }
  }

  const welcome = config.modules.welcome
  if (welcome?.enabled) {
    const join = welcome.sections.on_join
    if (!String(join?.values.message ?? '').trim()) {
      error('modules.welcome.on_join.message', 'required when welcome is on')
    }
    const leave = welcome.sections.on_leave
    if (leave?.enabled && !String(leave.values.message ?? '').trim()) {
      warn('modules.welcome.on_leave.message', 'empty, so no leave message will be sent')
    }
  }

  const autorole = config.modules.autorole
  if (autorole?.enabled) {
    const roles = (autorole.lists.roles ?? []).map((item) => String(item.value ?? '').trim()).filter(Boolean)
    if (roles.length === 0) {
      error('modules.autorole.roles', 'at least one role is required when autorole is on')
    }
  }

  const commands = config.modules.custom_commands
  if (commands?.enabled) {
    if (!String(config.prefix ?? '').trim()) {
      error('bot.prefix', 'required when custom_commands is on')
    }
    for (const [index, item] of (commands.lists.items ?? []).entries()) {
      const name = String(item.name ?? '')
      if (!name) error(`modules.custom_commands.items[${index}].name`, 'required')
      else if (/\s/.test(name)) error(`modules.custom_commands.items[${index}].name`, 'cannot contain spaces')
      if (!String(item.reply ?? '').trim()) {
        error(`modules.custom_commands.items[${index}].reply`, 'required')
      }
    }
  }

  const reactionRoles = config.modules.reaction_roles
  if (reactionRoles?.enabled) {
    ;(reactionRoles.lists.rules ?? []).forEach((rule, index) => {
      if (!String(rule.emoji ?? '').trim()) {
        error(`modules.reaction_roles.rules[${index}].emoji`, 'required')
      }
    })
  }

  const automod = config.modules.automod_lite
  if (automod?.enabled) {
    const words = (automod.lists.banned_words ?? []).map((item) => String(item.value ?? '').trim())
    words.forEach((word, index) => {
      if (!word) error(`modules.automod_lite.banned_words[${index}]`, 'cannot be empty')
    })
  }

  const enabled = modules.filter((def) => config.modules[def.id]?.enabled).length
  if (enabled === 0) warn('modules', 'no modules are enabled; the bot will connect and do nothing')
  if (!config.dryRun) warn('bot.dry_run', 'live mode: the bot will perform actions')

  return { errors, warnings }
}
