// Package autorole gives configured roles to every member who joins.
package autorole

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/config"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/dispatch"
)

// Client is the small part of the REST client this module needs.
type Client interface {
	AddMemberRole(guildID, userID, roleID string) error
}

type Autorole struct {
	cfg    config.AutoroleModule
	client Client
	dryRun bool
}

func New(cfg config.AutoroleModule, client Client, dryRun bool) *Autorole {
	return &Autorole{cfg: cfg, client: client, dryRun: dryRun}
}

func (a *Autorole) Name() string { return "autorole" }

func (a *Autorole) Events() []string {
	return []string{"GUILD_MEMBER_ADD"}
}

type joined struct {
	GuildID string `json:"guild_id"`
	User    struct {
		ID string `json:"id"`
	} `json:"user"`
}

func (a *Autorole) Handle(ctx context.Context, ev dispatch.Event) error {
	var m joined
	if err := json.Unmarshal(ev.Data, &m); err != nil {
		return fmt.Errorf("read member: %w", err)
	}
	if m.GuildID == "" || m.User.ID == "" {
		return nil
	}

	if a.cfg.Settings.DelaySeconds > 0 {
		delay := time.Duration(a.cfg.Settings.DelaySeconds) * time.Second
		go func() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			if err := a.apply(m.GuildID, m.User.ID); err != nil {
				slog.Error("autorole failed", "user", m.User.ID, "error", err)
			}
		}()
		return nil
	}

	return a.apply(m.GuildID, m.User.ID)
}

func (a *Autorole) apply(guildID, userID string) error {
	for _, roleID := range a.cfg.Roles {
		if err := a.client.AddMemberRole(guildID, userID, roleID); err != nil {
			return fmt.Errorf("grant role %s: %w", roleID, err)
		}
		if a.dryRun {
			slog.Info("dry-run: role not granted", "guild", guildID, "user", userID, "role", roleID)
		} else {
			slog.Info("role granted", "guild", guildID, "user", userID, "role", roleID)
		}
	}
	return nil
}
