package dispatch

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// GuildInfo is what the bot knows about one guild: enough to render a
// message template and to tell whether an event belongs to a guild we
// are still in.
type GuildInfo struct {
	ID          string
	Name        string
	MemberCount int
	Channels    map[string]string // channel ID -> name
}

// Guilds caches guild state from READY and the guild lifecycle events.
// It is safe to read from modules while the dispatcher updates it.
type Guilds struct {
	mu   sync.RWMutex
	byID map[string]*GuildInfo
}

func NewGuilds() *Guilds {
	return &Guilds{byID: make(map[string]*GuildInfo)}
}

// Known reports whether the bot is currently in the guild. Events for any
// other guild are dropped before a module sees them.
func (g *Guilds) Known(id string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, ok := g.byID[id]
	return ok
}

// Get returns a snapshot of one guild. The map is shared, but never
// mutated in place: a guild update replaces it under the lock.
func (g *Guilds) Get(id string) (GuildInfo, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	info, ok := g.byID[id]
	if !ok {
		return GuildInfo{}, false
	}
	return *info, true
}

// Apply folds one gateway event into the cache. Unknown events are ignored.
func (g *Guilds) Apply(ev Event) {
	switch ev.Name {
	case "READY":
		g.ready(ev.Data)
	case "GUILD_CREATE":
		g.create(ev.Data)
	case "GUILD_DELETE":
		g.delete(ev.Data)
	case "GUILD_MEMBER_ADD", "GUILD_MEMBER_REMOVE":
		g.memberCount(ev.Data, ev.Name == "GUILD_MEMBER_ADD")
	}
}

// ready records every guild the session is in. On a bot session the entries
// are only {"id": "...", "unavailable": true}; GUILD_CREATE fills them in.
func (g *Guilds) ready(data json.RawMessage) {
	var payload struct {
		Guilds []struct {
			ID string `json:"id"`
		} `json:"guilds"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		slog.Debug("unreadable READY payload", "error", err)
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	for _, entry := range payload.Guilds {
		if entry.ID == "" {
			continue
		}
		if _, ok := g.byID[entry.ID]; !ok {
			g.byID[entry.ID] = &GuildInfo{ID: entry.ID}
		}
	}
}

func (g *Guilds) create(data json.RawMessage) {
	var payload struct {
		ID          string `json:"id"`
		Unavailable bool   `json:"unavailable"`
		Properties  struct {
			Name string `json:"name"`
		} `json:"properties"`
		Channels []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"channels"`
		MemberCount int `json:"member_count"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		slog.Debug("unreadable GUILD_CREATE payload", "error", err)
		return
	}
	if payload.ID == "" {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	info, ok := g.byID[payload.ID]
	if !ok {
		info = &GuildInfo{ID: payload.ID}
		g.byID[payload.ID] = info
	}
	if payload.Unavailable {
		return
	}
	info.Name = payload.Properties.Name
	info.MemberCount = payload.MemberCount
	channels := make(map[string]string, len(payload.Channels))
	for _, ch := range payload.Channels {
		if ch.ID != "" {
			channels[ch.ID] = ch.Name
		}
	}
	info.Channels = channels
}

func (g *Guilds) delete(data json.RawMessage) {
	var payload struct {
		ID          string `json:"id"`
		GuildID     string `json:"guild_id"`
		Unavailable bool   `json:"unavailable"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		slog.Debug("unreadable GUILD_DELETE payload", "error", err)
		return
	}
	if payload.Unavailable {
		return // temporarily unavailable: keep the guild, wait for GUILD_CREATE
	}
	id := payload.ID
	if id == "" {
		id = payload.GuildID
	}
	if id == "" {
		return
	}

	g.mu.Lock()
	delete(g.byID, id)
	g.mu.Unlock()
}

// memberCount keeps {member.count} current between GUILD_CREATE events.
func (g *Guilds) memberCount(data json.RawMessage, add bool) {
	var payload struct {
		GuildID string `json:"guild_id"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.GuildID == "" {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	info, ok := g.byID[payload.GuildID]
	if !ok {
		return
	}
	if add {
		info.MemberCount++
	} else if info.MemberCount > 0 {
		info.MemberCount--
	}
}
