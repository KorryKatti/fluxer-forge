// Command bot is the Fluxer bot runtime: load config.toml, discover the
// instance, prove the token, then stay connected and print incoming events.
//
// Usage:
//
//	FLUXER_TOKEN=<token> bot [-config config.toml] [-dry-run]
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
	"github.com/KorryKatti/fluxer-forge/runtime/internal/fluxer"
)

func main() {
	configPath := flag.String("config", "config.toml", "path to the config file")
	dryRunFlag := flag.Bool("dry-run", false, "force dry-run, whatever the config says")
	flag.Parse()

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
	slog.Info("config ok",
		"instance", cfg.InstanceURL,
		"prefix", cfg.Bot.Prefix,
		"dry_run", cfg.Bot.DryRun || *dryRunFlag,
		"modules", enabledModules(cfg),
	)

	// The token comes from the environment, never from a flag or a file:
	// flags end up in shell history and process lists.
	token := os.Getenv("FLUXER_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "FLUXER_TOKEN is not set")
		fmt.Fprintln(os.Stderr, "example: FLUXER_TOKEN=xyz ./bot -config config.toml")
		os.Exit(1)
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
	rest.DryRun = cfg.Bot.DryRun || *dryRunFlag
	user, err := rest.GetCurrentUser()
	if err != nil {
		fatal("token check failed", err)
	}
	slog.Info("token ok", "user", user.Username, "id", user.ID)

	// 4. Stay connected. Run only returns when ctx is cancelled.
	gateway := fluxer.NewGateway(disco.Endpoints.Gateway, token)
	go gateway.Run(ctx)

	slog.Info("bot is running; press Ctrl+C to stop")
	for {
		select {
		case <-ctx.Done():
			slog.Info("shutting down")
			return
		case ev := <-gateway.Events:
			slog.Info("event", "name", ev.Name)
		}
	}
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
