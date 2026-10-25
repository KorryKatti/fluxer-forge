// Package dispatch routes gateway events to the modules that want them.
//
// Every guard the plan asks for lives here, once, so no module has to
// remember it: unknown guilds, the bot's own actions, bot and webhook
// messages, and panic recovery.
package dispatch

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/fluxer"
)

// Event is one gateway event. Modules decode Data into the payload they need.
type Event struct {
	Name string
	Data json.RawMessage
}

// Module is one feature. A module only ever sees the events it asked for.
type Module interface {
	Name() string
	Events() []string
	Handle(ctx context.Context, ev Event) error
}

// Dispatcher sends events to modules and enforces the shared guards.
type Dispatcher struct {
	botID   string
	guilds  *Guilds
	byEvent map[string][]Module
}

// New builds a dispatcher for the bot with the given user ID. Modules that
// are disabled are simply not passed in; they cost nothing per event.
func New(botID string, guilds *Guilds, modules ...Module) *Dispatcher {
	d := &Dispatcher{
		botID:   botID,
		guilds:  guilds,
		byEvent: make(map[string][]Module),
	}
	for _, m := range modules {
		for _, name := range m.Events() {
			d.byEvent[name] = append(d.byEvent[name], m)
		}
	}
	return d
}

// Dispatch hands one event to every module that subscribed to it.
//
// It updates the guild cache first (READY and guild lifecycle events), then
// applies the loop guards. A module panic is logged and does not kill the
// session or the other modules.
func (d *Dispatcher) Dispatch(ctx context.Context, ev fluxer.Event) {
	e := Event{Name: ev.Name, Data: ev.Data}
	d.guilds.Apply(e)

	if !d.allow(e) {
		return
	}

	for _, m := range d.byEvent[e.Name] {
		d.handle(ctx, m, e)
	}
}

func (d *Dispatcher) handle(ctx context.Context, m Module, ev Event) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("module panicked", "module", m.Name(), "event", ev.Name, "panic", recovered)
		}
	}()

	if err := m.Handle(ctx, ev); err != nil {
		slog.Error("module failed", "module", m.Name(), "event", ev.Name, "error", err)
	}
}

// envelope holds the few fields every guard needs. Everything else stays
// with the module.
type envelope struct {
	GuildID   string `json:"guild_id"`
	WebhookID string `json:"webhook_id"`
	Author    *struct {
		ID  string `json:"id"`
		Bot bool   `json:"bot"`
	} `json:"author"`
	User *struct {
		ID  string `json:"id"`
		Bot bool   `json:"bot"`
	} `json:"user"`
	UserID string `json:"user_id"`
}

// allow applies the guards from the plan. It returns false for events no
// module should ever see.
func (d *Dispatcher) allow(ev Event) bool {
	var env envelope
	if len(ev.Data) > 0 {
		if err := json.Unmarshal(ev.Data, &env); err != nil {
			slog.Debug("event payload unreadable", "event", ev.Name, "error", err)
			return false
		}
	}

	// Guild events from a guild we are not in (or have just left).
	if env.GuildID != "" && !d.guilds.Known(env.GuildID) {
		slog.Debug("event from unknown guild", "event", ev.Name, "guild", env.GuildID)
		return false
	}

	switch ev.Name {
	case "MESSAGE_CREATE":
		if env.Author == nil {
			return false
		}
		if env.Author.Bot || env.WebhookID != "" || env.Author.ID == d.botID {
			slog.Debug("ignoring message from bot or webhook", "author", env.Author.ID)
			return false
		}
	case "GUILD_MEMBER_ADD", "GUILD_MEMBER_UPDATE", "GUILD_MEMBER_REMOVE":
		if env.User != nil && env.User.ID == d.botID {
			return false
		}
	case "MESSAGE_REACTION_ADD", "MESSAGE_REACTION_REMOVE":
		if env.UserID == d.botID {
			return false
		}
	}
	return true
}
