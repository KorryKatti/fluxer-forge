// Command bot is the Fluxer bot runtime: load config.toml, discover the
// instance, prove the token, then stay connected and run the enabled modules.
//
// Usage:
//
//	bot [-config config.toml] [-dry-run]
//
// The token comes from FLUXER_TOKEN, .env, a local token file, or a prompt.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/config"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/dispatch"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/fluxer"
	botlog "github.com/KorryKatti/fluxer-forge/runtime/internal/log"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/modules/autorole"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/modules/customcommands"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/modules/reactionroles"
	"github.com/KorryKatti/fluxer-forge/runtime/internal/modules/welcome"
	bottoken "github.com/KorryKatti/fluxer-forge/runtime/internal/token"
)

func main() {
	configPath := flag.String("config", "config.toml", "path to the config file")
	dryRunFlag := flag.Bool("dry-run", false, "force dry-run, whatever the config says")
	flag.Parse()

	closeLogs, err := botlog.Setup("logs")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: file logging is off: %v\n", err)
	} else {
		defer closeLogs()
	}

	// 1. Load and validate the config. Every problem is printed at once.
	data, err := os.ReadFile(*configPath)
	if err != nil {
		fatal("cannot read config", err)
	}
	cfg, err := config.Parse(*configPath, data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	dryRun := cfg.Bot.DryRun || *dryRunFlag
	slog.Info("config ok",
		"config", *configPath,
		"instance", cfg.InstanceURL,
		"prefix", cfg.Bot.Prefix,
		"dry_run", dryRun,
		"modules", enabledModules(cfg),
	)

	// The token is never taken from a flag: flags end up in shell history
	// and process lists.
	token, err := findToken()
	if err != nil {
		fatal("no bot token", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 2. Ask the instance where its API and gateway live.
	disco, err := fluxer.Discover(ctx, cfg.InstanceURL)
	if err != nil {
		fatal("discovery failed", err)
	}
	slog.Info("instance discovered",
		"api", disco.Endpoints.APIPublic,
		"gateway", disco.Endpoints.Gateway,
	)

	// 3. Prove the token with one harmless API call.
	rest := fluxer.NewClient(disco.Endpoints.APIPublic, token)
	rest.DryRun = dryRun
	user, err := rest.GetCurrentUser()
	if err != nil {
		fatal("token check failed", err)
	}
	slog.Info("token ok", "user", user.Username, "id", user.ID)

	// 4. Build the modules this config turns on. Disabled modules are never
	// created, so they cost nothing per event.
	guilds := dispatch.NewGuilds()
	var modules []dispatch.Module
	if cfg.Modules.Welcome.Enabled {
		modules = append(modules, welcome.New(cfg.Modules.Welcome, rest, guilds, dryRun))
	}
	if cfg.Modules.Autorole.Enabled {
		modules = append(modules, autorole.New(cfg.Modules.Autorole, rest, dryRun))
	}
	if cfg.Modules.ReactionRoles.Enabled {
		modules = append(modules, reactionroles.New(cfg.Modules.ReactionRoles, rest, dryRun))
	}
	if cfg.Modules.CustomCommands.Enabled {
		modules = append(modules, customcommands.New(cfg.Modules.CustomCommands, cfg.Bot.Prefix, rest, guilds, dryRun))
	}
	dispatcher := dispatch.New(user.ID, guilds, modules...)

	// 5. Stay connected. Run only returns when ctx is cancelled.
	gateway := fluxer.NewGateway(disco.Endpoints.Gateway, token)
	go gateway.Run(ctx)

	slog.Info("bot is running; press Ctrl+C to stop")
	for {
		select {
		case <-ctx.Done():
			slog.Info("shutting down")
			return
		case err := <-gateway.Fatal:
			slog.Error("gateway stopped", "error", err)
			return
		case ev := <-gateway.Events:
			dispatcher.Dispatch(ctx, ev)
		}
	}
}

// findToken tries every source in order and stops at the first one that has
// something. The prompt writes ./token with 0600 so it is asked only once.
func findToken() (string, error) {
	if t := bottoken.FromEnv(); t != "" {
		return t, nil
	}
	if t, err := bottoken.FromDotEnv(".env"); err != nil {
		return "", fmt.Errorf("read .env: %w", err)
	} else if t != "" {
		return t, nil
	}
	if t, err := bottoken.FromFile("token"); err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	} else if t != "" {
		return t, nil
	}
	return bottoken.Prompt("token")
}

// enabledModules returns e.g. "welcome, autorole" for the startup log.
func enabledModules(cfg *config.Config) string {
	var names []string
	if cfg.JoinActive() || cfg.LeaveActive() {
		names = append(names, "welcome")
	}
	if cfg.Modules.Autorole.Enabled {
		names = append(names, "autorole")
	}
	if cfg.Modules.ReactionRoles.Enabled {
		names = append(names, "reaction_roles")
	}
	if cfg.Modules.CustomCommands.Enabled {
		names = append(names, "custom_commands")
	}
	if len(names) == 0 {
		return "none"
	}
	out := names[0]
	for _, n := range names[1:] {
		out += ", " + n
	}
	return out
}

func fatal(what string, err error) {
	slog.Error(what, "error", err)
	os.Exit(1)
}
