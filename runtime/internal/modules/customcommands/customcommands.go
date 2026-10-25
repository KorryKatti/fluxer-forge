// Package customcommands replies to prefix commands with configured text.
package customcommands

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/config"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/dispatch"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/template"
)

// Client is the small part of the REST client this module needs.
type Client interface {
	CreateMessage(channelID, content string) error
}

type CustomCommands struct {
	cfg    config.CustomCommandsModule
	prefix string
	client Client
	guilds *dispatch.Guilds
	dryRun bool

	mu       sync.Mutex
	lastUsed map[string]time.Time
}

func New(cfg config.CustomCommandsModule, prefix string, client Client, guilds *dispatch.Guilds, dryRun bool) *CustomCommands {
	return &CustomCommands{
		cfg:      cfg,
		prefix:   prefix,
		client:   client,
		guilds:   guilds,
		dryRun:   dryRun,
		lastUsed: make(map[string]time.Time),
	}
}

func (c *CustomCommands) Name() string { return "custom_commands" }

func (c *CustomCommands) Events() []string {
	return []string{"MESSAGE_CREATE"}
}

type message struct {
	GuildID   string `json:"guild_id"`
	ChannelID string `json:"channel_id"`
	Content   string `json:"content"`
	Author    struct {
		ID         string `json:"id"`
		Username   string `json:"username"`
		GlobalName string `json:"global_name"`
	} `json:"author"`
}

func (c *CustomCommands) Handle(ctx context.Context, ev dispatch.Event) error {
	var msg message
	if err := json.Unmarshal(ev.Data, &msg); err != nil {
		return fmt.Errorf("read message: %w", err)
	}
	if msg.Author.ID == "" || msg.ChannelID == "" {
		return nil
	}
	if c.prefix == "" || !strings.HasPrefix(msg.Content, c.prefix) {
		return nil
	}

	rest := strings.TrimSpace(strings.TrimPrefix(msg.Content, c.prefix))
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return nil
	}

	item, ok := c.find(fields[0])
	if !ok {
		return nil
	}
	if !c.allowed(item, msg.Author.ID) {
		slog.Debug("command on cooldown", "command", item.Name, "user", msg.Author.ID)
		return nil
	}

	text := c.render(item.Reply, msg)
	if err := c.client.CreateMessage(msg.ChannelID, text); err != nil {
		return fmt.Errorf("reply to command %q: %w", item.Name, err)
	}
	if c.dryRun {
		slog.Info("dry-run: command reply not sent", "command", item.Name, "user", msg.Author.ID)
	} else {
		slog.Info("command reply sent", "command", item.Name, "user", msg.Author.ID)
	}
	return nil
}

// find matches the first word after the prefix against a name or alias,
// ignoring case.
func (c *CustomCommands) find(word string) (config.CustomCommand, bool) {
	word = strings.ToLower(word)
	for _, item := range c.cfg.Items {
		if strings.ToLower(item.Name) == word {
			return item, true
		}
		for _, alias := range item.Aliases {
			if strings.ToLower(alias) == word {
				return item, true
			}
		}
	}
	return config.CustomCommand{}, false
}

// allowed applies the per-command, per-user cooldown.
func (c *CustomCommands) allowed(item config.CustomCommand, userID string) bool {
	if item.Cooldown <= 0 {
		return true
	}

	key := item.Name + ":" + userID
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	if last, ok := c.lastUsed[key]; ok {
		if now.Sub(last) < time.Duration(item.Cooldown)*time.Second {
			return false
		}
	}
	c.lastUsed[key] = now
	return true
}

func (c *CustomCommands) render(text string, msg message) string {
	info, _ := c.guilds.Get(msg.GuildID)
	return template.Render(text, template.Context{
		UserID:      msg.Author.ID,
		UserName:    msg.Author.Username,
		DisplayName: msg.Author.GlobalName,
		GuildID:     msg.GuildID,
		GuildName:   info.Name,
		ChannelID:   msg.ChannelID,
		ChannelName: info.Channels[msg.ChannelID],
		MemberCount: info.MemberCount,
	})
}
