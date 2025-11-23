package executor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ExecutionResult represents the result of executing a script or command
type ExecutionResult struct {
	ScriptName string
	Success    bool
	Output     string
	Error      string
	StartTime  time.Time
	EndTime    time.Time
	Duration   time.Duration
}

// ExecuteCommands executes commands from config with branch and repo context
// Sequential commands run one after another, stopping on first failure
// Async commands run in parallel
func ExecuteCommands(sequentialCommands, asyncCommands []string, branch, repoName string) ([]ExecutionResult, error) {
	results := make([]ExecutionResult, 0)
	
	// Set up environment variables for scripts
	env := os.Environ()
	env = append(env, fmt.Sprintf("GITHUB_BRANCH=%s", branch))
	env = append(env, fmt.Sprintf("GITHUB_REPO=%s", repoName))
	env = append(env, fmt.Sprintf("GITHUB_REPOSITORY=%s", repoName))
	
	// Execute sequential commands first (stop on failure)
	for _, cmd := range sequentialCommands {
		if cmd == "" {
			continue
		}
		result := executeCommand(cmd, env)
		results = append(results, result)
		
		if !result.Success {
			// Stop on first failure
			return results, fmt.Errorf("command failed: %s - %s", result.ScriptName, result.Error)
		}
	}
	
	// Execute async commands in parallel
	if len(asyncCommands) > 0 {
		var wg sync.WaitGroup
		asyncResults := make([]ExecutionResult, 0)
		mu := sync.Mutex{}
		
		for _, cmd := range asyncCommands {
			if cmd == "" {
				continue
			}
			wg.Add(1)
			go func(command string) {
				defer wg.Done()
				result := executeCommand(command, env)
				mu.Lock()
				asyncResults = append(asyncResults, result)
				mu.Unlock()
			}(cmd)
		}
		
		// Wait for all async commands to complete
		// This blocks until the last command finishes - ensuring all commands have completed
		// Each command's EndTime is recorded when it finishes, so we can determine
		// the true completion time from the results
		wg.Wait()
		
		results = append(results, asyncResults...)
	}
	
	// All commands have completed at this point
	// Individual results contain their StartTime, EndTime, and Duration
	// Overall execution timing is calculated in the webhook handler from these results
	
	return results, nil
}

// ExecuteScripts executes scripts from the specified folder sequentially
// Scripts are expected to be named like 001.sh, 002.sh, etc.
// Stops on first failure
// Deprecated: Use ExecuteCommands instead
func ExecuteScripts(scriptsFolder string) ([]ExecutionResult, error) {
	scripts, err := GetScripts(scriptsFolder)
	if err != nil {
		return nil, fmt.Errorf("failed to get scripts: %w", err)
	}

	if len(scripts) == 0 {
		return []ExecutionResult{}, nil
	}

	results := make([]ExecutionResult, 0, len(scripts))

	for _, script := range scripts {
		result := executeScript(script)
		results = append(results, result)

		if !result.Success {
			// Stop on first failure
			return results, fmt.Errorf("script %s failed: %s", result.ScriptName, result.Error)
		}
	}

	return results, nil
}

// GetScripts returns a sorted list of script files from the folder
func GetScripts(scriptsFolder string) ([]string, error) {
	entries, err := os.ReadDir(scriptsFolder)
	if err != nil {
		return nil, fmt.Errorf("failed to read scripts folder: %w", err)
	}

	scripts := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".sh") {
			continue
		}

		// Check if it starts with a number pattern (001, 002, etc.)
		baseName := strings.TrimSuffix(name, ".sh")
		if _, err := strconv.Atoi(baseName); err != nil {
			// Skip files that don't match the numeric pattern
			continue
		}

		scripts = append(scripts, filepath.Join(scriptsFolder, name))
	}

	// Sort scripts numerically
	sort.Slice(scripts, func(i, j int) bool {
		baseI := strings.TrimSuffix(filepath.Base(scripts[i]), ".sh")
		baseJ := strings.TrimSuffix(filepath.Base(scripts[j]), ".sh")
		numI, _ := strconv.Atoi(baseI)
		numJ, _ := strconv.Atoi(baseJ)
		return numI < numJ
	})

	return scripts, nil
}

// executeCommand executes a single command with environment variables
func executeCommand(command string, env []string) ExecutionResult {
	// Record start time before executing the command
	startTime := time.Now()
	
	// Parse command - support both shell commands and script paths
	var cmd *exec.Cmd
	if strings.HasSuffix(command, ".sh") || strings.HasPrefix(command, "./") || strings.HasPrefix(command, "/") {
		// It's a script file
		cmd = exec.Command("bash", command)
	} else {
		// It's a shell command
		cmd = exec.Command("bash", "-c", command)
	}
	
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	
	// Record end time immediately after command completes
	endTime := time.Now()
	duration := endTime.Sub(startTime)

	result := ExecutionResult{
		ScriptName: command,
		Output:     string(output),
		StartTime:  startTime,
		EndTime:    endTime,
		Duration:   duration,
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		if result.Output == "" {
			result.Output = err.Error()
		}
	} else {
		result.Success = true
	}

	return result
}

// executeScript executes a single script
// Deprecated: Use executeCommand instead
func executeScript(scriptPath string) ExecutionResult {
	scriptName := filepath.Base(scriptPath)

	// Record start time before executing the script
	startTime := time.Now()
	
	cmd := exec.Command("bash", scriptPath)
	output, err := cmd.CombinedOutput()
	
	// Record end time immediately after script completes
	endTime := time.Now()
	duration := endTime.Sub(startTime)

	result := ExecutionResult{
		ScriptName: scriptName,
		Output:     string(output),
		StartTime:  startTime,
		EndTime:    endTime,
		Duration:   duration,
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		if result.Output == "" {
			result.Output = err.Error()
		}
	} else {
		result.Success = true
	}

	return result
}

