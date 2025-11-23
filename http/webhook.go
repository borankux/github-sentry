package http

import (
	"net/http"
	"strings"
	"time"

	"github.com/allintech/github-sentry/config"
	"github.com/allintech/github-sentry/database"
	"github.com/allintech/github-sentry/executor"
	"github.com/allintech/github-sentry/logger"
	"github.com/allintech/github-sentry/notify"
	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v62/github"
)

func WebHook(c *gin.Context) {
	// Get config from gin context
	cfgInterface, exists := c.Get("config")
	if !exists {
		logger.LogError("config not found in context")
		c.String(http.StatusInternalServerError, "internal error")
		return
	}

	cfg, ok := cfgInterface.(*config.Config)
	if !ok {
		logger.LogError("invalid config type in context")
		c.String(http.StatusInternalServerError, "internal error")
		return
	}

	// Validate payload
	payload, err := github.ValidatePayload(c.Request, []byte(cfg.GitHubWebhookSecret))
	if err != nil {
		logger.LogError("invalid payload: %v", err)
		c.String(http.StatusBadRequest, "invalid payload")
		return
	}

	// Parse webhook event
	event, err := github.ParseWebHook(github.WebHookType(c.Request), payload)
	if err != nil {
		logger.LogError("failed to parse webhook: %v", err)
		c.String(http.StatusBadRequest, "invalid event")
		return
	}

	// Handle push events only
	pushEvent, ok := event.(*github.PushEvent)
	if !ok {
		logger.LogInfo("ignoring non-push event: %s", github.WebHookType(c.Request))
		c.String(http.StatusOK, "event ignored")
		return
	}

	// Check if this is a push to the staging branch
	branch := strings.TrimPrefix(pushEvent.GetRef(), "refs/heads/")
	if branch != cfg.StagingBranch {
		logger.LogInfo("ignoring push to branch: %s (expected: %s)", branch, cfg.StagingBranch)
		c.String(http.StatusOK, "branch ignored")
		return
	}

	// Extract commit information from the head commit
	headCommit := pushEvent.GetHeadCommit()
	if headCommit == nil {
		logger.LogInfo("push event has no head commit")
		c.String(http.StatusOK, "no head commit")
		return
	}

	commitID := headCommit.GetID()
	commitMessage := headCommit.GetMessage()
	commitTime := headCommit.GetTimestamp().Time

	logger.LogTrigger(commitID, commitMessage, branch)

	// Extract repo name and author
	repoName := pushEvent.GetRepo().GetFullName()
	if repoName == "" {
		repoName = "unknown/repo"
	}
	
	// Get commit author (prefer committer, fallback to pusher)
	author := headCommit.GetAuthor().GetName()
	if author == "" {
		author = headCommit.GetAuthor().GetLogin()
	}
	if author == "" {
		author = pushEvent.GetPusher().GetName()
	}
	if author == "" {
		author = pushEvent.GetPusher().GetLogin()
	}
	if author == "" {
		author = "unknown"
	}
	
	// Send "started" card notification immediately
	if notifyErr := notify.NotifyWithSecret(cfg.Feishu.WebhookURL, cfg.Feishu.WebhookSecret, notify.StatusStarted, repoName, author, commitID, commitMessage, branch, commitTime); notifyErr != nil {
		logger.LogError("failed to send Feishu started notification: %v", notifyErr)
		// Continue processing even if notification fails
	}

	// Record trigger in database
	triggerID, err := database.RecordTrigger(commitTime, commitID, commitMessage, branch)
	if err != nil {
		logger.LogError("failed to record trigger: %v", err)
		c.String(http.StatusInternalServerError, "failed to record trigger")
		return
	}

	// Respond to GitHub immediately with success
	// Script execution will happen asynchronously in the background
	c.String(http.StatusOK, "webhook received")

	// Launch async processing in background goroutine
	go processWebhookAsync(cfg, triggerID, commitID, commitMessage, branch, repoName, author, commitTime)
}

// processWebhookAsync handles script execution, result recording, and notifications asynchronously
// This function runs in a background goroutine and does not affect the HTTP response
func processWebhookAsync(cfg *config.Config, triggerID int64, commitID, commitMessage, branch, repoName, author string, commitTime time.Time) {
	// Look up commands for this specific project
	projectCommands, exists := cfg.Commands[repoName]
	// Backward compatibility: check for empty string key (old flat format)
	if !exists {
		projectCommands, exists = cfg.Commands[""]
	}
	if !exists {
		logger.LogInfo("no commands configured for project %s, skipping execution", repoName)
		// Send Feishu notification about skipped execution
		if notifyErr := notify.NotifyWithSecret(cfg.Feishu.WebhookURL, cfg.Feishu.WebhookSecret, notify.StatusSuccess, repoName, author, commitID, commitMessage+" (skipped - no commands configured)", branch, commitTime); notifyErr != nil {
			logger.LogError("failed to send Feishu notification: %v", notifyErr)
		}
		return
	}

	// Execute commands from config
	var results []executor.ExecutionResult
	var err error
	if len(projectCommands.Sequential) > 0 || len(projectCommands.Async) > 0 {
		// Use new command-based execution
		results, err = executor.ExecuteCommands(projectCommands.Sequential, projectCommands.Async, branch, repoName)
	} else {
		// Fallback to old scripts folder method (deprecated)
		results, err = executor.ExecuteScripts(cfg.ScriptsFolder)
	}

	if err != nil {
		logger.LogError("script execution failed: %v", err)
		// Record failed executions
		for _, result := range results {
			status := "success"
			if !result.Success {
				status = "failed"
			}
			if dbErr := database.RecordExecution(triggerID, result.ScriptName, status, result.Output, result.Error); dbErr != nil {
				logger.LogError("failed to record execution: %v", dbErr)
			}
			logger.LogExecution(result.ScriptName, result.Success, result.Output, result.Error)
		}

		// Build failure message including reason from first failed result (if any)
		failureMessage := commitMessage + " (FAILED)"
		for _, result := range results {
			if !result.Success {
				const maxOutputLen = 2000
				output := result.Output
				if len(output) > maxOutputLen {
					output = output[:maxOutputLen] + "...(truncated)"
				}
				failureMessage = failureMessage + "\n\nFailure Reason:\n" +
					"Script: " + result.ScriptName + "\n" +
					"Error: " + result.Error + "\n" +
					"Output:\n" + output
				break
			}
		}

		// Send Feishu notification about failure (with reason)
		if notifyErr := notify.NotifyWithSecret(cfg.Feishu.WebhookURL, cfg.Feishu.WebhookSecret, notify.StatusFailure, repoName, author, commitID, failureMessage, branch, commitTime); notifyErr != nil {
			logger.LogError("failed to send Feishu notification: %v", notifyErr)
		}
		return
	}

	// Record successful executions
	for _, result := range results {
		status := "success"
		if !result.Success {
			status = "failed"
		}
		if dbErr := database.RecordExecution(triggerID, result.ScriptName, status, result.Output, result.Error); dbErr != nil {
			logger.LogError("failed to record execution: %v", dbErr)
		}
		logger.LogExecution(result.ScriptName, result.Success, result.Output, result.Error)
	}

	// Send Feishu notification for success
	if err := notify.NotifyWithSecret(cfg.Feishu.WebhookURL, cfg.Feishu.WebhookSecret, notify.StatusSuccess, repoName, author, commitID, commitMessage, branch, commitTime); err != nil {
		logger.LogError("failed to send Feishu notification: %v", err)
	}

	logger.LogInfo("webhook processed successfully for commit %s", commitID)
}
