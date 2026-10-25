// Package template fills the fixed message placeholders. There is no
// scripting: an unknown {token} is left exactly as written.
package template

import (
	"strconv"
	"strings"
)

// Context is what a message can talk about.
type Context struct {
	UserID      string
	UserName    string
	DisplayName string

	GuildID   string
	GuildName string

	ChannelID   string
	ChannelName string

	MemberCount int
}

// Render replaces the tokens from the plan:
// {user} {user.name} {user.id} {guild} {guild.id} {channel} {member.count}
func Render(text string, c Context) string {
	displayName := c.DisplayName
	if displayName == "" {
		displayName = c.UserName
	}
	if displayName == "" && c.UserID != "" {
		displayName = "<@" + c.UserID + ">"
	}

	guildName := c.GuildName
	if guildName == "" {
		guildName = "this server"
	}

	channel := c.ChannelName
	if channel == "" {
		channel = c.ChannelID
	}

	user := "<@" + c.UserID + ">"
	if c.UserID == "" {
		user = ""
	}

	// Longer tokens first, otherwise {user} would eat the start of {user.name}.
	replacer := strings.NewReplacer(
		"{user.name}", displayName,
		"{user.id}", c.UserID,
		"{guild.id}", c.GuildID,
		"{member.count}", strconv.Itoa(c.MemberCount),
		"{user}", user,
		"{guild}", guildName,
		"{channel}", channel,
	)
	return replacer.Replace(text)
}
