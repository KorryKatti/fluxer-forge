package autorole

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/config"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/dispatch"
)

type fakeClient struct {
	guilds []string
	users  []string
	roles  []string
}

func (f *fakeClient) AddMemberRole(guildID, userID, roleID string) error {
	f.guilds = append(f.guilds, guildID)
	f.users = append(f.users, userID)
	f.roles = append(f.roles, roleID)
	return nil
}

func TestGrantsEveryConfiguredRole(t *testing.T) {
	cfg := config.AutoroleModule{Enabled: true, Roles: []string{"r1", "r2"}}
	client := &fakeClient{}
	a := New(cfg, client, false)

	err := a.Handle(context.Background(), dispatch.Event{
		Name: "GUILD_MEMBER_ADD",
		Data: json.RawMessage(`{"guild_id":"g1","user":{"id":"u1"}}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if len(client.roles) != 2 || client.roles[0] != "r1" || client.roles[1] != "r2" {
		t.Errorf("roles = %v, want [r1 r2]", client.roles)
	}
	if client.guilds[0] != "g1" || client.users[0] != "u1" {
		t.Errorf("gave role in %q to %q, want g1/u1", client.guilds[0], client.users[0])
	}
}

func TestIgnoresEventWithoutMember(t *testing.T) {
	client := &fakeClient{}
	a := New(config.AutoroleModule{Enabled: true, Roles: []string{"r1"}}, client, false)

	err := a.Handle(context.Background(), dispatch.Event{
		Name: "GUILD_MEMBER_ADD",
		Data: json.RawMessage(`{"guild_id":"g1"}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(client.roles) != 0 {
		t.Errorf("roles = %v, want none", client.roles)
	}
}
