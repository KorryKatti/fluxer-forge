// Package config loads and validates config.toml.
//
// The structs below are the single definition of what a config can say.
// Anything deeper than they allow simply cannot be written, so a config can
// never grow a fourth nesting level or an unknown field we would choke on.
package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

// SupportedSchemaVersion is the only schema_version this binary accepts.
const SupportedSchemaVersion = 1

type Config struct {
	SchemaVersion int     `toml:"schema_version"`
	InstanceURL   string  `toml:"instance_url"`
	Bot           Bot     `toml:"bot"`
	Modules       Modules `toml:"modules"`
}

type Bot struct {
	Prefix string `toml:"prefix"`
	DryRun bool   `toml:"dry_run"`
}

type Modules struct {
	Welcome        WelcomeModule        `toml:"welcome"`
	Autorole       AutoroleModule       `toml:"autorole"`
	ReactionRoles  ReactionRolesModule  `toml:"reaction_roles"`
	CustomCommands CustomCommandsModule `toml:"custom_commands"`
	AutomodLite    AutomodLiteModule    `toml:"automod_lite"`
}

type WelcomeModule struct {
	Enabled bool           `toml:"enabled"`
	OnJoin  WelcomeSection `toml:"on_join"`
	OnLeave WelcomeSection `toml:"on_leave"`
}

// WelcomeSection holds both the join and the leave message. OnJoin has no
// "enabled" line of its own (it follows the module); OnLeave does.
type WelcomeSection struct {
	Enabled   bool   `toml:"enabled"`
	ChannelID string `toml:"channel_id"`
	Message   string `toml:"message"`
	DM        bool   `toml:"dm"`
}

type AutoroleModule struct {
	Enabled  bool     `toml:"enabled"`
	Roles    []string `toml:"roles"`
	Settings struct {
		DelaySeconds int `toml:"delay_seconds"`
	} `toml:"settings"`
}

type ReactionRolesModule struct {
	Enabled bool               `toml:"enabled"`
	Rules   []ReactionRoleRule `toml:"rules"`
}

type ReactionRoleRule struct {
	ChannelID string `toml:"channel_id"`
	MessageID string `toml:"message_id"`
	Emoji     string `toml:"emoji"`
	RoleID    string `toml:"role_id"`
}

type CustomCommandsModule struct {
	Enabled bool            `toml:"enabled"`
	Items   []CustomCommand `toml:"items"`
}

type CustomCommand struct {
	Name     string   `toml:"name"`
	Aliases  []string `toml:"aliases"`
	Cooldown int      `toml:"cooldown"`
	Reply    string   `toml:"reply"`
}

type AutomodLiteModule struct {
	Enabled     bool     `toml:"enabled"`
	BannedWords []string `toml:"banned_words"`
	Settings    struct {
		BlockInvites bool `toml:"block_invites"`
		MaxMentions  int  `toml:"max_mentions"`
	} `toml:"settings"`
}

// JoinActive says whether welcome messages on join should be sent.
func (c *Config) JoinActive() bool {
	return c.Modules.Welcome.Enabled
}

// LeaveActive says whether leave messages should be sent.
func (c *Config) LeaveActive() bool {
	return c.Modules.Welcome.Enabled && c.Modules.Welcome.OnLeave.Enabled
}

// Parse reads a config file's contents. filename is only used in messages.
//
// It returns every problem at once — nobody wants to fix one error and
// rerun just to meet the next one.
func Parse(filename string, data []byte) (*Config, error) {
	var cfg Config
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}

	// Defaults for keys the file never mentions.
	if !md.IsDefined("bot", "dry_run") {
		cfg.Bot.DryRun = true // safe default: log instead of acting
	}
	if !md.IsDefined("bot", "prefix") {
		cfg.Bot.Prefix = "!"
	}

	warnUndecoded(md.Undecoded())

	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	at := func(path string) string {
		return where(filename, string(data), path)
	}

	if cfg.SchemaVersion == 0 {
		add("schema_version: required, add \"schema_version = %d\"", SupportedSchemaVersion)
	} else if cfg.SchemaVersion > SupportedSchemaVersion {
		add("schema_version: %d is newer than this bot supports (max %d) — update the bot",
			cfg.SchemaVersion, SupportedSchemaVersion)
	}

	if cfg.InstanceURL == "" {
		add("instance_url: required")
	} else if u, err := url.Parse(cfg.InstanceURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		add("instance_url: %q is not an http(s) URL %s", cfg.InstanceURL, at("instance_url"))
	}

	if utf8.RuneCountInString(cfg.Bot.Prefix) > 4 {
		add("bot.prefix: %q is longer than 4 characters %s", cfg.Bot.Prefix, at("bot.prefix"))
	}

	join := cfg.Modules.Welcome.OnJoin
	leave := cfg.Modules.Welcome.OnLeave

	// checkChannel: an empty id is only a problem when the section is live;
	// a present id must always be a snowflake (catch typos while disabled too).
	checkChannel := func(section string, id string, required bool) {
		path := "modules.welcome." + section + ".channel_id"
		switch {
		case id == "" && required:
			add("%s: required when this section is on %s", path, at(path))
		case id != "" && !isSnowflake(id):
			add("%s: %q is not a snowflake %s", path, id, at(path))
		}
	}
	checkChannel("on_join", join.ChannelID, cfg.Modules.Welcome.Enabled)
	checkChannel("on_leave", leave.ChannelID, cfg.JoinActive() && leave.Enabled)

	if cfg.Modules.Welcome.Enabled && join.Message == "" {
		add("modules.welcome.on_join.message: required when the welcome module is on %s",
			at("modules.welcome.on_join.message"))
	}

	if cfg.Modules.Autorole.Enabled && len(cfg.Modules.Autorole.Roles) == 0 {
		add("modules.autorole.roles: at least one role is required when autorole is on %s",
			at("modules.autorole.roles"))
	}
	for i, role := range cfg.Modules.Autorole.Roles {
		if !isSnowflake(role) {
			add("modules.autorole.roles[%d]: %q is not a snowflake %s", i, role, at("modules.autorole.roles"))
		}
	}
	if cfg.Modules.Autorole.Settings.DelaySeconds < 0 {
		add("modules.autorole.settings.delay_seconds: cannot be negative %s",
			at("modules.autorole.settings.delay_seconds"))
	}

	if cfg.Modules.ReactionRoles.Enabled && len(cfg.Modules.ReactionRoles.Rules) == 0 {
		add("modules.reaction_roles.rules: at least one rule is required when reaction_roles is on %s",
			at("modules.reaction_roles.rules"))
	}
	for i, rule := range cfg.Modules.ReactionRoles.Rules {
		prefix := fmt.Sprintf("modules.reaction_roles.rules[%d]", i)
		if !isSnowflake(rule.ChannelID) {
			add("%s.channel_id: %q is not a snowflake %s", prefix, rule.ChannelID, at(prefix+".channel_id"))
		}
		if !isSnowflake(rule.MessageID) {
			add("%s.message_id: %q is not a snowflake %s", prefix, rule.MessageID, at(prefix+".message_id"))
		}
		if !isSnowflake(rule.RoleID) {
			add("%s.role_id: %q is not a snowflake %s", prefix, rule.RoleID, at(prefix+".role_id"))
		}
		if rule.Emoji == "" {
			add("%s.emoji: required %s", prefix, at(prefix+".emoji"))
		}
	}

	if cfg.Modules.CustomCommands.Enabled && cfg.Bot.Prefix == "" {
		add("bot.prefix: required when custom_commands is on %s", at("bot.prefix"))
	}
	if cfg.Modules.CustomCommands.Enabled && len(cfg.Modules.CustomCommands.Items) == 0 {
		add("modules.custom_commands.items: at least one command is required when custom_commands is on %s",
			at("modules.custom_commands.items"))
	}
	for i, item := range cfg.Modules.CustomCommands.Items {
		prefix := fmt.Sprintf("modules.custom_commands.items[%d]", i)
		if item.Name == "" {
			add("%s.name: required %s", prefix, at(prefix+".name"))
		} else if strings.ContainsAny(item.Name, " \t") {
			add("%s.name: %q cannot contain spaces %s", prefix, item.Name, at(prefix+".name"))
		}
		for j, alias := range item.Aliases {
			if alias == "" || strings.ContainsAny(alias, " \t") {
				add("%s.aliases[%d]: %q cannot be empty or contain spaces %s",
					prefix, j, alias, at(prefix+".aliases"))
			}
		}
		if item.Reply == "" {
			add("%s.reply: required %s", prefix, at(prefix+".reply"))
		}
		if item.Cooldown < 0 {
			add("%s.cooldown: cannot be negative %s", prefix, at(prefix+".cooldown"))
		}
	}

	if cfg.Modules.AutomodLite.Settings.MaxMentions < 0 {
		add("modules.automod_lite.settings.max_mentions: cannot be negative %s",
			at("modules.automod_lite.settings.max_mentions"))
	}
	for i, word := range cfg.Modules.AutomodLite.BannedWords {
		if strings.TrimSpace(word) == "" {
			add("modules.automod_lite.banned_words[%d]: cannot be empty %s",
				i, at("modules.automod_lite.banned_words"))
		}
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("%s has %d problem(s):\n%s", filename, len(problems), strings.Join(problems, "\n"))
	}
	return &cfg, nil
}

// warnUndecoded logs keys the structs did not consume. Unknown keys are a
// warning, never an error: a config written by a newer site must still run.
func warnUndecoded(keys []toml.Key) {
	reported := map[string]bool{}
	for _, key := range keys {
		name := key.String()
		parts := key
		// Group everything under a whole module, e.g. all
		// modules.reaction_roles.* keys become one warning.
		if len(parts) > 1 && parts[0] == "modules" {
			name = "modules." + parts[1]
		}
		if reported[name] {
			continue
		}
		reported[name] = true
		slog.Warn("ignoring unknown config key", "key", name)
	}
}

// isSnowflake reports whether s is an all-digits id, e.g. "1501314428688998182".
func isSnowflake(s string) bool {
	if s == "" || len(s) > 25 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// where returns "(config.toml:14)" for a path like
// "modules.welcome.on_join.channel_id", or "(config.toml)" if we cannot
// find the line. It scans the raw text: the TOML library only exposes line
// numbers for syntax errors, not for values that are merely wrong.
func where(filename, data, path string) string {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return "(" + filename + ")"
	}
	wantSection := strings.Join(parts[:len(parts)-1], ".")
	wantKey := parts[len(parts)-1]

	section := ""
	for i, line := range strings.Split(data, "\n") {
		text := strings.TrimSpace(line)
		if strings.HasPrefix(text, "[") {
			section = strings.Trim(text, "[]") // works for [a.b] and [[a.b]]
			continue
		}
		if section != wantSection {
			continue
		}
		if text == wantKey || strings.HasPrefix(text, wantKey+" ") || strings.HasPrefix(text, wantKey+"=") {
			return fmt.Sprintf("(%s:%d)", filename, i+1)
		}
	}
	return "(" + filename + ")"
}
