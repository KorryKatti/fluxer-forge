// Package welcome sends a message when a member joins or leaves a guild.
package welcome

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/config"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/dispatch"
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

	var section config.WelcomeSection
	switch ev.Name {
	case "GUILD_MEMBER_ADD":
		section = w.cfg.OnJoin
	case "GUILD_MEMBER_REMOVE":
		section = w.cfg.OnLeave
	default:
		return nil
	}
	if section.ChannelID == "" || section.Message == "" {
		return nil
	}

	text := w.render(section.Message, m, section.ChannelID)

	if err := w.client.CreateMessage(section.ChannelID, text); err != nil {
		return fmt.Errorf("post welcome message: %w", err)
	}
	if w.dryRun {
		slog.Info("dry-run: welcome message not sent", "guild", m.GuildID, "user", m.User.ID, "event", ev.Name)
	} else {
		slog.Info("welcome message sent", "guild", m.GuildID, "user", m.User.ID, "event", ev.Name)
	}

	// On join only, optionally repeat the same text as a direct message.
	if ev.Name == "GUILD_MEMBER_ADD" && section.DM {
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

// render fills the fixed template tokens. There is no scripting: an unknown
// {token} is left exactly as written.
func (w *Welcome) render(text string, m member, channelID string) string {
	info, _ := w.guilds.Get(m.GuildID)

	guildName := info.Name
	if guildName == "" {
		guildName = "this server"
	}

	displayName := m.User.GlobalName
	if displayName == "" {
		displayName = m.User.Username
	}
	if displayName == "" {
		displayName = "<@" + m.User.ID + ">"
	}

	channel := info.Channels[channelID]
	if channel == "" {
		channel = channelID
	}

	memberCount := strconv.Itoa(info.MemberCount)

	// Longer tokens first, otherwise {user} would eat the start of {user.name}.
	replacer := strings.NewReplacer(
		"{user.name}", displayName,
		"{user.id}", m.User.ID,
		"{guild.id}", m.GuildID,
		"{member.count}", memberCount,
		"{user}", "<@"+m.User.ID+">",
		"{guild}", guildName,
		"{channel}", channel,
	)
	return replacer.Replace(text)
}
