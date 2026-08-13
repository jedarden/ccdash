package metrics

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CodexSource reads usage events from ~/.codex/sessions/YYYY/MM/DD rollout
// files. Codex emits the model in turn_context and the counters in a later
// token_count event, so the parser retains the current model while one file is
// being walked. TokenCollector calls Reset before each file.
type CodexSource struct {
	mu        sync.Mutex
	model     string
	modelSeen bool
}

func NewCodexSource() *CodexSource { return &CodexSource{} }

func (s *CodexSource) Name() string { return "codex" }

func (s *CodexSource) ProjectDirs(home string) []string {
	if home == "" {
		return nil
	}
	return []string{filepath.Join(home, ".codex", "sessions")}
}

func (s *CodexSource) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.model = ""
	s.modelSeen = false
}

type codexRolloutLine struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type codexTurnContext struct {
	Model string `json:"model"`
}

type codexPayload struct {
	Type string          `json:"type"`
	Info json.RawMessage `json:"info"`
}

type codexTokenInfo struct {
	LastTokenUsage codexTokenUsage `json:"last_token_usage"`
}

type codexTokenUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	CacheWriteInputTokens int64 `json:"cache_write_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
	TotalTokens           int64 `json:"total_tokens"`
}

func (s *CodexSource) ParseUsageLine(raw []byte) (*TokenEvent, bool, error) {
	var line codexRolloutLine
	if err := json.Unmarshal(raw, &line); err != nil {
		return nil, false, err
	}

	switch line.Type {
	case "turn_context":
		var context codexTurnContext
		if err := json.Unmarshal(line.Payload, &context); err != nil {
			return nil, false, err
		}
		s.mu.Lock()
		s.model = strings.TrimSpace(context.Model)
		s.modelSeen = s.model != ""
		s.mu.Unlock()
		return nil, false, nil
	case "event_msg":
		var payload codexPayload
		if err := json.Unmarshal(line.Payload, &payload); err != nil {
			return nil, false, err
		}
		if payload.Type != "token_count" {
			return nil, false, nil
		}
		var info codexTokenInfo
		if err := json.Unmarshal(payload.Info, &info); err != nil {
			return nil, false, err
		}
		if info.LastTokenUsage.TotalTokens == 0 &&
			info.LastTokenUsage.InputTokens == 0 &&
			info.LastTokenUsage.OutputTokens == 0 {
			return nil, false, nil
		}

		s.mu.Lock()
		model := s.model
		seen := s.modelSeen
		s.mu.Unlock()
		if !seen || model == "" {
			return nil, false, nil
		}
		timestamp, err := time.Parse(time.RFC3339Nano, line.Timestamp)
		if err != nil {
			return nil, false, nil
		}

		// Codex's input_tokens includes cached_input_tokens. Keep the dashboard
		// counters mutually exclusive, just as Claude's input/cache counters are.
		input := info.LastTokenUsage.InputTokens - info.LastTokenUsage.CachedInputTokens
		if input < 0 {
			input = 0
		}
		return &TokenEvent{
			Timestamp:           timestamp,
			Model:               model,
			Source:              s.Name(),
			InputTokens:         input,
			OutputTokens:        info.LastTokenUsage.OutputTokens,
			CacheReadTokens:     info.LastTokenUsage.CachedInputTokens,
			CacheCreationTokens: info.LastTokenUsage.CacheWriteInputTokens,
		}, true, nil
	default:
		return nil, false, nil
	}
}

// codexPricing is maintained separately from Claude pricing because OpenAI's
// model and cache rates have a different release cadence.
var codexPricing = map[string]ModelPricing{
	"gpt-5.6-luna": {
		InputPerMillion: 1.00, OutputPerMillion: 6.00,
		CacheReadPerMillion: 0.10, CacheCreatePerMillion: 1.25,
	},
	"gpt-5.6-terra": {
		InputPerMillion: 2.50, OutputPerMillion: 15.00,
		CacheReadPerMillion: 0.25, CacheCreatePerMillion: 3.125,
	},
	"gpt-5.6-sol": {
		InputPerMillion: 5.00, OutputPerMillion: 30.00,
		CacheReadPerMillion: 0.50, CacheCreatePerMillion: 6.25,
	},
	"gpt-5-codex": {
		InputPerMillion: 1.25, OutputPerMillion: 10.00,
		CacheReadPerMillion: 0.125, CacheCreatePerMillion: 1.5625,
	},
	"gpt-5.3-codex": {
		InputPerMillion: 1.75, OutputPerMillion: 14.00,
		CacheReadPerMillion: 0.175, CacheCreatePerMillion: 2.1875,
	},
	"gpt-5.2-codex": {
		InputPerMillion: 1.75, OutputPerMillion: 14.00,
		CacheReadPerMillion: 0.175, CacheCreatePerMillion: 2.1875,
	},
	"codex-mini-latest": {
		InputPerMillion: 1.50, OutputPerMillion: 6.00,
		CacheReadPerMillion: 0.15, CacheCreatePerMillion: 1.875,
	},
}

func (s *CodexSource) PricingForModel(model string) ModelPricing {
	if pricing, ok := codexPricing[model]; ok {
		return pricing
	}
	// Codex may emit dated snapshots. Prefer the stable model family when a
	// snapshot suffix is present; unknown models use zero pricing rather than
	// accidentally applying a Claude rate.
	for prefix, pricing := range codexPricing {
		if strings.HasPrefix(model, prefix+"-") {
			return pricing
		}
	}
	return ModelPricing{}
}

func (s *CodexSource) HookInstaller() HookInstaller {
	return NewCodexHookInstaller()
}
