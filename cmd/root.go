package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "github-sentry",
	Short: "GitHub Sentry - Webhook handler and notification system",
	Long: `GitHub Sentry is a webhook handler that processes GitHub push events,
executes scripts, and sends Feishu notifications.

Run without arguments to start the webhook server, or use subcommands for CLI operations.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior: run server
		runServer()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Add global flags here if needed
}

