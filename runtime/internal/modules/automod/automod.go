// Package automod deletes messages that contain invites, banned words or
// too many mentions.
package automod

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/config"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/dispatch"
)

// Client is the small part of the REST client this module needs.
type Client interface {
	DeleteMessage(channelID, messageID string) error
}

type Automod struct {
	cfg    config.AutomodLiteModule
	client Client
	dryRun bool
}

func New(cfg config.AutomodLiteModule, client Client, dryRun bool) *Automod {
	return &Automod{cfg: cfg, client: client, dryRun: dryRun}
}

func (a *Automod) Name() string { return "automod_lite" }

func (a *Automod) Events() []string {
	return []string{"MESSAGE_CREATE"}
}

type message struct {
	ID              string            `json:"id"`
	GuildID         string            `json:"guild_id"`
	ChannelID       string            `json:"channel_id"`
	Content         string            `json:"content"`
	Mentions        []json.RawMessage `json:"mentions"`
	MentionRoles    []string          `json:"mention_roles"`
	MentionEveryone bool              `json:"mention_everyone"`
}

func (a *Automod) Handle(ctx context.Context, ev dispatch.Event) error {
	var msg message
	if err := json.Unmarshal(ev.Data, &msg); err != nil {
		return fmt.Errorf("read message: %w", err)
	}
	if msg.GuildID == "" || msg.ID == "" || msg.ChannelID == "" {
		return nil
	}

	reason := a.check(msg)
	if reason == "" {
		return nil
	}

	if err := a.client.DeleteMessage(msg.ChannelID, msg.ID); err != nil {
		return fmt.Errorf("delete message %s: %w", msg.ID, err)
	}
	if a.dryRun {
		slog.Info("dry-run: message not deleted", "channel", msg.ChannelID, "message", msg.ID, "reason", reason)
	} else {
		slog.Info("message deleted", "channel", msg.ChannelID, "message", msg.ID, "reason", reason)
	}
	return nil
}

// check returns the first rule the message breaks, or "" when it is fine.
func (a *Automod) check(msg message) string {
	lower := strings.ToLower(msg.Content)

	if a.cfg.Settings.BlockInvites && hasInvite(lower) {
		return "invite"
	}

	for _, word := range a.cfg.BannedWords {
		word = strings.ToLower(strings.TrimSpace(word))
		if word != "" && strings.Contains(lower, word) {
			return "banned word"
		}
	}

	if a.cfg.Settings.MaxMentions > 0 {
		count := len(msg.Mentions) + len(msg.MentionRoles)
		if msg.MentionEveryone {
			count++
		}
		if count > a.cfg.Settings.MaxMentions {
			return "too many mentions"
		}
	}

	return ""
}

// hasInvite looks for the three shapes a chat invite usually takes. The
// generic /invite/ pattern covers Fluxer and self-hosted instances.
var invitePatterns = []*regexp.Regexp{
	regexp.MustCompile(`discord\.gg/[a-z0-9]+`),
	regexp.MustCompile(`discord(app)?\.com/invite/[a-z0-9]+`),
	regexp.MustCompile(`/invite/[a-z0-9_-]+`),
}

func hasInvite(content string) bool {
	for _, pattern := range invitePatterns {
		if pattern.MatchString(content) {
			return true
		}
	}
	return false
}
