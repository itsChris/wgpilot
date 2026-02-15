package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/itsChris/wgpilot/internal/auth"
	"github.com/itsChris/wgpilot/internal/config"
	"github.com/itsChris/wgpilot/internal/db"
	"github.com/itsChris/wgpilot/internal/logging"
	"github.com/itsChris/wgpilot/internal/sdnotify"
	"github.com/itsChris/wgpilot/internal/server"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "wgpilot",
		Short: "WireGuard management tool",
		Long:  "wgpilot is a WireGuard management tool with an embedded web UI.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().String("config", "", "path to config file (default: /etc/wgpilot/config.yaml)")
	root.PersistentFlags().String("data-dir", "", "path to data directory (default: /var/lib/wgpilot)")
	root.PersistentFlags().String("log-level", "info", "log level (debug, info, warn, error)")
	root.PersistentFlags().Bool("dev-mode", false, "enable development mode")

	root.AddCommand(
		newServeCmd(),
		newInitCmd(),
		newDiagnoseCmd(),
		newVersionCmd(),
		newBackupCmd(),
		newRestoreCmd(),
		newConfigCmd(),
		newUpdateCmd(),
	)

	return root
}

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the wgpilot server",
		RunE:  runServe,
	}
}

func runServe(cmd *cobra.Command, args []string) error {
	// ── Load configuration ───────────────────────────────────────────
	configPath, _ := cmd.Flags().GetString("config")
	cfg, err := config.Load(configPath, cmd.Flags())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Apply CLI flag overrides that don't map directly to koanf paths.
	if f := cmd.Flags().Lookup("dev-mode"); f != nil && f.Changed {
		devMode, _ := cmd.Flags().GetBool("dev-mode")
		cfg.Server.DevMode = devMode
	}
	if f := cmd.Flags().Lookup("log-level"); f != nil && f.Changed {
		level, _ := cmd.Flags().GetString("log-level")
		cfg.Logging.Level = level
	}
	if f := cmd.Flags().Lookup("data-dir"); f != nil && f.Changed {
		dataDir, _ := cmd.Flags().GetString("data-dir")
		cfg.Database.Path = filepath.Join(dataDir, "wgpilot.db")
	}

	// Dev mode forces debug logging.
	if cfg.Server.DevMode && cfg.Logging.Level == "info" {
		cfg.Logging.Level = "debug"
	}

	// ── Create logger ────────────────────────────────────────────────
	logLevel := parseLogLevel(cfg.Logging.Level)
	ring := logging.NewRingBuffer(logging.DefaultRingSize)
	logger := logging.NewWithRing(logging.Config{
		Level:   logLevel,
		DevMode: cfg.Server.DevMode,
	}, ring)

	logger.Info("wgpilot_starting",
		"version", version,
		"go_version", runtime.Version(),
		"os", runtime.GOOS,
		"arch", runtime.GOARCH,
		"pid", os.Getpid(),
		"uid", os.Getuid(),
		"gid", os.Getgid(),
		"listen", cfg.Server.Listen,
		"log_level", cfg.Logging.Level,
		"dev_mode", cfg.Server.DevMode,
		"db_path", cfg.Database.Path,
		"component", "main",
	)

	// ── Open database and run migrations ─────────────────────────────
	ctx := context.Background()

	database, err := db.New(ctx, cfg.Database.Path, logger, cfg.Server.DevMode)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	if err := db.Migrate(ctx, database, logger); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	// ── Load JWT secret from database ────────────────────────────────
	jwtSecretB64, err := database.GetSetting(ctx, "jwt_secret")
	if err != nil {
		return fmt.Errorf("read jwt secret: %w", err)
	}
	if jwtSecretB64 == "" {
		return fmt.Errorf("jwt secret not found in database — run 'wgpilot init' first")
	}
	jwtSecret, err := base64.StdEncoding.DecodeString(jwtSecretB64)
	if err != nil {
		return fmt.Errorf("decode jwt secret: %w", err)
	}

	sessionTTL, err := time.ParseDuration(cfg.Auth.SessionTTL)
	if err != nil {
		return fmt.Errorf("parse session ttl %q: %w", cfg.Auth.SessionTTL, err)
	}

	jwtSvc, err := auth.NewJWTService(jwtSecret, sessionTTL, logger)
	if err != nil {
		return fmt.Errorf("create jwt service: %w", err)
	}

	secureCookies := !cfg.Server.DevMode
	sessions, err := auth.NewSessionManager(secureCookies, logger)
	if err != nil {
		return fmt.Errorf("create session manager: %w", err)
	}

	rateLimiter, err := auth.NewLoginRateLimiter(cfg.Auth.RateLimitRPM, time.Minute)
	if err != nil {
		return fmt.Errorf("create rate limiter: %w", err)
	}
	defer rateLimiter.Stop()

	// ── Create HTTP server ───────────────────────────────────────────
	srv, err := server.New(server.Config{
		DB:          database,
		Logger:      logger,
		JWTService:  jwtSvc,
		Sessions:    sessions,
		RateLimiter: rateLimiter,
		DevMode:     cfg.Server.DevMode,
	})
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	httpServer := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      srv,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ── Signal handling ──────────────────────────────────────────────
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http_listening",
			"addr", cfg.Server.Listen,
			"component", "main",
		)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http listen: %w", err)
		}
	}()

	// ── Systemd notify READY ─────────────────────────────────────────
	if err := sdnotify.Ready(); err != nil {
		logger.Warn("sd_notify_ready_failed",
			"error", err,
			"component", "main",
		)
	}

	// ── Watchdog heartbeat ───────────────────────────────────────────
	if interval := sdnotify.WatchdogInterval(); interval > 0 {
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for range ticker.C {
				if err := sdnotify.Watchdog(); err != nil {
					logger.Warn("sd_watchdog_failed",
						"error", err,
						"component", "main",
					)
				}
			}
		}()
		logger.Info("watchdog_enabled",
			"interval", interval.String(),
			"component", "main",
		)
	}

	// ── Main loop: wait for signals or fatal errors ──────────────────
	for {
		select {
		case sig := <-sigCh:
			switch sig {
			case syscall.SIGHUP:
				logger.Info("config_reload_requested", "component", "main")
				if err := sdnotify.Reloading(); err != nil {
					logger.Warn("sd_notify_reloading_failed",
						"error", err,
						"component", "main",
					)
				}

				newCfg, err := config.Load(configPath, cmd.Flags())
				if err != nil {
					logger.Error("config_reload_failed",
						"error", err,
						"component", "main",
					)
					// Signal ready again even after failed reload so
					// systemd doesn't think we're stuck reloading.
					sdnotify.Ready()
					continue
				}
				logger.Info("config_reloaded",
					"listen", newCfg.Server.Listen,
					"log_level", newCfg.Logging.Level,
					"component", "main",
				)
				sdnotify.Ready()

			case syscall.SIGTERM, syscall.SIGINT:
				logger.Info("shutdown_requested",
					"signal", sig.String(),
					"component", "main",
				)
				if err := sdnotify.Stopping(); err != nil {
					logger.Warn("sd_notify_stopping_failed",
						"error", err,
						"component", "main",
					)
				}

				shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				if err := httpServer.Shutdown(shutdownCtx); err != nil {
					logger.Error("shutdown_error",
						"error", err,
						"component", "main",
					)
					return fmt.Errorf("shutdown: %w", err)
				}

				logger.Info("shutdown_complete", "component", "main")
				return nil
			}

		case err := <-errCh:
			return err
		}
	}
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize wgpilot configuration and database",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			devMode, _ := cmd.Flags().GetBool("dev-mode")
			logLevel, _ := cmd.Flags().GetString("log-level")

			level := slog.LevelInfo
			if logLevel == "debug" {
				level = slog.LevelDebug
			}
			logger := logging.New(logging.Config{Level: level, DevMode: devMode})

			dataDir, _ := cmd.Flags().GetString("data-dir")
			if dataDir == "" {
				dataDir = "/var/lib/wgpilot"
			}

			if err := os.MkdirAll(dataDir, 0750); err != nil {
				return fmt.Errorf("create data directory: %w", err)
			}

			dsn := filepath.Join(dataDir, "wgpilot.db")
			database, err := db.New(ctx, dsn, logger, devMode)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer database.Close()

			if err := db.Migrate(ctx, database, logger); err != nil {
				return fmt.Errorf("run migrations: %w", err)
			}

			// Generate and store JWT secret.
			jwtSecret, err := auth.GenerateSecret(32)
			if err != nil {
				return fmt.Errorf("generate JWT secret: %w", err)
			}
			if err := database.SetSetting(ctx, "jwt_secret", base64.StdEncoding.EncodeToString(jwtSecret)); err != nil {
				return fmt.Errorf("store JWT secret: %w", err)
			}

			// Generate and store OTP.
			otp, err := auth.GenerateOTP(16)
			if err != nil {
				return fmt.Errorf("generate OTP: %w", err)
			}
			otpHash, err := auth.HashPassword(otp)
			if err != nil {
				return fmt.Errorf("hash OTP: %w", err)
			}
			if err := database.SetSetting(ctx, "setup_otp", otpHash); err != nil {
				return fmt.Errorf("store OTP: %w", err)
			}

			fmt.Printf("Database initialized at %s\n", dsn)
			fmt.Printf("One-time setup password: %s\n", otp)
			fmt.Println("Use this password to complete setup via the web UI.")

			return nil
		},
	}
}

func newDiagnoseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diagnose",
		Short: "Run system diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "not implemented")
			return nil
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("wgpilot %s\n", version)
			fmt.Printf("  commit: %s\n", commit)
			fmt.Printf("  built:  %s\n", date)
		},
	}
}

func newBackupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "backup",
		Short: "Create a backup of the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "not implemented")
			return nil
		},
	}
}

func newRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore",
		Short: "Restore database from a backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "not implemented")
			return nil
		},
	}
}

func newConfigCmd() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Configuration management",
	}

	configCmd.AddCommand(&cobra.Command{
		Use:   "check",
		Short: "Validate configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "not implemented")
			return nil
		},
	})

	return configCmd
}

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update wgpilot to the latest version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "not implemented")
			return nil
		},
	}
}
