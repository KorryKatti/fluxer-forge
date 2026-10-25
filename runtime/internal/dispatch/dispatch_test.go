package dispatch

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/fluxer"
)

type fakeModule struct {
	name   string
	events []string
	got    []string
	panic  bool
}

func (f *fakeModule) Name() string     { return f.name }
func (f *fakeModule) Events() []string { return f.events }

func (f *fakeModule) Handle(ctx context.Context, ev Event) error {
	f.got = append(f.got, ev.Name)
	if f.panic {
		panic("boom")
	}
	return nil
}

func event(name string, data string) fluxer.Event {
	return fluxer.Event{Name: name, Data: json.RawMessage(data)}
}

func ready(guildIDs ...string) fluxer.Event {
	guilds := []map[string]any{}
	for _, id := range guildIDs {
		guilds = append(guilds, map[string]any{"id": id, "unavailable": true})
	}
	data, _ := json.Marshal(map[string]any{"guilds": guilds})
	return event("READY", string(data))
}

func guildCreate(id, name string) fluxer.Event {
	data, _ := json.Marshal(map[string]any{
		"id":           id,
		"properties":   map[string]any{"name": name},
		"member_count": 10,
		"channels":     []map[string]any{{"id": "chan1", "name": "general"}},
	})
	return event("GUILD_CREATE", string(data))
}

func TestOnlySubscribedEventsReachModules(t *testing.T) {
	guilds := NewGuilds()
	mod := &fakeModule{name: "test", events: []string{"GUILD_MEMBER_ADD"}}
	d := New("bot", guilds, mod)
	ctx := context.Background()

	d.Dispatch(ctx, ready("g1"))
	d.Dispatch(ctx, guildCreate("g1", "Test"))
	d.Dispatch(ctx, event("GUILD_MEMBER_REMOVE", `{"guild_id":"g1","user":{"id":"u1"}}`))
	d.Dispatch(ctx, event("GUILD_MEMBER_ADD", `{"guild_id":"g1","user":{"id":"u1"}}`))

	if len(mod.got) != 1 || mod.got[0] != "GUILD_MEMBER_ADD" {
		t.Errorf("module saw %v, want only GUILD_MEMBER_ADD", mod.got)
	}
}

func TestMessageFromBotOrWebhookIsDropped(t *testing.T) {
	guilds := NewGuilds()
	mod := &fakeModule{name: "test", events: []string{"MESSAGE_CREATE"}}
	d := New("bot", guilds, mod)
	ctx := context.Background()

	d.Dispatch(ctx, ready("g1"))
	d.Dispatch(ctx, guildCreate("g1", "Test"))

	d.Dispatch(ctx, event("MESSAGE_CREATE", `{"guild_id":"g1","author":{"id":"u1","bot":true}}`))
	d.Dispatch(ctx, event("MESSAGE_CREATE", `{"guild_id":"g1","author":{"id":"u1"},"webhook_id":"w1"}`))
	d.Dispatch(ctx, event("MESSAGE_CREATE", `{"guild_id":"g1","author":{"id":"bot"}}`))
	d.Dispatch(ctx, event("MESSAGE_CREATE", `{"guild_id":"g1","author":{"id":"u1"}}`))

	if len(mod.got) != 1 {
		t.Errorf("module saw %d messages, want 1", len(mod.got))
	}
}

func TestEventsFromUnknownGuildAreDropped(t *testing.T) {
	guilds := NewGuilds()
	mod := &fakeModule{name: "test", events: []string{"GUILD_MEMBER_ADD"}}
	d := New("bot", guilds, mod)
	ctx := context.Background()

	d.Dispatch(ctx, ready("g1"))
	d.Dispatch(ctx, event("GUILD_MEMBER_ADD", `{"guild_id":"other","user":{"id":"u1"}}`))
	d.Dispatch(ctx, event("GUILD_MEMBER_ADD", `{"guild_id":"g1","user":{"id":"u1"}}`))

	if len(mod.got) != 1 {
		t.Errorf("module saw %d events, want 1", len(mod.got))
	}
}

func TestMemberCountFollowsJoinsAndLeaves(t *testing.T) {
	guilds := NewGuilds()
	d := New("bot", guilds, &fakeModule{name: "test"})
	ctx := context.Background()

	d.Dispatch(ctx, ready("g1"))
	d.Dispatch(ctx, guildCreate("g1", "Test"))
	d.Dispatch(ctx, event("GUILD_MEMBER_ADD", `{"guild_id":"g1","user":{"id":"u1"}}`))

	info, _ := guilds.Get("g1")
	if info.MemberCount != 11 {
		t.Errorf("member count after join = %d, want 11", info.MemberCount)
	}

	d.Dispatch(ctx, event("GUILD_MEMBER_REMOVE", `{"guild_id":"g1","user":{"id":"u1"}}`))
	info, _ = guilds.Get("g1")
	if info.MemberCount != 10 {
		t.Errorf("member count after leave = %d, want 10", info.MemberCount)
	}
}

func TestGuildDeleteRemovesGuild(t *testing.T) {
	guilds := NewGuilds()
	mod := &fakeModule{name: "test", events: []string{"GUILD_MEMBER_ADD"}}
	d := New("bot", guilds, mod)
	ctx := context.Background()

	d.Dispatch(ctx, ready("g1"))
	d.Dispatch(ctx, event("GUILD_DELETE", `{"id":"g1"}`))
	d.Dispatch(ctx, event("GUILD_MEMBER_ADD", `{"guild_id":"g1","user":{"id":"u1"}}`))

	if len(mod.got) != 0 {
		t.Errorf("module saw %d events after guild delete, want 0", len(mod.got))
	}
}

func TestModulePanicDoesNotStopTheDispatcher(t *testing.T) {
	guilds := NewGuilds()
	bad := &fakeModule{name: "bad", events: []string{"GUILD_MEMBER_ADD"}, panic: true}
	good := &fakeModule{name: "good", events: []string{"GUILD_MEMBER_ADD"}}
	d := New("bot", guilds, bad, good)
	ctx := context.Background()

	d.Dispatch(ctx, ready("g1"))
	d.Dispatch(ctx, event("GUILD_MEMBER_ADD", `{"guild_id":"g1","user":{"id":"u1"}}`))

	if len(good.got) != 1 {
		t.Errorf("good module saw %d events, want 1", len(good.got))
	}
}
