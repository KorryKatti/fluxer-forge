package config

import (
	"os"
	"strings"
	"testing"
)

// goodConfig is the reference config: everything the frontend can generate
// for the two v0 modules.
const goodConfig = `# a comment
schema_version = 1
instance_url = "https://fluxer.app"

[bot]
prefix = "!"
dry_run = true

[modules.welcome]
enabled = true

[modules.welcome.on_join]
channel_id = "1501314428688998182"
message = "Welcome {user} to {guild}!"
dm = false

[modules.welcome.on_leave]
enabled = true
channel_id = "1501314428688998182"
message = "{user} left {guild}."

[modules.autorole]
enabled = true
roles = ["1501314428688999000"]

[modules.autorole.settings]
delay_seconds = 0
`

func TestParseGoodConfig(t *testing.T) {
	cfg, err := Parse("config.toml", []byte(goodConfig))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", cfg.SchemaVersion)
	}
	if cfg.InstanceURL != "https://fluxer.app" {
		t.Errorf("InstanceURL = %q", cfg.InstanceURL)
	}
	if cfg.Bot.Prefix != "!" || !cfg.Bot.DryRun {
		t.Errorf("Bot = %+v, want prefix ! dry_run true", cfg.Bot)
	}
	if !cfg.JoinActive() {
		t.Error("JoinActive = false, want true")
	}
	if !cfg.LeaveActive() {
		t.Error("LeaveActive = false, want true")
	}
	if got := cfg.Modules.Welcome.OnJoin.ChannelID; got != "1501314428688998182" {
		t.Errorf("OnJoin.ChannelID = %q", got)
	}
	if len(cfg.Modules.Autorole.Roles) != 1 {
		t.Errorf("Roles = %v, want one entry", cfg.Modules.Autorole.Roles)
	}
}

func TestDefaultsWhenKeysAreMissing(t *testing.T) {
	cfg, err := Parse("config.toml", []byte(`schema_version = 1
instance_url = "https://fluxer.app"
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cfg.Bot.DryRun {
		t.Error("DryRun = false, want default true")
	}
	if cfg.Bot.Prefix != "!" {
		t.Errorf("Prefix = %q, want default !", cfg.Bot.Prefix)
	}
}

func TestDisabledModulesAreInactive(t *testing.T) {
	cfg, err := Parse("config.toml", []byte(`schema_version = 1
instance_url = "https://fluxer.app"
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.JoinActive() || cfg.LeaveActive() {
		t.Error("absent welcome module should be inactive")
	}
}

// wantProblem runs Parse and checks that the error mentions every fragment.
func wantProblem(t *testing.T, cfg string, fragments ...string) {
	t.Helper()
	_, err := Parse("config.toml", []byte(cfg))
	if err == nil {
		t.Fatalf("want error, got none (fragments: %v)", fragments)
	}
	for _, fragment := range fragments {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("error missing %q\nerror was:\n%s", fragment, err)
		}
	}
}

func TestMissingSchemaVersion(t *testing.T) {
	wantProblem(t, `instance_url = "https://fluxer.app"`, "schema_version: required")
}

func TestNewerSchemaVersionIsRefused(t *testing.T) {
	wantProblem(t, `schema_version = 2
instance_url = "https://fluxer.app"
`, "newer than this bot supports")
}

func TestBadInstanceURL(t *testing.T) {
	wantProblem(t, `schema_version = 1
instance_url = "fluxer.app"
`, "instance_url", "not an http(s) URL")
}

func TestMissingInstanceURL(t *testing.T) {
	wantProblem(t, `schema_version = 1
`, "instance_url: required")
}

func TestBadSnowflakeNamesPathAndLine(t *testing.T) {
	// channel_id sits on line 10 of this file.
	wantProblem(t, `schema_version = 1
instance_url = "https://fluxer.app"

[bot]
prefix = "!"

[modules.welcome]
enabled = true

[modules.welcome.on_join]
channel_id = "not-a-snowflake"
message = "hi"
`, "modules.welcome.on_join.channel_id", "(config.toml:11)")
}

func TestWelcomeNeedsAChannelAndMessage(t *testing.T) {
	wantProblem(t, `schema_version = 1
instance_url = "https://fluxer.app"

[modules.welcome]
enabled = true

[modules.welcome.on_join]
`, "on_join.channel_id: required", "on_join.message")
}

func TestAutoroleNeedsRolesWhenEnabled(t *testing.T) {
	wantProblem(t, `schema_version = 1
instance_url = "https://fluxer.app"

[modules.autorole]
enabled = true
roles = []
`, "autorole.roles: at least one role")
}

func TestBadRoleInDisabledModuleStillCaught(t *testing.T) {
	wantProblem(t, `schema_version = 1
instance_url = "https://fluxer.app"

[modules.autorole]
enabled = false
roles = ["abc"]
`, "roles[0]", "not a snowflake")
}

func TestUnknownModuleIsIgnoredWithWarning(t *testing.T) {
	// A config from the site with modules this binary does not know yet
	// must still load: unknown keys warn, never fail.
	cfg, err := Parse("config.toml", []byte(goodConfig+`
[modules.sticky_messages]
enabled = true
channel_id = "1"
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cfg.JoinActive() {
		t.Error("known modules should still load")
	}
}

func TestSyntaxErrorReportsLine(t *testing.T) {
	wantProblem(t, `schema_version = 1
instance_url = https://fluxer.app
`, "line 2")
}

func TestAllProblemsReportedAtOnce(t *testing.T) {
	_, err := Parse("config.toml", []byte(`schema_version = 9
instance_url = "nope"
`))
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "2 problem(s)") {
		t.Errorf("want both problems reported, got:\n%s", err)
	}
}

func TestParseReactionRoleRules(t *testing.T) {
	cfg, err := Parse("config.toml", []byte(`schema_version = 1
instance_url = "https://fluxer.app"

[modules.reaction_roles]
enabled = true

[[modules.reaction_roles.rules]]
channel_id = "1501314428688998182"
message_id = "1501320000000000000"
emoji = "👍"
role_id = "1501314428688999001"
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Modules.ReactionRoles.Rules) != 1 {
		t.Fatalf("Rules = %v, want one", cfg.Modules.ReactionRoles.Rules)
	}
	rule := cfg.Modules.ReactionRoles.Rules[0]
	if rule.ChannelID != "1501314428688998182" || rule.Emoji != "👍" || rule.RoleID != "1501314428688999001" {
		t.Errorf("rule = %+v", rule)
	}
}

func TestReactionRoleRuleMustBeComplete(t *testing.T) {
	_, err := Parse("config.toml", []byte(`schema_version = 1
instance_url = "https://fluxer.app"

[modules.reaction_roles]
enabled = true

[[modules.reaction_roles.rules]]
channel_id = "not-a-number"
message_id = ""
emoji = ""
role_id = "also-wrong"
`))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	for _, want := range []string{"rules[0].channel_id", "rules[0].message_id", "rules[0].emoji", "rules[0].role_id"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s:\n%v", want, err)
		}
	}
}

func TestParseCustomCommands(t *testing.T) {
	cfg, err := Parse("config.toml", []byte(`schema_version = 1
instance_url = "https://fluxer.app"

[bot]
prefix = "!"

[modules.custom_commands]
enabled = true

[[modules.custom_commands.items]]
name = "ping"
aliases = ["p"]
cooldown = 3
reply = "Pong!"
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Modules.CustomCommands.Items) != 1 {
		t.Fatalf("Items = %v, want one", cfg.Modules.CustomCommands.Items)
	}
	item := cfg.Modules.CustomCommands.Items[0]
	if item.Name != "ping" || item.Cooldown != 3 || item.Reply != "Pong!" || len(item.Aliases) != 1 {
		t.Errorf("item = %+v", item)
	}
}

func TestCustomCommandMustBeComplete(t *testing.T) {
	_, err := Parse("config.toml", []byte(`schema_version = 1
instance_url = "https://fluxer.app"

[bot]
prefix = ""

[modules.custom_commands]
enabled = true

[[modules.custom_commands.items]]
name = "two words"
reply = ""
cooldown = -1
`))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	for _, want := range []string{"bot.prefix", "items[0].name", "items[0].reply", "items[0].cooldown"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s:\n%v", want, err)
		}
	}
}

func TestParseAutomodLite(t *testing.T) {
	cfg, err := Parse("config.toml", []byte(`schema_version = 1
instance_url = "https://fluxer.app"

[modules.automod_lite]
enabled = true
banned_words = ["spam", "scam"]

[modules.automod_lite.settings]
block_invites = true
max_mentions = 3
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Modules.AutomodLite.BannedWords) != 2 {
		t.Errorf("BannedWords = %v, want two", cfg.Modules.AutomodLite.BannedWords)
	}
	if !cfg.Modules.AutomodLite.Settings.BlockInvites || cfg.Modules.AutomodLite.Settings.MaxMentions != 3 {
		t.Errorf("Settings = %+v", cfg.Modules.AutomodLite.Settings)
	}
}

func TestAutomodRejectsNegativeMentions(t *testing.T) {
	_, err := Parse("config.toml", []byte(`schema_version = 1
instance_url = "https://fluxer.app"

[modules.automod_lite]
enabled = true

[modules.automod_lite.settings]
max_mentions = -1
`))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if !strings.Contains(err.Error(), "max_mentions") {
		t.Errorf("error does not mention max_mentions:\n%v", err)
	}
}

// TestCommittedExampleConfigIsValid keeps config.example.toml runnable: the
// starter file in the repo must always pass the same validation as a
// downloaded config.
func TestCommittedExampleConfigIsValid(t *testing.T) {
	data, err := os.ReadFile("../../../config.example.toml")
	if err != nil {
		t.Fatalf("read config.example.toml: %v", err)
	}
	cfg, err := Parse("config.example.toml", data)
	if err != nil {
		t.Fatalf("config.example.toml is invalid: %v", err)
	}
	if !cfg.Modules.Welcome.Enabled {
		t.Error("example config should have welcome enabled as a working demo")
	}
	if !cfg.Bot.DryRun {
		t.Error("example config must default to dry_run = true")
	}
}
