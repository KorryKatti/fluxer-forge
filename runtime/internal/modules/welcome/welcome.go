// Package welcome sends a message when a member joins or leaves a guild.
package welcome

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/config"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/dispatch"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/template"
)

// Client is the small part of the REST client this module needs.
// It is an interface so tests can use a fake.
type Client interface {
	CreateMessage(channelID, content string) error
	CreateDM(userID string) (string, error)
}

type Welcome struct {
	cfg    config.WelcomeModule
	client Client
	guilds *dispatch.Guilds
	dryRun bool
}

func New(cfg config.WelcomeModule, client Client, guilds *dispatch.Guilds, dryRun bool) *Welcome {
	return &Welcome{cfg: cfg, client: client, guilds: guilds, dryRun: dryRun}
}

func (w *Welcome) Name() string { return "welcome" }

func (w *Welcome) Events() []string {
	events := []string{"GUILD_MEMBER_ADD"}
	if w.cfg.OnLeave.Enabled {
		events = append(events, "GUILD_MEMBER_REMOVE")
	}
	return events
}

type member struct {
	GuildID string `json:"guild_id"`
	User    struct {
		ID         string `json:"id"`
		Username   string `json:"username"`
		GlobalName string `json:"global_name"`
	} `json:"user"`
}

func (w *Welcome) Handle(ctx context.Context, ev dispatch.Event) error {
	var m member
	if err := json.Unmarshal(ev.Data, &m); err != nil {
		return fmt.Errorf("read member: %w", err)
	}
	if m.User.ID == "" || m.GuildID == "" {
		return nil
	}

	var (
		channelID string
		message   string
		viaDM     bool
	)
	switch ev.Name {
	case "GUILD_MEMBER_ADD":
		channelID, message, viaDM = w.cfg.OnJoin.ChannelID, w.cfg.OnJoin.Message, w.cfg.OnJoin.DM
	case "GUILD_MEMBER_REMOVE":
		if !w.cfg.OnLeave.Enabled {
			return nil
		}
		channelID, message = w.cfg.OnLeave.ChannelID, w.cfg.OnLeave.Message
	default:
		return nil
	}
	if channelID == "" || message == "" {
		return nil
	}

	text := w.render(message, m, channelID)

	if err := w.client.CreateMessage(channelID, text); err != nil {
		return fmt.Errorf("post welcome message: %w", err)
	}
	if w.dryRun {
		slog.Info("dry-run: welcome message not sent", "guild", m.GuildID, "user", m.User.ID, "event", ev.Name)
	} else {
		slog.Info("welcome message sent", "guild", m.GuildID, "user", m.User.ID, "event", ev.Name)
	}

	// On join only, optionally repeat the same text as a direct message.
	if ev.Name == "GUILD_MEMBER_ADD" && viaDM {
		if w.dryRun {
			slog.Info("dry-run: would send direct message", "user", m.User.ID)
			return nil
		}
		dmChannel, err := w.client.CreateDM(m.User.ID)
		if err != nil {
			return fmt.Errorf("open direct message: %w", err)
		}
		if err := w.client.CreateMessage(dmChannel, text); err != nil {
			return fmt.Errorf("send direct message: %w", err)
		}
		slog.Info("welcome direct message sent", "user", m.User.ID)
	}
	return nil
}

// render fills the fixed template tokens with this guild and member.
func (w *Welcome) render(text string, m member, channelID string) string {
	info, _ := w.guilds.Get(m.GuildID)
	return template.Render(text, template.Context{
		UserID:      m.User.ID,
		UserName:    m.User.Username,
		DisplayName: m.User.GlobalName,
		GuildID:     m.GuildID,
		GuildName:   info.Name,
		ChannelID:   channelID,
		ChannelName: info.Channels[channelID],
		MemberCount: info.MemberCount,
	})
}
