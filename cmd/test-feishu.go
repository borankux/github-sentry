package cmd

import (
	"fmt"
	"time"

	"github.com/allintech/github-sentry/config"
	"github.com/allintech/github-sentry/notify"
	"github.com/spf13/cobra"
)

var (
	testCommitID      string
	testCommitMessage string
	testBranch        string
)

var testFeishuCmd = &cobra.Command{
	Use:   "test-feishu",
	Short: "Test Feishu notification without using database",
	Long: `Send a test notification to Feishu using the webhook URL and secret
configured in config.yml. This command does not require database connection.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load config
		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Validate Feishu config
		if cfg.Feishu.WebhookURL == "" {
			return fmt.Errorf("feishu.webhook_url must be set in config.yml")
		}

		// Set defaults if not provided
		if testCommitID == "" {
			testCommitID = "abc1234"
		}
		if testCommitMessage == "" {
			testCommitMessage = "Test commit message"
		}
		if testBranch == "" {
			testBranch = "main"
		}

		fmt.Printf("Sending test notification to Feishu...\n")
		fmt.Printf("  Webhook URL: %s\n", cfg.Feishu.WebhookURL)
		if cfg.Feishu.WebhookSecret != "" {
			fmt.Printf("  Using signature: yes\n")
		} else {
			fmt.Printf("  Using signature: no\n")
		}
		fmt.Printf("  Commit ID: %s\n", testCommitID)
		fmt.Printf("  Commit Message: %s\n", testCommitMessage)
		fmt.Printf("  Branch: %s\n", testBranch)
		fmt.Println()

		// Send notification
		err = notify.NotifyWithSecret(
			cfg.Feishu.WebhookURL,
			cfg.Feishu.WebhookSecret,
			testCommitID,
			testCommitMessage,
			testBranch,
			time.Now(),
		)
		if err != nil {
			return fmt.Errorf("failed to send notification: %w", err)
		}

		fmt.Println("✅ Feishu notification sent successfully!")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(testFeishuCmd)

	// Add flags
	testFeishuCmd.Flags().StringVarP(&testCommitID, "commit-id", "c", "", "Commit ID (default: abc1234)")
	testFeishuCmd.Flags().StringVarP(&testCommitMessage, "message", "m", "", "Commit message (default: 'Test commit message')")
	testFeishuCmd.Flags().StringVarP(&testBranch, "branch", "b", "", "Branch name (default: main)")
}

