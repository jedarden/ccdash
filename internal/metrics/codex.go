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

		// Codex's input_tokens includes both cache reads and cache writes. Keep
		// every dashboard counter mutually exclusive, just as Claude's
		// input/cache counters are.
		input := info.LastTokenUsage.InputTokens -
			info.LastTokenUsage.CachedInputTokens -
			info.LastTokenUsage.CacheWriteInputTokens
		if input < 0 {
			input = 0
		}
		// A request whose prompt exceeds OpenAI's long-context threshold is
		// billed at the long-context rate. Tag it in the model id so the
		// cache, aggregates and pricing keep it apart without a schema change.
		if info.LastTokenUsage.InputTokens > codexLongContextThreshold {
			model += CodexLongContextSuffix
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

// OpenAI bills a request whose prompt (input_tokens, which includes cached
// input) exceeds 272K tokens at a long-context rate. Such requests are
// recorded under "<model>:long-context" and priced from
// codexLongContextPricing.
const (
	codexLongContextThreshold = 272_000
	CodexLongContextSuffix    = ":long-context"
)

// codexLongContextPricing holds OpenAI's published standard-tier long-context
// (>272K input tokens) rates, checked 2026-10-07 at
// https://developers.openai.com/api/docs/pricing.
var codexLongContextPricing = map[string]ModelPricing{
	"gpt-6-astra": {
		InputPerMillion: 20.00, OutputPerMillion: 75.00,
		CacheReadPerMillion: 2.00, CacheCreatePerMillion: 25.00,
	},
	"gpt-6.1-sol": {
		InputPerMillion: 4.00, OutputPerMillion: 15.00,
		CacheReadPerMillion: 0.20, CacheCreatePerMillion: 5.00,
	},
	"gpt-6-sol": {
		InputPerMillion: 4.00, OutputPerMillion: 15.00,
		CacheReadPerMillion: 0.40, CacheCreatePerMillion: 5.00,
	},
	"gpt-6-luna": {
		InputPerMillion: 0.20, OutputPerMillion: 0.75,
		CacheReadPerMillion: 0.02, CacheCreatePerMillion: 0.25,
	},
	"gpt-5.6-sol": {
		InputPerMillion: 8.00, OutputPerMillion: 30.00,
		CacheReadPerMillion: 0.80, CacheCreatePerMillion: 10.00,
	},
	"gpt-5.6-terra": {
		InputPerMillion: 4.00, OutputPerMillion: 18.00,
		CacheReadPerMillion: 0.40, CacheCreatePerMillion: 5.00,
	},
	"gpt-5.6-luna": {
		InputPerMillion: 0.40, OutputPerMillion: 1.80,
		CacheReadPerMillion: 0.04, CacheCreatePerMillion: 0.50,
	},
}

// codexPricing is maintained separately from Claude pricing because OpenAI's
// model and cache rates have a different release cadence.
//
// The gpt-5.6 and gpt-6 rows are OpenAI's published standard-tier,
// short-context (<=272K input tokens) rates, checked 2026-10-07 at
// https://developers.openai.com/api/docs/pricing.
var codexPricing = map[string]ModelPricing{
	"gpt-6-astra": {
		InputPerMillion: 10.00, OutputPerMillion: 50.00,
		CacheReadPerMillion: 1.00, CacheCreatePerMillion: 12.50,
	},
	"gpt-6.1-sol": {
		InputPerMillion: 2.00, OutputPerMillion: 10.00,
		CacheReadPerMillion: 0.10, CacheCreatePerMillion: 2.50,
	},
	"gpt-6-sol": {
		InputPerMillion: 2.00, OutputPerMillion: 10.00,
		CacheReadPerMillion: 0.20, CacheCreatePerMillion: 2.50,
	},
	"gpt-6-luna": {
		InputPerMillion: 0.10, OutputPerMillion: 0.50,
		CacheReadPerMillion: 0.01, CacheCreatePerMillion: 0.125,
	},
	"gpt-5.6-sol": {
		InputPerMillion: 4.00, OutputPerMillion: 20.00,
		CacheReadPerMillion: 0.40, CacheCreatePerMillion: 5.00,
	},
	"gpt-5.6-terra": {
		InputPerMillion: 2.00, OutputPerMillion: 12.00,
		CacheReadPerMillion: 0.20, CacheCreatePerMillion: 2.50,
	},
	"gpt-5.6-luna": {
		InputPerMillion: 0.20, OutputPerMillion: 1.20,
		CacheReadPerMillion: 0.02, CacheCreatePerMillion: 0.25,
	},
	// gpt-5.3-codex is the one Codex-specific model on the same page
	// (Specialized models). gpt-5-codex, gpt-5.2-codex and codex-mini-latest
	// were removed 2026-10-07: not on the page and never seen in this
	// workspace's token history, so a price for them could not be sourced.
	// Any use now shows as an unpriced model ('?').
	"gpt-5.3-codex": {
		InputPerMillion: 1.75, OutputPerMillion: 14.00,
		CacheReadPerMillion: 0.175, CacheCreatePerMillion: 2.1875,
	},
}

func (s *CodexSource) PricingForModel(model string) ModelPricing {
	return s.pricingDetailsForModel(model).pricing
}

func (s *CodexSource) pricingDetailsForModel(model string) pricingDetails {
	if base, long := strings.CutSuffix(model, CodexLongContextSuffix); long {
		if pricing, ok := codexLongContextPricing[base]; ok {
			return pricingDetails{pricing: pricing}
		}
		// No published long-context rate: the short-context rate would
		// understate, so mark it estimated.
		return pricingDetails{pricing: s.pricingDetailsForModel(base).pricing, estimated: true}
	}
	if pricing, ok := codexPricing[model]; ok {
		return pricingDetails{pricing: pricing}
	}
	// Codex may emit dated snapshots. Prefer the stable model family when a
	// snapshot suffix is present; unknown models use zero pricing rather than
	// accidentally applying a Claude rate, marked estimated so the $0 is not
	// read as a real price.
	for prefix, pricing := range codexPricing {
		if strings.HasPrefix(model, prefix+"-") {
			return pricingDetails{pricing: pricing, estimated: true}
		}
	}
	return pricingDetails{estimated: true}
}

func (s *CodexSource) HookInstaller() HookInstaller {
	return NewCodexHookInstaller()
}
