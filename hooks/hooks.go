package hooks

import (
	"context"
	"fmt"
	"os/exec"
	"sentinel/logger"
	"strings"
	"time"
)

// HookType defines when a hook runs
type HookType string

const (
	HookPreCheck    HookType = "pre-check"    // before checking for update
	HookPostCheck   HookType = "post-check"   // after checking for update
	HookPreUpdate   HookType = "pre-update"   // before pulling/stopping
	HookPostUpdate  HookType = "post-update"  // after container is running again
	HookPreRollback HookType = "pre-rollback" // before rollback
	HookPostRollback HookType = "post-rollback" // after rollback
)

// Hook defines a single lifecycle hook
type Hook struct {
	Type    HookType
	Command string // shell command to run
	Timeout int    // seconds before killing command
}

// HookResult holds the result of a hook execution
type HookResult struct {
	Type     HookType
	Command  string
	Output   string
	Error    error
	Duration time.Duration
	ExitCode int
}

// Runner executes lifecycle hooks
type Runner struct {
	hooks map[HookType][]Hook
}

// New creates a new hook Runner
func New() *Runner {
	return &Runner{
		hooks: make(map[HookType][]Hook),
	}
}

// Register adds a hook for a given type
func (r *Runner) Register(h Hook) {
	if h.Timeout == 0 {
		h.Timeout = 30
	}
	r.hooks[h.Type] = append(r.hooks[h.Type], h)
	logger.Log.WithFields(logger.Fields{
		"type":    h.Type,
		"command": h.Command,
	}).Debug("🪝  Hook registered")
}

// Run executes all hooks for a given type
// Returns error only if a hook exits non-zero
func (r *Runner) Run(hookType HookType, containerName string, image string) error {
	hooks, ok := r.hooks[hookType]
	if !ok || len(hooks) == 0 {
		return nil
	}

	logger.Log.WithFields(logger.Fields{
		"type":      hookType,
		"container": containerName,
		"count":     len(hooks),
	}).Infof("🪝  Running %s hooks for %s", hookType, containerName)

	for _, h := range hooks {
		result := r.runOne(h, containerName, image)
		if result.Error != nil {
			logger.Log.WithFields(logger.Fields{
				"type":      hookType,
				"container": containerName,
				"command":   h.Command,
				"exit_code": result.ExitCode,
				"output":    result.Output,
			}).Errorf("🪝  Hook failed: %v", result.Error)
			return fmt.Errorf("hook %s failed: %v", hookType, result.Error)
		}
		logger.Log.WithFields(logger.Fields{
			"type":      hookType,
			"container": containerName,
			"duration":  result.Duration,
		}).Infof("🪝  Hook %s completed", hookType)
	}

	return nil
}

// RunSoft runs hooks but does not fail on error - used for post hooks
func (r *Runner) RunSoft(hookType HookType, containerName string, image string) {
	if err := r.Run(hookType, containerName, image); err != nil {
		logger.Log.Warnf("🪝  Soft hook %s failed (non-fatal): %v", hookType, err)
	}
}

// runOne executes a single hook command
func (r *Runner) runOne(h Hook, containerName string, image string) HookResult {
	start := time.Now()

	result := HookResult{
		Type:    h.Type,
		Command: h.Command,
	}

	// Expand variables in command
	cmd := expandVars(h.Command, containerName, image)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(h.Timeout)*time.Second,
	)
	defer cancel()

	// Run command via shell
	c := exec.CommandContext(ctx, "sh", "-c", cmd)

	output, err := c.CombinedOutput()
	result.Duration = time.Since(start)
	result.Output = strings.TrimSpace(string(output))

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Error = fmt.Errorf("hook timed out after %ds", h.Timeout)
			result.ExitCode = -1
		} else {
			if exitErr, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exitErr.ExitCode()
			}
			result.Error = fmt.Errorf("hook exited %d: %v", result.ExitCode, err)
		}
	}

	return result
}

// expandVars replaces template variables in hook command
// Available variables:
//   {{container}} - container name
//   {{image}}     - full image name
func expandVars(cmd string, containerName string, image string) string {
	cmd = strings.ReplaceAll(cmd, "{{container}}", containerName)
	cmd = strings.ReplaceAll(cmd, "{{image}}", image)
	return cmd
}

// ParseHooksFromLabels reads hook commands from container labels
// Label format:
//   sentinel.hook.pre-update=echo "starting update of {{container}}"
//   sentinel.hook.post-update=curl http://myapp/reload
//   sentinel.hook.pre-check=echo "checking {{container}}"
func ParseHooksFromLabels(labels map[string]string) []Hook {
	var hooks []Hook

	hookTypes := []HookType{
		HookPreCheck,
		HookPostCheck,
		HookPreUpdate,
		HookPostUpdate,
		HookPreRollback,
		HookPostRollback,
	}

	for _, ht := range hookTypes {
		labelKey := "sentinel.hook." + string(ht)
		if cmd, ok := labels[labelKey]; ok && cmd != "" {
			hooks = append(hooks, Hook{
				Type:    ht,
				Command: cmd,
				Timeout: 30,
			})
			logger.Log.WithFields(logger.Fields{
				"type":    ht,
				"command": cmd,
			}).Debug("🪝  Hook loaded from label")
		}
	}

	return hooks
}