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

	// Extract repo information
	repo := pushEvent.GetRepo()
	orgName := ""
	repoName := ""
	if repo != nil {
		if owner := repo.GetOwner(); owner != nil {
			orgName = owner.GetLogin()
		}
		repoName = repo.GetName()
	}
	
	// Build full repo name for display/logging purposes
	fullRepoName := orgName + "/" + repoName
	if fullRepoName == "/" {
		fullRepoName = "unknown/repo"
		orgName = "unknown"
		repoName = "repo"
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
	if notifyErr := notify.NotifyWithSecret(cfg.Feishu.WebhookURL, cfg.Feishu.WebhookSecret, notify.StatusStarted, fullRepoName, author, commitID, commitMessage, branch, commitTime); notifyErr != nil {
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
	go processWebhookAsync(cfg, triggerID, commitID, commitMessage, branch, fullRepoName, orgName, repoName, author, commitTime)
}

// processWebhookAsync handles script execution, result recording, and notifications asynchronously
// This function runs in a background goroutine and does not affect the HTTP response
func processWebhookAsync(cfg *config.Config, triggerID int64, commitID, commitMessage, branch, fullRepoName, orgName, repoName, author string, commitTime time.Time) {
	// Look up commands for this specific project by matching organization and repo
	var projectCommands config.CommandsConfig
	var projectName string
	found := false
	
	if cfg.Commands != nil {
		for name, commands := range cfg.Commands {
			if commands.Organization == orgName && commands.Repo == repoName {
				projectCommands = commands
				projectName = name
				found = true
				break
			}
		}
	}
	
	if !found {
		logger.LogInfo("no commands configured for project %s (org: %s, repo: %s), skipping execution", fullRepoName, orgName, repoName)
		// Send Feishu notification about skipped execution
		if notifyErr := notify.NotifyWithSecret(cfg.Feishu.WebhookURL, cfg.Feishu.WebhookSecret, notify.StatusSuccess, fullRepoName, author, commitID, commitMessage+" (skipped - no commands configured)", branch, commitTime); notifyErr != nil {
			logger.LogError("failed to send Feishu notification: %v", notifyErr)
		}
		return
	}
	
	logger.LogInfo("matched project %s for org=%s, repo=%s", projectName, orgName, repoName)

	// Execute commands from config
	logger.LogInfo("Starting command execution for commit %s", commitID)
	executionStartTime := time.Now()
	
	var results []executor.ExecutionResult
	var err error
	if len(projectCommands.Sequential) > 0 || len(projectCommands.Async) > 0 {
		// Use new command-based execution
		results, err = executor.ExecuteCommands(projectCommands.Sequential, projectCommands.Async, branch, fullRepoName)
	} else {
		// Fallback to old scripts folder method (deprecated)
		results, err = executor.ExecuteScripts(cfg.ScriptsFolder)
	}
	
	// Calculate execution completion time and duration
	var executionEndTime time.Time
	var totalDuration time.Duration
	if len(results) > 0 {
		// Find the latest end time from all results (for async commands, this is the last one to finish)
		executionEndTime = results[0].EndTime
		for _, result := range results[1:] {
			if result.EndTime.After(executionEndTime) {
				executionEndTime = result.EndTime
			}
		}
		totalDuration = executionEndTime.Sub(executionStartTime)
	} else {
		executionEndTime = time.Now()
		totalDuration = executionEndTime.Sub(executionStartTime)
	}
	
	// Verify all results have completion times
	allCompleted := true
	for _, result := range results {
		if result.EndTime.IsZero() {
			logger.LogError("Execution result for %s has no end time", result.ScriptName)
			allCompleted = false
		}
	}
	
	// Log execution completion with timing
	logger.LogInfo("Execution completed at %s (duration: %v)", executionEndTime.Format("2006-01-02 15:04:05.000000"), totalDuration)
	if !allCompleted {
		logger.LogError("Warning: Some execution results are missing completion times")
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
			logger.LogExecutionWithTiming(result.ScriptName, result.Success, result.Output, result.Error, result.StartTime, result.EndTime, result.Duration)
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
		// This is sent synchronously (blocking) immediately after execution completion is verified
		notificationStartTime := time.Now()
		logger.LogInfo("Sending failure notification at %s", notificationStartTime.Format("2006-01-02 15:04:05.000000"))
		if notifyErr := notify.NotifyWithSecret(cfg.Feishu.WebhookURL, cfg.Feishu.WebhookSecret, notify.StatusFailure, fullRepoName, author, commitID, failureMessage, branch, commitTime); notifyErr != nil {
			logger.LogError("failed to send Feishu notification: %v", notifyErr)
		} else {
			notificationEndTime := time.Now()
			notificationDuration := notificationEndTime.Sub(notificationStartTime)
			logger.LogInfo("Notification sent at %s (duration: %v)", notificationEndTime.Format("2006-01-02 15:04:05.000000"), notificationDuration)
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
		logger.LogExecutionWithTiming(result.ScriptName, result.Success, result.Output, result.Error, result.StartTime, result.EndTime, result.Duration)
	}

	// Send Feishu notification for success
	// This is sent synchronously (blocking) immediately after execution completion is verified
	notificationStartTime := time.Now()
	logger.LogInfo("Sending success notification at %s", notificationStartTime.Format("2006-01-02 15:04:05.000000"))
	if err := notify.NotifyWithSecret(cfg.Feishu.WebhookURL, cfg.Feishu.WebhookSecret, notify.StatusSuccess, fullRepoName, author, commitID, commitMessage, branch, commitTime); err != nil {
		logger.LogError("failed to send Feishu notification: %v", err)
	} else {
		notificationEndTime := time.Now()
		notificationDuration := notificationEndTime.Sub(notificationStartTime)
		logger.LogInfo("Notification sent at %s (duration: %v)", notificationEndTime.Format("2006-01-02 15:04:05.000000"), notificationDuration)
	}

	logger.LogInfo("webhook processed successfully for commit %s", commitID)
}
