package metrics

import (
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

type codexModelAggregateWant struct {
	inputTokens         int64
	outputTokens        int64
	cacheReadTokens     int64
	cacheCreationTokens int64
	cost                float64
}

func TestCodexRolloutAggregationFixtures(t *testing.T) {
	root := t.TempDir()
	rolloutDir := filepath.Join(root, "2026", "08", "13")
	if err := os.MkdirAll(rolloutDir, 0755); err != nil {
		t.Fatal(err)
	}
	rolloutPath := filepath.Join(rolloutDir, "rollout-fixture.jsonl")
	writeCodexFixture(t, rolloutPath, "aggregation.jsonl")

	cache := &TokenCache{
		dbPath:   filepath.Join(root, "tokens.db"),
		cacheDir: root,
	}
	if err := cache.initDB(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.db.Close() })

	source := NewCodexSource()
	collector := &TokenCollector{
		sources:      []Source{source},
		sourceDirs:   map[string][]string{"codex": {root}},
		lookbackFrom: time.Time{},
		cache:        cache,
	}

	if err := collector.ingestJSONLFileForSource(rolloutPath, source); err != nil {
		t.Fatalf("ingest representative rollout: %v", err)
	}
	fullAggregate := codexAggregateWant{
		inputTokens:         2200,
		outputTokens:        260,
		cacheReadTokens:     1950,
		cacheCreationTokens: 350,
		prompts:             4,
		totalCost:           0.01027,
		models: map[string]codexModelAggregateWant{
			"gpt-5.6-luna":  {inputTokens: 850, outputTokens: 80, cacheReadTokens: 650, cacheCreationTokens: 100, cost: 0.00152},
			"gpt-5.6-sol":   {inputTokens: 800, outputTokens: 100, cacheReadTokens: 1000, cacheCreationTokens: 200, cost: 0.00875},
			"gpt-9-private": {inputTokens: 550, outputTokens: 80, cacheReadTokens: 300, cacheCreationTokens: 50, cost: 0},
		},
	}
	assertCodexAggregate(t, collector, fullAggregate)

	// Completed rollout files use the persisted model breakdown rather than
	// individual events, so verify the same fixture through that query path.
	if err := cache.MarkFileComplete(rolloutPath); err != nil {
		t.Fatalf("aggregate completed rollout: %v", err)
	}
	assertCodexAggregate(t, collector, fullAggregate)

	// A rollout can be rewritten to fewer lines, for example when a partial
	// file is replaced. Advance mtime beyond the cached value to trigger the
	// collector's truncation detection and re-ingestion path.
	writeCodexFixture(t, rolloutPath, "truncated.jsonl")
	_, lastModified, exists := cache.GetFileState(rolloutPath)
	if !exists {
		t.Fatal("expected cached rollout state before truncation")
	}
	modified := lastModified.Add(2 * time.Second)
	if err := os.Chtimes(rolloutPath, modified, modified); err != nil {
		t.Fatal(err)
	}
	if err := cache.MarkFileActive(rolloutPath); err != nil {
		t.Fatalf("mark rewritten rollout active: %v", err)
	}
	if err := collector.ingestJSONLFileForSource(rolloutPath, source); err != nil {
		t.Fatalf("re-ingest truncated rollout: %v", err)
	}
	assertCodexAggregate(t, collector, codexAggregateWant{
		inputTokens:         2400,
		outputTokens:        150,
		cacheReadTokens:     500,
		cacheCreationTokens: 100,
		prompts:             1,
		totalCost:           0.017375,
		models: map[string]codexModelAggregateWant{
			"gpt-5.6-sol": {inputTokens: 2400, outputTokens: 150, cacheReadTokens: 500, cacheCreationTokens: 100, cost: 0.017375},
		},
	})
}

type codexAggregateWant struct {
	inputTokens         int64
	outputTokens        int64
	cacheReadTokens     int64
	cacheCreationTokens int64
	prompts             int64
	totalCost           float64
	models              map[string]codexModelAggregateWant
}

func assertCodexAggregate(t *testing.T, collector *TokenCollector, want codexAggregateWant) {
	t.Helper()
	got, err := collector.Collect()
	if err != nil {
		t.Fatalf("collect rollout metrics: %v", err)
	}
	if !got.Available {
		t.Fatalf("Codex metrics unavailable: %s", got.Error)
	}
	if got.InputTokens != want.inputTokens || got.OutputTokens != want.outputTokens ||
		got.CacheReadTokens != want.cacheReadTokens || got.CacheCreationTokens != want.cacheCreationTokens ||
		got.Prompts != want.prompts {
		t.Fatalf("aggregate counters = input %d, output %d, cache read %d, cache creation %d, events %d; want input %d, output %d, cache read %d, cache creation %d, events %d",
			got.InputTokens, got.OutputTokens, got.CacheReadTokens, got.CacheCreationTokens, got.Prompts,
			want.inputTokens, want.outputTokens, want.cacheReadTokens, want.cacheCreationTokens, want.prompts)
	}
	if got.TotalTokens != want.inputTokens+want.outputTokens+want.cacheReadTokens+want.cacheCreationTokens {
		t.Errorf("total tokens = %d, want sum of disjoint counters %d", got.TotalTokens,
			want.inputTokens+want.outputTokens+want.cacheReadTokens+want.cacheCreationTokens)
	}
	if math.Abs(got.TotalCost-want.totalCost) > 1e-12 {
		t.Errorf("total cost = %.12f, want %.12f", got.TotalCost, want.totalCost)
	}

	wantModels := make([]string, 0, len(want.models))
	for model := range want.models {
		wantModels = append(wantModels, model)
	}
	sort.Strings(wantModels)
	if len(got.Models) != len(wantModels) {
		t.Errorf("models = %v, want %v", got.Models, wantModels)
	} else {
		for i, model := range wantModels {
			if got.Models[i] != model {
				t.Errorf("models = %v, want %v", got.Models, wantModels)
				break
			}
		}
	}
	if len(got.ModelUsages) != len(want.models) {
		t.Fatalf("model usage count = %d, want %d: %+v", len(got.ModelUsages), len(want.models), got.ModelUsages)
	}
	for _, usage := range got.ModelUsages {
		modelWant, ok := want.models[usage.Model]
		if !ok {
			t.Errorf("unexpected model usage: %+v", usage)
			continue
		}
		if usage.Source != "codex" || usage.InputTokens != modelWant.inputTokens || usage.OutputTokens != modelWant.outputTokens ||
			usage.CacheReadTokens != modelWant.cacheReadTokens || usage.CacheCreationTokens != modelWant.cacheCreationTokens {
			t.Errorf("model %q counters/source = %+v, want %+v", usage.Model, usage, modelWant)
		}
		if math.Abs(usage.Cost-modelWant.cost) > 1e-12 {
			t.Errorf("model %q cost = %.12f, want %.12f", usage.Model, usage.Cost, modelWant.cost)
		}
	}
}

func writeCodexFixture(t *testing.T, destination, name string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "codex", name))
	if err != nil {
		t.Fatalf("read Codex fixture %q: %v", name, err)
	}
	if err := os.WriteFile(destination, data, 0600); err != nil {
		t.Fatalf("write Codex fixture %q: %v", name, err)
	}
}
