package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// HookInstaller is the harness-specific status hook lifecycle used by a
// Source. Hook payloads are deliberately not a token source.
type HookInstaller interface {
	InstallHooks() error
	AreHooksInstalled() bool
	UninstallHooks() error
}

// Source describes one coding harness that ccdash can ingest.
type Source interface {
	Name() string
	ProjectDirs(home string) []string
	ParseUsageLine(raw []byte) (*TokenEvent, bool, error)
	PricingForModel(model string) ModelPricing
	HookInstaller() HookInstaller
}

// ClaudeSource adapts the existing Claude Code JSONL schema to Source.
type ClaudeSource struct{}

func NewClaudeSource() *ClaudeSource { return &ClaudeSource{} }

func (s *ClaudeSource) Name() string { return "claude" }

func (s *ClaudeSource) ProjectDirs(home string) []string {
	return buildDefaultProjectsDirs(home)
}

func (s *ClaudeSource) ParseUsageLine(raw []byte) (*TokenEvent, bool, error) {
	var msg claudeMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, false, err
	}
	if msg.Type != "assistant" {
		return nil, false, nil
	}
	timestamp, err := time.Parse(time.RFC3339Nano, msg.Timestamp)
	if err != nil {
		return nil, false, nil
	}
	usage := msg.Message.Usage
	cacheCreation := usage.CacheCreationInputTokens
	if cacheCreation == 0 {
		cacheCreation = usage.CacheCreation.Ephemeral5mInputTokens +
			usage.CacheCreation.Ephemeral1hInputTokens
	}
	return &TokenEvent{
		Timestamp:           timestamp,
		Model:               msg.Message.Model,
		Source:              s.Name(),
		InputTokens:         usage.InputTokens,
		OutputTokens:        usage.OutputTokens,
		CacheReadTokens:     usage.CacheReadInputTokens,
		CacheCreationTokens: cacheCreation,
	}, true, nil
}

func (s *ClaudeSource) PricingForModel(model string) ModelPricing {
	return getPricingForModel(model)
}

func (s *ClaudeSource) HookInstaller() HookInstaller {
	installer, _ := NewHookSessionCollector()
	return installer
}

// sourceDirs returns source roots after applying the historical extra-dir
// behavior to Claude while keeping Codex's date-tree root independent.
func sourceDirs(source Source, home string) []string {
	if source == nil {
		return nil
	}
	dirs := source.ProjectDirs(home)
	result := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			result = append(result, filepath.Clean(dir))
		}
	}
	return result
}
