package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/itsChris/wgpilot/internal/auth"
	"github.com/itsChris/wgpilot/internal/db"
	"github.com/itsChris/wgpilot/internal/logging"
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
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "not implemented")
			return nil
		},
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
