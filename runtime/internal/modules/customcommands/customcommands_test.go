package customcommands

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/config"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/dispatch"
)

type fakeClient struct {
	channels []string
	texts    []string
}

func (f *fakeClient) CreateMessage(channelID, content string) error {
	f.channels = append(f.channels, channelID)
	f.texts = append(f.texts, content)
	return nil
}

func testConfig(cooldown int) config.CustomCommandsModule {
	return config.CustomCommandsModule{
		Enabled: true,
		Items: []config.CustomCommand{
			{Name: "ping", Aliases: []string{"p"}, Cooldown: cooldown, Reply: "Pong!"},
			{Name: "hello", Reply: "Hi {user.name} in {guild}!"},
		},
	}
}

func msgEvent(content string) dispatch.Event {
	data, _ := json.Marshal(map[string]any{
		"guild_id":   "g1",
		"channel_id": "c1",
		"content":    content,
		"author":     map[string]any{"id": "u1", "username": "ada", "global_name": "Ada"},
	})
	return dispatch.Event{Name: "MESSAGE_CREATE", Data: data}
}

func loadedGuilds() *dispatch.Guilds {
	guilds := dispatch.NewGuilds()
	guilds.Apply(dispatch.Event{Name: "READY", Data: json.RawMessage(`{"guilds":[{"id":"g1"}]}`)})
	guilds.Apply(dispatch.Event{Name: "GUILD_CREATE", Data: json.RawMessage(`{
		"id":"g1",
		"properties":{"name":"Test Guild"},
		"member_count":5,
		"channels":[{"id":"c1","name":"general"}]
	}`)})
	return guilds
}

func TestMatchesNameAndAlias(t *testing.T) {
	client := &fakeClient{}
	c := New(testConfig(0), "!", client, loadedGuilds(), false)

	for _, content := range []string{"!ping", "!PING", "!p"} {
		if err := c.Handle(context.Background(), msgEvent(content)); err != nil {
			t.Fatalf("Handle(%q): %v", content, err)
		}
	}
	if len(client.texts) != 3 || client.texts[0] != "Pong!" {
		t.Errorf("replies = %v, want three Pong!", client.texts)
	}
}

func TestIgnoresMessagesWithoutPrefix(t *testing.T) {
	client := &fakeClient{}
	c := New(testConfig(0), "!", client, loadedGuilds(), false)

	if err := c.Handle(context.Background(), msgEvent("ping")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(client.texts) != 0 {
		t.Errorf("replies = %v, want none", client.texts)
	}
}

func TestCooldownBlocksSecondUse(t *testing.T) {
	client := &fakeClient{}
	c := New(testConfig(60), "!", client, loadedGuilds(), false)

	if err := c.Handle(context.Background(), msgEvent("!ping")); err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	if err := c.Handle(context.Background(), msgEvent("!ping")); err != nil {
		t.Fatalf("second Handle: %v", err)
	}
	if len(client.texts) != 1 {
		t.Errorf("replies = %v, want one", client.texts)
	}
}

func TestReplyUsesTemplateTokens(t *testing.T) {
	client := &fakeClient{}
	c := New(testConfig(0), "!", client, loadedGuilds(), false)

	if err := c.Handle(context.Background(), msgEvent("!hello")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	want := "Hi Ada in Test Guild!"
	if len(client.texts) != 1 || client.texts[0] != want {
		t.Errorf("reply = %v, want %q", client.texts, want)
	}
	if client.channels[0] != "c1" {
		t.Errorf("channel = %q, want c1", client.channels[0])
	}
}
