# Codex rollout JSONL usage schema

Captured from a live Codex CLI rollout on 2026-08-13. Codex stores the
transcript under `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`; the directory
tree is date-based rather than project-based.

The per-turn usage record is an `event_msg` line whose payload has
`type: "token_count"`. The usage counters are nested below `payload.info`:

```json
{
  "type": "event_msg",
  "timestamp": "2026-08-13T20:25:06.671Z",
  "payload": {
    "type": "token_count",
    "info": {
      "last_token_usage": {
        "input_tokens": 36839,
        "cached_input_tokens": 28416,
        "cache_write_input_tokens": 0,
        "output_tokens": 526,
        "reasoning_output_tokens": 192,
        "total_tokens": 37365
      },
      "total_token_usage": {
        "input_tokens": 105385,
        "cached_input_tokens": 76800,
        "cache_write_input_tokens": 0,
        "output_tokens": 1719,
        "reasoning_output_tokens": 465,
        "total_tokens": 107104
      },
      "model_context_window": 258400
    }
  }
}
```

The `last_token_usage` object is the current per-turn usage record to ingest.
`total_token_usage` is cumulative for the rollout and must not be ingested as
another event. `input_tokens` includes the cached portion, so ccdash stores
`input_tokens - cached_input_tokens` as uncached input and stores
`cached_input_tokens` separately as cache-read tokens. `reasoning_output_tokens`
is already included in `output_tokens`; `cache_write_input_tokens` maps to
cache-creation tokens.

The model id is recorded separately in the preceding turn context record:

```json
{
  "type": "turn_context",
  "payload": {
    "model": "gpt-5.6-luna"
  }
}
```

In practice, a token-count event is associated with the current model from the
most recent `turn_context` line in that rollout. The parser therefore keeps
the model seen while walking a file and applies it to the next usage event.
Model ids observed in the captured rollout corpus include `gpt-5.6-luna`,
`gpt-5.6-sol`, and `gpt-5.6-terra`.

Malformed lines, token-count records without a usable `last_token_usage`, and
records without a model are ignored. Codex hook payloads are status signals
only; they do not contain token or cost counters, so token accounting remains
derived from rollout JSONL.
