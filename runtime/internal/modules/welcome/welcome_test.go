package welcome

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
	dmCalls  []string
	dmID     string
}

func (f *fakeClient) CreateMessage(channelID, content string) error {
	f.channels = append(f.channels, channelID)
	f.texts = append(f.texts, content)
	return nil
}

func (f *fakeClient) CreateDM(userID string) (string, error) {
	f.dmCalls = append(f.dmCalls, userID)
	return f.dmID, nil
}

func loadedGuilds() *dispatch.Guilds {
	guilds := dispatch.NewGuilds()
	guilds.Apply(dispatch.Event{Name: "READY", Data: json.RawMessage(`{"guilds":[{"id":"g1"}]}`)})
	guilds.Apply(dispatch.Event{Name: "GUILD_CREATE", Data: json.RawMessage(`{
		"id":"g1",
		"properties":{"name":"Test Guild"},
		"member_count":10,
		"channels":[{"id":"c1","name":"general"}]
	}`)})
	guilds.Apply(dispatch.Event{Name: "GUILD_MEMBER_ADD", Data: json.RawMessage(`{"guild_id":"g1"}`)})
	return guilds
}

func testConfig(message string) config.WelcomeModule {
	return config.WelcomeModule{
		Enabled: true,
		OnJoin: config.WelcomeSection{
			ChannelID: "c1",
			Message:   message,
		},
	}
}

func joinEvent() dispatch.Event {
	return dispatch.Event{Name: "GUILD_MEMBER_ADD", Data: json.RawMessage(`{
		"guild_id":"g1",
		"user":{"id":"u1","username":"ada","global_name":"Ada"}
	}`)}
}

func TestJoinMessageRendersEveryToken(t *testing.T) {
	client := &fakeClient{}
	w := New(testConfig("Welcome {user} ({user.name}, {user.id}) to {guild} ({guild.id}) in {channel}, member #{member.count}"), client, loadedGuilds(), false)

	if err := w.Handle(context.Background(), joinEvent()); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	want := "Welcome <@u1> (Ada, u1) to Test Guild (g1) in general, member #11"
	if len(client.texts) != 1 || client.texts[0] != want {
		t.Errorf("message = %v\nwant %q", client.texts, want)
	}
	if client.channels[0] != "c1" {
		t.Errorf("channel = %q, want c1", client.channels[0])
	}
}

func TestLeaveMessageFallsBackToMention(t *testing.T) {
	cfg := testConfig("")
	cfg.OnLeave = config.WelcomeSection{Enabled: true, ChannelID: "c1", Message: "{user} left {guild} ({user.name})"}
	client := &fakeClient{}
	w := New(cfg, client, loadedGuilds(), false)

	err := w.Handle(context.Background(), dispatch.Event{
		Name: "GUILD_MEMBER_REMOVE",
		Data: json.RawMessage(`{"guild_id":"g1","user":{"id":"u1"}}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	want := "<@u1> left Test Guild (<@u1>)"
	if len(client.texts) != 1 || client.texts[0] != want {
		t.Errorf("message = %v\nwant %q", client.texts, want)
	}
}

func TestJoinAlsoSendsDirectMessage(t *testing.T) {
	cfg := testConfig("Hi {user.name}")
	cfg.OnJoin.DM = true
	client := &fakeClient{dmID: "dm1"}
	w := New(cfg, client, loadedGuilds(), false)

	if err := w.Handle(context.Background(), joinEvent()); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if len(client.dmCalls) != 1 || client.dmCalls[0] != "u1" {
		t.Errorf("dm calls = %v, want [u1]", client.dmCalls)
	}
	if len(client.channels) != 2 || client.channels[0] != "c1" || client.channels[1] != "dm1" {
		t.Errorf("channels = %v, want [c1 dm1]", client.channels)
	}
}

func TestDryRunDoesNotOpenDirectMessage(t *testing.T) {
	cfg := testConfig("Hi")
	cfg.OnJoin.DM = true
	client := &fakeClient{dmID: "dm1"}
	w := New(cfg, client, loadedGuilds(), true)

	if err := w.Handle(context.Background(), joinEvent()); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if len(client.dmCalls) != 0 {
		t.Errorf("dm calls = %v, want none in dry-run", client.dmCalls)
	}
}

func TestLeaveEventOnlyWhenEnabled(t *testing.T) {
	cfg := testConfig("hi")
	w := New(cfg, &fakeClient{}, loadedGuilds(), false)
	if len(w.Events()) != 1 {
		t.Errorf("events = %v, want only GUILD_MEMBER_ADD", w.Events())
	}

	cfg.OnLeave.Enabled = true
	w = New(cfg, &fakeClient{}, loadedGuilds(), false)
	if len(w.Events()) != 2 {
		t.Errorf("events = %v, want join and leave", w.Events())
	}
}
