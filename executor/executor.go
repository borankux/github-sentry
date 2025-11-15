package executor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ExecutionResult represents the result of executing a script
type ExecutionResult struct {
	ScriptName string
	Success    bool
	Output     string
	Error      string
}

// ExecuteScripts executes scripts from the specified folder sequentially
// Scripts are expected to be named like 001.sh, 002.sh, etc.
// Stops on first failure
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

// executeScript executes a single script
func executeScript(scriptPath string) ExecutionResult {
	scriptName := filepath.Base(scriptPath)

	cmd := exec.Command("bash", scriptPath)
	output, err := cmd.CombinedOutput()

	result := ExecutionResult{
		ScriptName: scriptName,
		Output:     string(output),
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

