package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"github.com/ios9000/PGPulse_01/internal/agent"
	"github.com/ios9000/PGPulse_01/internal/cluster/etcd"
	"github.com/ios9000/PGPulse_01/internal/cluster/patroni"
)

// Version is set at build time via ldflags.
var Version = "0.1.0-dev"

func main() {
	configPath := flag.String("config", "configs/pgpulse-agent.yml", "path to agent config file")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	logger.Info("starting pgpulse-agent", "version", Version)

	// Load config via koanf.
	k := koanf.New(".")
	if err := k.Load(file.Provider(*configPath), yaml.Parser()); err != nil {
		logger.Warn("config file not found, using defaults", "path", *configPath, "err", err)
	}

	listenAddr := k.String("agent.listen_addr")
	if listenAddr == "" {
		listenAddr = "0.0.0.0:9187"
	}

	patroniProvider := patroni.NewProvider(patroni.PatroniConfig{
		PatroniURL:     k.String("agent.patroni_url"),
		PatroniConfig:  k.String("agent.patroni_config"),
		PatroniCtlPath: k.String("agent.patroni_ctl_path"),
	})

	etcdProvider := etcd.NewProvider(etcd.ETCDConfig{
		Endpoints: k.Strings("agent.etcd_endpoints"),
		CtlPath:   k.String("agent.etcd_ctl_path"),
	})

	srv := agent.NewServer(agent.ServerConfig{
		ListenAddr:      listenAddr,
		MountPoints:     k.Strings("agent.mount_points"),
		PatroniProvider: patroniProvider,
		ETCDProvider:    etcdProvider,
	}, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "agent error: %v\n", err)
		os.Exit(1)
	}
}
