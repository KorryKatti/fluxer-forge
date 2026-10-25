// Package reactionroles gives and removes roles when members react to a
// configured message with a configured emoji.
package reactionroles

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/config"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/dispatch"
)

// Client is the small part of the REST client this module needs.
type Client interface {
	AddMemberRole(guildID, userID, roleID string) error
	RemoveMemberRole(guildID, userID, roleID string) error
}

type ReactionRoles struct {
	cfg    config.ReactionRolesModule
	client Client
	dryRun bool
}

func New(cfg config.ReactionRolesModule, client Client, dryRun bool) *ReactionRoles {
	return &ReactionRoles{cfg: cfg, client: client, dryRun: dryRun}
}

func (r *ReactionRoles) Name() string { return "reaction_roles" }

func (r *ReactionRoles) Events() []string {
	return []string{"MESSAGE_REACTION_ADD", "MESSAGE_REACTION_REMOVE"}
}

type reaction struct {
	GuildID   string `json:"guild_id"`
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
	UserID    string `json:"user_id"`
	Emoji     struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"emoji"`
}

func (r *ReactionRoles) Handle(ctx context.Context, ev dispatch.Event) error {
	var event reaction
	if err := json.Unmarshal(ev.Data, &event); err != nil {
		return fmt.Errorf("read reaction: %w", err)
	}
	if event.GuildID == "" || event.UserID == "" {
		return nil
	}

	rule, ok := r.findRule(event)
	if !ok {
		return nil
	}

	add := ev.Name == "MESSAGE_REACTION_ADD"
	var err error
	if add {
		err = r.client.AddMemberRole(event.GuildID, event.UserID, rule.RoleID)
	} else {
		err = r.client.RemoveMemberRole(event.GuildID, event.UserID, rule.RoleID)
	}
	if err != nil {
		return fmt.Errorf("change role %s: %w", rule.RoleID, err)
	}

	action := "removed"
	if add {
		action = "added"
	}
	if r.dryRun {
		slog.Info("dry-run: reaction role not changed",
			"guild", event.GuildID, "user", event.UserID, "role", rule.RoleID, "action", action)
	} else {
		slog.Info("reaction role changed",
			"guild", event.GuildID, "user", event.UserID, "role", rule.RoleID, "action", action)
	}
	return nil
}

func (r *ReactionRoles) findRule(event reaction) (config.ReactionRoleRule, bool) {
	for _, rule := range r.cfg.Rules {
		if rule.ChannelID != event.ChannelID || rule.MessageID != event.MessageID {
			continue
		}
		if rule.Emoji == event.Emoji.Name {
			return rule, true
		}
		if event.Emoji.ID != "" && rule.Emoji == event.Emoji.Name+":"+event.Emoji.ID {
			return rule, true
		}
	}
	return config.ReactionRoleRule{}, false
}
