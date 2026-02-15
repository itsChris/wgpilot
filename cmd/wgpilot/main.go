package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	wgpilot "github.com/itsChris/wgpilot"
	authpkg "github.com/itsChris/wgpilot/internal/auth"
	"github.com/itsChris/wgpilot/internal/config"
	"github.com/itsChris/wgpilot/internal/crypto"
	"github.com/itsChris/wgpilot/internal/db"
	"github.com/itsChris/wgpilot/internal/debug"
	"github.com/itsChris/wgpilot/internal/logging"
	"github.com/itsChris/wgpilot/internal/monitor"
	"github.com/itsChris/wgpilot/internal/sdnotify"
	"github.com/itsChris/wgpilot/internal/server"
	wgtls "github.com/itsChris/wgpilot/internal/tls"
	"github.com/itsChris/wgpilot/internal/updater"
	"github.com/itsChris/wgpilot/internal/wg"
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
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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
		newAPIKeyCmd(),
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

	// ── Set up encryption key for private keys at rest ──────────────
	encKey, err := crypto.DeriveKey(jwtSecret)
	if err != nil {
		return fmt.Errorf("derive encryption key: %w", err)
	}
	database.SetEncryptionKey(encKey)

	// Encrypt any existing unencrypted private keys.
	if err := db.MigrateEncryptKeys(ctx, database, logger); err != nil {
		return fmt.Errorf("encrypt existing keys: %w", err)
	}

	sessionTTL, err := time.ParseDuration(cfg.Auth.SessionTTL)
	if err != nil {
		return fmt.Errorf("parse session ttl %q: %w", cfg.Auth.SessionTTL, err)
	}

	jwtSvc, err := authpkg.NewJWTService(jwtSecret, sessionTTL, logger)
	if err != nil {
		return fmt.Errorf("create jwt service: %w", err)
	}

	secureCookies := !cfg.Server.DevMode
	sessions, err := authpkg.NewSessionManager(secureCookies, logger)
	if err != nil {
		return fmt.Errorf("create session manager: %w", err)
	}

	rateLimiter, err := authpkg.NewLoginRateLimiter(cfg.Auth.RateLimitRPM, time.Minute)
	if err != nil {
		return fmt.Errorf("create rate limiter: %w", err)
	}
	defer rateLimiter.Stop()

	// ── Create WireGuard manager ─────────────────────────────────────
	wgCtrl, err := wg.NewWireGuardController()
	if err != nil {
		logger.Warn("wireguard_controller_init_failed",
			"error", err,
			"component", "main",
		)
	}

	var wgMgr *wg.Manager
	if wgCtrl != nil {
		linkMgr := wg.NewLinkManager()
		wgMgr, err = wg.NewManager(wgCtrl, linkMgr, logger)
		if err != nil {
			logger.Warn("wireguard_manager_init_failed",
				"error", err,
				"component", "main",
			)
		}
	}

	// ── Create HTTP server ───────────────────────────────────────────
	srv, err := server.New(server.Config{
		DB:          database,
		Logger:      logger,
		JWTService:  jwtSvc,
		Sessions:    sessions,
		RateLimiter: rateLimiter,
		WGManager:   wgMgr,
		DevMode:     cfg.Server.DevMode,
		Ring:        ring,
		Version:     version,
	})
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	// ── Embed frontend SPA ─────────────────────────────────────────
	frontendFS, err := fs.Sub(wgpilot.FrontendDist, "frontend/dist")
	if err != nil {
		return fmt.Errorf("embed frontend: %w", err)
	}
	srv.RegisterFrontend(frontendFS)

	// ── Configure TLS ───────────────────────────────────────────────
	dataDir := filepath.Dir(cfg.Database.Path)
	tlsMgr, err := wgtls.NewManager(wgtls.Config{
		Mode:     cfg.TLS.Mode,
		Domain:   cfg.TLS.ACMEDomain,
		Email:    cfg.TLS.ACMEEmail,
		CertFile: cfg.TLS.CertFile,
		KeyFile:  cfg.TLS.KeyFile,
		DataDir:  dataDir,
	}, logger)
	if err != nil {
		return fmt.Errorf("configure tls: %w", err)
	}

	httpServer := &http.Server{
		Addr:           cfg.Server.Listen,
		Handler:        srv,
		TLSConfig:      tlsMgr.TLSConfig(),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 32 << 10,
	}

	// ── Start background monitor ────────────────────────────────────
	monitorCtx, monitorCancel := context.WithCancel(context.Background())
	defer monitorCancel()

	pollInterval := 30 * time.Second
	if pi, err := time.ParseDuration(cfg.Monitor.PollInterval); err == nil {
		pollInterval = pi
	}

	compactInterval := 1 * time.Hour
	if ci, err := time.ParseDuration(cfg.Monitor.CompactionInterval); err == nil {
		compactInterval = ci
	}

	retention := 24 * time.Hour
	if ret, err := time.ParseDuration(cfg.Monitor.SnapshotRetention); err == nil {
		retention = ret
	}

	poller, err := monitor.NewPoller(database, wgMgr, logger, pollInterval)
	if err != nil {
		logger.Warn("monitor_poller_init_failed",
			"error", err,
			"component", "main",
		)
	} else {
		go poller.Run(monitorCtx)
	}

	compactor, err := monitor.NewCompactor(database, logger, compactInterval, retention)
	if err != nil {
		logger.Warn("monitor_compactor_init_failed",
			"error", err,
			"component", "main",
		)
	} else {
		go compactor.Run(monitorCtx)
	}

	// ── Start peer expiry checker ────────────────────────────────────
	expiryChecker, err := monitor.NewExpiryChecker(database, wgMgr, logger, 1*time.Hour)
	if err != nil {
		logger.Warn("expiry_checker_init_failed",
			"error", err,
			"component", "main",
		)
	} else {
		go expiryChecker.Run(monitorCtx)
	}

	// ── Signal handling ──────────────────────────────────────────────
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("https_listening",
			"addr", cfg.Server.Listen,
			"tls_mode", string(tlsMgr.ActiveMode()),
			"component", "main",
		)
		// TLSConfig is already set on the server; pass empty cert/key
		// so ListenAndServeTLS uses the server's TLSConfig.
		if err := httpServer.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("https listen: %w", err)
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

				monitorCancel()

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
			jwtSecret, err := authpkg.GenerateSecret(32)
			if err != nil {
				return fmt.Errorf("generate JWT secret: %w", err)
			}
			if err := database.SetSetting(ctx, "jwt_secret", base64.StdEncoding.EncodeToString(jwtSecret)); err != nil {
				return fmt.Errorf("store JWT secret: %w", err)
			}

			// Reset setup state so the wizard runs again.
			if err := database.DeleteSetting(ctx, "setup_complete"); err != nil {
				return fmt.Errorf("reset setup state: %w", err)
			}

			// Generate and store OTP.
			otp, err := authpkg.GenerateOTP(16)
			if err != nil {
				return fmt.Errorf("generate OTP: %w", err)
			}
			otpHash, err := authpkg.HashPassword(otp)
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
	cmd := &cobra.Command{
		Use:   "diagnose",
		Short: "Run system diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, _ := cmd.Flags().GetBool("json")
			dataDir, _ := cmd.Flags().GetString("data-dir")
			if dataDir == "" {
				dataDir = "/var/lib/wgpilot"
			}

			return debug.Run(debug.Config{
				Version:    version,
				DataDir:    dataDir,
				DBPath:     filepath.Join(dataDir, "wgpilot.db"),
				JSONOutput: jsonOutput,
				Writer:     os.Stdout,
			})
		},
	}
	cmd.Flags().Bool("json", false, "output in JSON format")
	return cmd
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
	cmd := &cobra.Command{
		Use:   "backup [output-path]",
		Short: "Create a backup of the database",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			cfg, err := config.Load(configPath, cmd.Flags())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			dbPath := cfg.Database.Path
			dataDir := filepath.Dir(dbPath)
			if _, err := os.Stat(dbPath); os.IsNotExist(err) {
				return fmt.Errorf("database not found at %s", dbPath)
			}

			outPath := filepath.Join(dataDir, fmt.Sprintf("wgpilot-backup-%s.db", time.Now().Format("20060102-150405")))
			if len(args) > 0 {
				outPath = args[0]
			}

			database, err := db.Open(dbPath, false, nil)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer database.Close()

			if err := database.VacuumInto(context.Background(), outPath); err != nil {
				return fmt.Errorf("backup failed: %w", err)
			}

			fmt.Printf("Backup created: %s\n", outPath)
			return nil
		},
	}
	return cmd
}

func newRestoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore <backup-path>",
		Short: "Restore database from a backup",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			cfg, err := config.Load(configPath, cmd.Flags())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			backupPath := args[0]
			if _, err := os.Stat(backupPath); os.IsNotExist(err) {
				return fmt.Errorf("backup file not found: %s", backupPath)
			}

			dbPath := cfg.Database.Path

			// Validate the backup by opening it.
			backupDB, err := db.Open(backupPath, true, nil)
			if err != nil {
				return fmt.Errorf("invalid backup file: %w", err)
			}
			backupDB.Close()

			// Copy backup over the current database.
			data, err := os.ReadFile(backupPath)
			if err != nil {
				return fmt.Errorf("read backup: %w", err)
			}
			if err := os.WriteFile(dbPath, data, 0640); err != nil {
				return fmt.Errorf("write database: %w", err)
			}

			fmt.Printf("Database restored from %s\n", backupPath)
			fmt.Println("Please restart the wgpilot service.")
			return nil
		},
	}
	return cmd
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
			configPath, _ := cmd.Flags().GetString("config")
			cfg, err := config.Load(configPath, cmd.Flags())
			if err != nil {
				return fmt.Errorf("config error: %w", err)
			}

			dataDir := filepath.Dir(cfg.Database.Path)
			fmt.Println("Configuration validated successfully.")
			fmt.Printf("  Data directory: %s\n", dataDir)
			fmt.Printf("  Listen address: %s\n", cfg.Server.Listen)
			fmt.Printf("  TLS mode:       %s\n", cfg.TLS.Mode)
			fmt.Printf("  Dev mode:       %v\n", cfg.Server.DevMode)

			// Check data directory.
			if _, err := os.Stat(dataDir); os.IsNotExist(err) {
				fmt.Printf("  WARNING: Data directory %s does not exist\n", dataDir)
			}

			// Check database.
			if _, err := os.Stat(cfg.Database.Path); os.IsNotExist(err) {
				fmt.Println("  Database: not found (will be created on first run)")
			} else {
				fmt.Printf("  Database: %s\n", cfg.Database.Path)
			}

			return nil
		},
	})

	return configCmd
}

func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update wgpilot to the latest version",
		RunE: func(cmd *cobra.Command, args []string) error {
			checkOnly, _ := cmd.Flags().GetBool("check")
			logger := logging.New(logging.Config{Level: slog.LevelInfo})

			u, err := updater.NewUpdater(logger)
			if err != nil {
				return fmt.Errorf("create updater: %w", err)
			}

			ctx := context.Background()

			if checkOnly {
				result, err := u.CheckLatest(ctx, version)
				if err != nil {
					return fmt.Errorf("check for updates: %w", err)
				}
				if result.UpdateAvailable {
					fmt.Printf("Update available: %s -> %s\n", result.CurrentVersion, result.LatestVersion)
					fmt.Printf("Release: %s\n", result.ReleaseURL)
				} else {
					fmt.Printf("Already up to date (version %s)\n", result.CurrentVersion)
				}
				return nil
			}

			result, err := u.Update(ctx, version)
			if err != nil {
				return fmt.Errorf("update: %w", err)
			}

			if !result.UpdateAvailable {
				fmt.Printf("Already up to date (version %s)\n", result.CurrentVersion)
				return nil
			}

			fmt.Printf("Updated to version %s\n", result.LatestVersion)
			fmt.Println("Restart the service to apply: systemctl restart wgpilot")
			return nil
		},
	}
	cmd.Flags().Bool("check", false, "only check for updates, don't install")
	return cmd
}

func newAPIKeyCmd() *cobra.Command {
	apiKeyCmd := &cobra.Command{
		Use:   "api-key",
		Short: "Manage API keys",
	}

	apiKeyCmd.AddCommand(newAPIKeyCreateCmd())
	apiKeyCmd.AddCommand(newAPIKeyListCmd())
	apiKeyCmd.AddCommand(newAPIKeyRevokeCmd())

	return apiKeyCmd
}

func newAPIKeyCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new API key",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("name")
			role, _ := cmd.Flags().GetString("role")
			expiresIn, _ := cmd.Flags().GetString("expires-in")

			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if role != "admin" && role != "viewer" {
				return fmt.Errorf("--role must be 'admin' or 'viewer'")
			}

			configPath, _ := cmd.Flags().GetString("config")
			cfg, err := config.Load(configPath, cmd.Flags())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			ctx := context.Background()
			logger := logging.New(logging.Config{Level: slog.LevelWarn})

			database, err := db.New(ctx, cfg.Database.Path, logger, false)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer database.Close()

			if err := db.Migrate(ctx, database, logger); err != nil {
				return fmt.Errorf("run migrations: %w", err)
			}

			// Find or create a system user for CLI-created keys.
			users, err := database.ListUsers(ctx)
			if err != nil {
				return fmt.Errorf("list users: %w", err)
			}
			if len(users) == 0 {
				return fmt.Errorf("no users found — run setup first")
			}
			userID := users[0].ID // Use the first (admin) user.

			var expiresAt *time.Time
			if expiresIn != "" {
				d, err := time.ParseDuration(expiresIn)
				if err != nil {
					return fmt.Errorf("invalid --expires-in duration: %w", err)
				}
				t := time.Now().Add(d)
				expiresAt = &t
			}

			key, hash, prefix, err := authpkg.GenerateAPIKey()
			if err != nil {
				return fmt.Errorf("generate api key: %w", err)
			}

			apiKey := &db.APIKey{
				Name:      name,
				KeyHash:   hash,
				KeyPrefix: prefix,
				UserID:    userID,
				Role:      role,
				ExpiresAt: expiresAt,
			}

			id, err := database.CreateAPIKey(ctx, apiKey)
			if err != nil {
				return fmt.Errorf("create api key: %w", err)
			}

			fmt.Printf("API key created (id=%d):\n", id)
			fmt.Printf("  Name:    %s\n", name)
			fmt.Printf("  Role:    %s\n", role)
			fmt.Printf("  Key:     %s\n", key)
			if expiresAt != nil {
				fmt.Printf("  Expires: %s\n", expiresAt.Format(time.RFC3339))
			} else {
				fmt.Printf("  Expires: never\n")
			}
			fmt.Println("\nSave this key — it cannot be retrieved again.")
			return nil
		},
	}
	cmd.Flags().String("name", "", "name for the API key (required)")
	cmd.Flags().String("role", "admin", "role for the API key (admin or viewer)")
	cmd.Flags().String("expires-in", "", "expiry duration (e.g. 720h for 30 days)")
	return cmd
}

func newAPIKeyListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all API keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			cfg, err := config.Load(configPath, cmd.Flags())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			ctx := context.Background()
			logger := logging.New(logging.Config{Level: slog.LevelWarn})

			database, err := db.New(ctx, cfg.Database.Path, logger, false)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer database.Close()

			if err := db.Migrate(ctx, database, logger); err != nil {
				return fmt.Errorf("run migrations: %w", err)
			}

			keys, err := database.ListAllAPIKeys(ctx)
			if err != nil {
				return fmt.Errorf("list api keys: %w", err)
			}

			if len(keys) == 0 {
				fmt.Println("No API keys found.")
				return nil
			}

			fmt.Printf("%-4s %-20s %-20s %-8s %-20s %-20s\n", "ID", "NAME", "PREFIX", "ROLE", "EXPIRES", "LAST USED")
			for _, k := range keys {
				expires := "never"
				if k.ExpiresAt != nil {
					expires = k.ExpiresAt.Format("2006-01-02 15:04")
				}
				lastUsed := "never"
				if k.LastUsed != nil {
					lastUsed = k.LastUsed.Format("2006-01-02 15:04")
				}
				fmt.Printf("%-4d %-20s %-20s %-8s %-20s %-20s\n", k.ID, k.Name, k.KeyPrefix, k.Role, expires, lastUsed)
			}
			return nil
		},
	}
}

func newAPIKeyRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <id>",
		Short: "Revoke (delete) an API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid key ID: %w", err)
			}

			configPath, _ := cmd.Flags().GetString("config")
			cfg, err := config.Load(configPath, cmd.Flags())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			ctx := context.Background()
			logger := logging.New(logging.Config{Level: slog.LevelWarn})

			database, err := db.New(ctx, cfg.Database.Path, logger, false)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer database.Close()

			if err := db.Migrate(ctx, database, logger); err != nil {
				return fmt.Errorf("run migrations: %w", err)
			}

			if err := database.DeleteAPIKey(ctx, id); err != nil {
				return fmt.Errorf("revoke api key: %w", err)
			}

			fmt.Printf("API key %d revoked.\n", id)
			return nil
		},
	}
}
