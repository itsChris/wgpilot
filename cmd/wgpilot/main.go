package main

import (
	"fmt"
	"os"

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
			fmt.Fprintln(os.Stderr, "not implemented")
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
