package notify

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// signFeishuRequest generates a signature for Feishu webhook requests
// According to Feishu custom bot docs:
// string_to_sign = timestamp + "\n" + secret
// sign = base64(HMAC-SHA256(key=string_to_sign, msg=""))
func signFeishuRequest(timestamp int64, secret string) (string, error) {
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)

	mac := hmac.New(sha256.New, []byte(stringToSign))
	// Sign with empty message as per Feishu spec
	mac.Write([]byte(""))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return signature, nil
}

// NotificationStatus represents the status of the notification
type NotificationStatus string

const (
	StatusStarted NotificationStatus = "started"
	StatusSuccess NotificationStatus = "success"
	StatusFailure NotificationStatus = "failure"
)

// Notify sends a Feishu card notification with commit information
func Notify(webhookURL string, commitID, commitMessage, branch string, commitTime time.Time) error {
	return NotifyWithSecret(webhookURL, "", StatusSuccess, "", "", commitID, commitMessage, branch, commitTime)
}

// NotifyWithSecret sends a Feishu card notification with optional signature
func NotifyWithSecret(webhookURL, webhookSecret string, status NotificationStatus, repoName, author, commitID, commitMessage, branch string, commitTime time.Time) error {
	card := buildCard(status, repoName, author, commitID, commitMessage, branch, commitTime)

	var payload map[string]interface{}

	if webhookSecret != "" {
		// Sign the request
		timestamp := time.Now().Unix()
		signature, err := signFeishuRequest(timestamp, webhookSecret)
		if err != nil {
			return fmt.Errorf("failed to sign request: %w", err)
		}

		payload = map[string]interface{}{
			"timestamp": timestamp,
			"sign":      signature,
			"msg_type":  "interactive",
			"card":      card,
		}
	} else {
		// No signature
		payload = map[string]interface{}{
			"msg_type": "interactive",
			"card":     card,
		}
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("feishu webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	// Check Feishu response code
	var feishuResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &feishuResp); err == nil {
		if feishuResp.Code != 0 && feishuResp.Code != -1 {
			return fmt.Errorf("feishu webhook returned error code %d: %s\nResponse body: %s", feishuResp.Code, feishuResp.Msg, string(body))
		}
	} else if len(body) > 0 {
		// If we can't parse the response, include it in the error anyway
		return fmt.Errorf("feishu webhook returned unexpected response: %s", string(body))
	}

	return nil
}

// buildCard creates a Feishu card message with status-based colors and emojis
// Returns just the card object (without msg_type wrapper)
func buildCard(status NotificationStatus, repoName, author, commitID, commitMessage, branch string, commitTime time.Time) map[string]interface{} {
	// Set default values
	if repoName == "" {
		repoName = "unknown/repo"
	}
	if author == "" {
		author = "unknown"
	}

	// Determine emoji, color, and status text based on status
	var emoji, template, statusText string
	switch status {
	case StatusStarted:
		emoji = "🚀"
		template = "blue"
		statusText = "Workflow Started"
	case StatusSuccess:
		emoji = "✅"
		template = "green"
		statusText = "Success"
	case StatusFailure:
		emoji = "🚨"
		template = "red"
		statusText = "Failure"
	default:
		emoji = "ℹ️"
		template = "blue"
		statusText = "Notification"
	}

	// Build title with emoji and repo name
	title := fmt.Sprintf("%s %s", emoji, repoName)
	if branch != "" {
		title = fmt.Sprintf("%s %s - %s", emoji, repoName, branch)
	}

	// Build elements
	elements := []map[string]interface{}{
		{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**Status:** %s\n**Author:** %s\n**Branch:** %s", statusText, author, branch),
			},
		},
		{
			"tag": "hr",
		},
		{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**Commit ID:** `%s`\n**Time:** %s", commitID, commitTime.Format("2006-01-02 15:04:05")),
			},
		},
		{
			"tag": "hr",
		},
		{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**Commit Message:**\n%s", commitMessage),
			},
		},
	}

	// Feishu card format - just the card object
	card := map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
			"enable_forward":   true,
		},
		"header": map[string]interface{}{
			"template": template,
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": title,
			},
		},
		"elements": elements,
	}

	return card
}

// NotifyStarted sends a simple text notification when workflow starts
// This is a lightweight notification sent immediately when webhook is triggered
func NotifyStarted(webhookURL, webhookSecret, repoName, actor, commitMessage string) error {
	// Truncate commit message to 200 chars
	commitMsgShort := commitMessage
	if len(commitMsgShort) > 200 {
		commitMsgShort = commitMsgShort[:200]
	}

	textContent := fmt.Sprintf("🚀 %s updated %s about %s", actor, repoName, commitMsgShort)

	var payload map[string]interface{}

	if webhookSecret != "" {
		// Sign the request
		timestamp := time.Now().Unix()
		signature, err := signFeishuRequest(timestamp, webhookSecret)
		if err != nil {
			return fmt.Errorf("failed to sign request: %w", err)
		}

		payload = map[string]interface{}{
			"timestamp": timestamp,
			"sign":      signature,
			"msg_type":  "text",
			"content": map[string]interface{}{
				"text": textContent,
			},
		}
	} else {
		// No signature
		payload = map[string]interface{}{
			"msg_type": "text",
			"content": map[string]interface{}{
				"text": textContent,
			},
		}
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("feishu webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	// Check Feishu response code
	var feishuResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &feishuResp); err == nil {
		if feishuResp.Code != 0 && feishuResp.Code != -1 {
			return fmt.Errorf("feishu webhook returned error code %d: %s\nResponse body: %s", feishuResp.Code, feishuResp.Msg, string(body))
		}
	} else if len(body) > 0 {
		// If we can't parse the response, include it in the error anyway
		return fmt.Errorf("feishu webhook returned unexpected response: %s", string(body))
	}

	return nil
}
