// Package doctor runs the preflight checks behind --doctor: instance
// discovery, token validity, gateway connection, and access to every
// configured channel.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/config"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/fluxer"
)

// Run prints one line per check and returns an error if any check failed.
func Run(ctx context.Context, cfg *config.Config, token string, out io.Writer) error {
	failed := false
	report := func(check, status, detail string) {
		fmt.Fprintf(out, "%-9s %-4s %s\n", check, status, detail)
	}

	disco, err := fluxer.Discover(ctx, cfg.InstanceURL)
	if err != nil {
		report("instance", "FAIL", err.Error())
		return errors.New("doctor found problems")
	}
	report("instance", "ok", cfg.InstanceURL)

	rest := fluxer.NewClient(disco.Endpoints.APIPublic, token)
	user, err := rest.GetCurrentUser()
	if err != nil {
		report("token", "FAIL", err.Error())
		return errors.New("doctor found problems")
	}
	report("token", "ok", user.Username+" ("+user.ID+")")

	if err := checkGateway(ctx, disco.Endpoints.Gateway, token); err != nil {
		report("gateway", "FAIL", err.Error())
		failed = true
	} else {
		report("gateway", "ok", "connected and ready")
	}

	for _, id := range channelIDs(cfg) {
		channel, err := rest.GetChannel(id)
		if err != nil {
			report("channel", "FAIL", id+": "+err.Error())
			failed = true
			continue
		}
		name := channel.Name
		if name == "" {
			name = channel.ID
		}
		report("channel", "ok", id+" (#"+name+")")
	}

	if failed {
		return errors.New("doctor found problems")
	}
	return nil
}

// checkGateway connects, identifies and waits for READY.
func checkGateway(ctx context.Context, url, token string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	gateway := fluxer.NewGateway(url, token)
	go gateway.Run(ctx)

	for {
		select {
		case <-ctx.Done():
			return errors.New("no READY from the gateway within 15s")
		case err := <-gateway.Fatal:
			return err
		case ev := <-gateway.Events:
			if ev.Name == "READY" {
				return nil
			}
		}
	}
}

// channelIDs lists every channel the config expects the bot to use, once.
func channelIDs(cfg *config.Config) []string {
	var ids []string
	seen := map[string]bool{}
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}

	if cfg.Modules.Welcome.Enabled {
		add(cfg.Modules.Welcome.OnJoin.ChannelID)
		if cfg.Modules.Welcome.OnLeave.Enabled {
			add(cfg.Modules.Welcome.OnLeave.ChannelID)
		}
	}
	for _, rule := range cfg.Modules.ReactionRoles.Rules {
		add(rule.ChannelID)
	}
	return ids
}
