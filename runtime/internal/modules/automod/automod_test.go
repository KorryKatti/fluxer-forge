package automod

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/config"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/dispatch"
)

type fakeClient struct {
	deleted []string
}

func (f *fakeClient) DeleteMessage(channelID, messageID string) error {
	f.deleted = append(f.deleted, messageID)
	return nil
}

func testConfig() config.AutomodLiteModule {
	cfg := config.AutomodLiteModule{
		Enabled:     true,
		BannedWords: []string{"spam"},
	}
	cfg.Settings.BlockInvites = true
	cfg.Settings.MaxMentions = 2
	return cfg
}

func msgEvent(content string) dispatch.Event {
	data, _ := json.Marshal(map[string]any{
		"id":         "m1",
		"guild_id":   "g1",
		"channel_id": "c1",
		"content":    content,
	})
	return dispatch.Event{Name: "MESSAGE_CREATE", Data: data}
}

func TestDeletesInvites(t *testing.T) {
	client := &fakeClient{}
	a := New(testConfig(), client, false)

	if err := a.Handle(context.Background(), msgEvent("join https://fluxer.app/invite/abc123")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(client.deleted) != 1 {
		t.Errorf("deleted = %v, want one", client.deleted)
	}
}

func TestDeletesBannedWordsCaseInsensitive(t *testing.T) {
	client := &fakeClient{}
	a := New(testConfig(), client, false)

	if err := a.Handle(context.Background(), msgEvent("this is SPAM")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(client.deleted) != 1 {
		t.Errorf("deleted = %v, want one", client.deleted)
	}
}

func TestDeletesMentionFlood(t *testing.T) {
	client := &fakeClient{}
	a := New(testConfig(), client, false)

	data, _ := json.Marshal(map[string]any{
		"id":         "m1",
		"guild_id":   "g1",
		"channel_id": "c1",
		"content":    "hi",
		"mentions":   []any{map[string]any{"id": "1"}, map[string]any{"id": "2"}, map[string]any{"id": "3"}},
	})
	if err := a.Handle(context.Background(), dispatch.Event{Name: "MESSAGE_CREATE", Data: data}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(client.deleted) != 1 {
		t.Errorf("deleted = %v, want one", client.deleted)
	}
}

func TestKeepsNormalMessages(t *testing.T) {
	client := &fakeClient{}
	a := New(testConfig(), client, false)

	if err := a.Handle(context.Background(), msgEvent("hello everyone")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(client.deleted) != 0 {
		t.Errorf("deleted = %v, want none", client.deleted)
	}
}

func TestIgnoresDirectMessages(t *testing.T) {
	client := &fakeClient{}
	a := New(testConfig(), client, false)

	data, _ := json.Marshal(map[string]any{
		"id":         "m1",
		"channel_id": "c1",
		"content":    "spam",
	})
	if err := a.Handle(context.Background(), dispatch.Event{Name: "MESSAGE_CREATE", Data: data}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(client.deleted) != 0 {
		t.Errorf("deleted = %v, want none", client.deleted)
	}
}
