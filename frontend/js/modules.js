// Module definitions and config state helpers.

const MODULES = [
  {
    id: 'welcome',
    name: 'Welcome & leave',
    blurb: 'Greet new members and announce departures.',
    stage: 'v0',
    defaultEnabled: true,
    permissions: ['view_channel', 'send_messages'],
    sections: [
      {
        key: 'on_join',
        label: 'On join',
        fields: [
          {
            key: 'channel_id',
            label: 'Channel',
            kind: 'id',
            placeholder: '1501314428688998182',
            help: 'Right-click a channel → Copy ID.',
          },
          {
            key: 'message',
            label: 'Message',
            kind: 'textarea',
            default: 'Welcome {user} to {guild}!',
            help: 'Placeholders: {user} {user.name} {user.id} {guild} {guild.id} {channel} {member.count}',
          },
          {
            key: 'dm',
            label: 'Also send as a DM',
            kind: 'bool',
            default: false,
          },
        ],
      },
      {
        key: 'on_leave',
        label: 'On leave',
        toggle: true,
        defaultEnabled: false,
        fields: [
          {
            key: 'channel_id',
            label: 'Channel',
            kind: 'id',
            placeholder: '1501314428688998182',
          },
          {
            key: 'message',
            label: 'Message',
            kind: 'textarea',
            default: '{user} left {guild}.',
          },
        ],
      },
    ],
  },
  {
    id: 'autorole',
    name: 'Auto role',
    blurb: 'Give members a role the moment they join.',
    stage: 'v0',
    defaultEnabled: true,
    permissions: ['manage_roles'],
    sections: [
      {
        key: 'settings',
        label: 'Settings',
        fields: [
          {
            key: 'delay_seconds',
            label: 'Delay (seconds)',
            kind: 'number',
            default: 0,
            help: '0 applies immediately.',
          },
        ],
      },
    ],
    lists: [
      {
        key: 'roles',
        label: 'Roles to grant',
        itemLabel: 'Role',
        scalar: true,
        note: 'The bot’s own role must sit above these.',
        fields: [{ key: 'value', label: 'Role ID', kind: 'id', placeholder: '1501314428688998182' }],
      },
    ],
  },
  {
    id: 'reaction_roles',
    name: 'Reaction roles',
    blurb: 'Members pick roles by reacting to a message.',
    stage: 'v1',
    permissions: ['view_channel', 'add_reactions', 'manage_roles'],
    sections: [],
    lists: [
      {
        key: 'rules',
        label: 'Rules',
        itemLabel: 'Rule',
        fields: [
          { key: 'channel_id', label: 'Channel ID', kind: 'id' },
          { key: 'message_id', label: 'Message ID', kind: 'id' },
          { key: 'emoji', label: 'Emoji', kind: 'text', placeholder: '🎮' },
          { key: 'role_id', label: 'Role ID', kind: 'id' },
        ],
      },
    ],
  },
  {
    id: 'custom_commands',
    name: 'Custom commands',
    blurb: 'Prefix commands that reply with text or an embed.',
    stage: 'v1',
    permissions: ['view_channel', 'send_messages', 'embed_links'],
    sections: [],
    lists: [
      {
        key: 'items',
        label: 'Commands',
        itemLabel: 'Command',
        fields: [
          { key: 'name', label: 'Name', kind: 'text', placeholder: 'rules' },
          { key: 'aliases', label: 'Aliases', kind: 'csv', placeholder: 'rule, info' },
          { key: 'cooldown', label: 'Cooldown (s)', kind: 'number', default: 3 },
          { key: 'reply', label: 'Reply', kind: 'textarea', default: 'Read the rules in #rules.' },
        ],
      },
    ],
  },
  {
    id: 'automod_lite',
    name: 'AutoMod lite',
    blurb: 'Drop invites, banned words and mention floods.',
    stage: 'v1',
    permissions: ['view_channel', 'manage_messages'],
    sections: [
      {
        key: 'settings',
        label: 'Filters',
        fields: [
          { key: 'block_invites', label: 'Block invites', kind: 'bool', default: true },
          { key: 'max_mentions', label: 'Max mentions', kind: 'number', default: 5 },
        ],
      },
    ],
    lists: [
      {
        key: 'banned_words',
        label: 'Banned words',
        itemLabel: 'Word',
        scalar: true,
        fields: [{ key: 'value', label: 'Word', kind: 'text' }],
      },
    ],
  },
]

function fieldDefault(field) {
  if (field.default !== undefined) return field.default
  return field.kind === 'bool' ? false : field.kind === 'number' ? 0 : ''
}

function createSectionState(def) {
  const values = {}
  for (const field of def.fields) values[field.key] = fieldDefault(field)
  return { enabled: def.defaultEnabled ?? true, values }
}

function createModuleState(def) {
  const sections = {}
  for (const section of def.sections) sections[section.key] = createSectionState(section)

  const lists = {}
  for (const list of def.lists ?? []) lists[list.key] = []

  return { enabled: def.defaultEnabled ?? false, sections, lists }
}

function createConfig(modules) {
  const state = {}
  for (const def of modules) state[def.id] = createModuleState(def)
  return {
    instanceUrl: 'https://fluxer.app',
    prefix: '!',
    dryRun: true,
    modules: state,
  }
}

function emptyListItem(def, listKey) {
  const list = (def.lists ?? []).find((l) => l.key === listKey)
  const item = {}
  for (const field of list?.fields ?? []) item[field.key] = fieldDefault(field)
  return item
}

function enabledModuleCount(config) {
  return Object.values(config.modules).filter((m) => m.enabled).length
}
