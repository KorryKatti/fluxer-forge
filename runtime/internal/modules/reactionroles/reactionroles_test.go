package reactionroles

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/config"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/dispatch"
)

type fakeClient struct {
	added   []string
	removed []string
}

func (f *fakeClient) AddMemberRole(guildID, userID, roleID string) error {
	f.added = append(f.added, roleID)
	return nil
}

func (f *fakeClient) RemoveMemberRole(guildID, userID, roleID string) error {
	f.removed = append(f.removed, roleID)
	return nil
}

func testConfig() config.ReactionRolesModule {
	return config.ReactionRolesModule{
		Enabled: true,
		Rules: []config.ReactionRoleRule{
			{ChannelID: "c1", MessageID: "m1", Emoji: "🎮", RoleID: "r1"},
			{ChannelID: "c1", MessageID: "m1", Emoji: "star:custom1", RoleID: "r2"},
		},
	}
}

func TestAddReactionGivesRole(t *testing.T) {
	client := &fakeClient{}
	r := New(testConfig(), client, false)

	err := r.Handle(context.Background(), dispatch.Event{
		Name: "MESSAGE_REACTION_ADD",
		Data: json.RawMessage(`{
			"guild_id":"g1","channel_id":"c1","message_id":"m1","user_id":"u1",
			"emoji":{"name":"🎮"}
		}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(client.added) != 1 || client.added[0] != "r1" {
		t.Errorf("added = %v, want [r1]", client.added)
	}
}

func TestRemoveReactionTakesRoleAway(t *testing.T) {
	client := &fakeClient{}
	r := New(testConfig(), client, false)

	err := r.Handle(context.Background(), dispatch.Event{
		Name: "MESSAGE_REACTION_REMOVE",
		Data: json.RawMessage(`{
			"guild_id":"g1","channel_id":"c1","message_id":"m1","user_id":"u1",
			"emoji":{"name":"🎮"}
		}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(client.removed) != 1 || client.removed[0] != "r1" {
		t.Errorf("removed = %v, want [r1]", client.removed)
	}
}

func TestCustomEmojiMatchesByNameAndID(t *testing.T) {
	client := &fakeClient{}
	r := New(testConfig(), client, false)

	err := r.Handle(context.Background(), dispatch.Event{
		Name: "MESSAGE_REACTION_ADD",
		Data: json.RawMessage(`{
			"guild_id":"g1","channel_id":"c1","message_id":"m1","user_id":"u1",
			"emoji":{"name":"star","id":"custom1"}
		}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(client.added) != 1 || client.added[0] != "r2" {
		t.Errorf("added = %v, want [r2]", client.added)
	}
}

func TestUnknownReactionDoesNothing(t *testing.T) {
	client := &fakeClient{}
	r := New(testConfig(), client, false)

	err := r.Handle(context.Background(), dispatch.Event{
		Name: "MESSAGE_REACTION_ADD",
		Data: json.RawMessage(`{
			"guild_id":"g1","channel_id":"c9","message_id":"m1","user_id":"u1",
			"emoji":{"name":"🎮"}
		}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(client.added) != 0 {
		t.Errorf("added = %v, want none", client.added)
	}
}
