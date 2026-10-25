package schema

import (
	"bytes"
	"os"
	"testing"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/config"
)

// TestCommittedSchemaIsCurrent is the local half of the CI gate: regenerate
// from the structs and fail if schema/schema.json was not updated with them.
func TestCommittedSchemaIsCurrent(t *testing.T) {
	want, err := Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../../schema/schema.json")
	if err != nil {
		t.Fatalf("read committed schema: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("schema/schema.json is stale; run: cd runtime && go run ./cmd/bot -print-schema > ../schema/schema.json")
	}
}

func TestSchemaIsWithinDepthLimit(t *testing.T) {
	doc, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if doc.MaxDepth != 3 {
		t.Errorf("max_depth = %d, want 3", doc.MaxDepth)
	}
	if doc.SchemaVersion != config.SupportedSchemaVersion {
		t.Errorf("schema_version = %d, want %d", doc.SchemaVersion, config.SupportedSchemaVersion)
	}
	for _, want := range []string{"schema_version", "instance_url"} {
		found := false
		for _, name := range doc.Required {
			if name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s is not required", want)
		}
	}
}

func TestModulesAndSnowflakesAreTagged(t *testing.T) {
	doc, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	modules := doc.Properties["modules"]
	if modules == nil {
		t.Fatal("no modules node")
	}
	for _, name := range []string{"welcome", "autorole", "reaction_roles", "custom_commands", "automod_lite"} {
		mod := modules.Properties[name]
		if mod == nil {
			t.Errorf("module %q missing from schema", name)
			continue
		}
		if len(mod.Required) == 0 || mod.Required[0] != "enabled" {
			t.Errorf("module %q does not require enabled", name)
		}
	}

	welcome := modules.Properties["welcome"]
	channel := welcome.Properties["on_join"].Properties["channel_id"]
	if channel.Format != "snowflake" {
		t.Errorf("welcome channel format = %q, want snowflake", channel.Format)
	}
	roles := modules.Properties["autorole"].Properties["roles"]
	if roles.Items == nil || roles.Items.Format != "snowflake" {
		t.Errorf("autorole roles item format = %+v, want snowflake", roles.Items)
	}
}
