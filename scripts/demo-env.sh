#!/usr/bin/env bash
# Build a synthetic ccdash environment for screenshots and demos.
#
#   scripts/demo-env.sh DIR        # create DIR/home, DIR/tmux, DIR/bin and demo sessions
#   scripts/demo-env.sh DIR stop   # stop the demo tmux server
#
# Everything is generated: transcripts, hook records and agent sessions use
# made-up projects and session IDs, and the tmux sessions run on a private
# server (its own TMUX_TMPDIR), so no real session, workspace or cost can
# appear. The System panel still shows the host it runs on.
#
# Run ccdash against it with the command this script prints at the end.
set -euo pipefail

dir=${1:?usage: demo-env.sh DIR [stop]}
mkdir -p "$dir"
dir=$(cd "$dir" && pwd)
# A short socket directory: Unix socket paths are limited to ~108 bytes.
export TMUX_TMPDIR="/tmp/ccdash-demo-$(id -u)"
mkdir -p "$TMUX_TMPDIR"
unset TMUX

if [ "${2:-}" = stop ]; then
	tmux kill-server 2>/dev/null || true
	exit 0
fi

home="$dir/home"
bin="$dir/bin"
mkdir -p "$home" "$bin"
now=$(date -u +%s)

iso() { date -u -d "@$1" +%Y-%m-%dT%H:%M:%S.000Z; }

# claude_transcript PROJECT SESSION_ID MODEL REQUESTS SPAN_SECONDS
claude_transcript() {
	local project=$1 id=$2 model=$3 n=$4 span=$5 i ts
	local file="$home/.claude/projects/-home-demo-$project/$id.jsonl"
	mkdir -p "$(dirname "$file")"
	: >"$file"
	for ((i = n; i >= 1; i--)); do
		ts=$((now - span * i / n))
		printf '{"type":"user","timestamp":"%s","message":{"role":"user"}}\n' "$(iso "$ts")" >>"$file"
		printf '{"type":"assistant","timestamp":"%s","message":{"model":"%s","usage":{"input_tokens":%d,"output_tokens":%d,"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d}}}\n' \
			"$(iso "$ts")" "$model" $((40 + i % 90)) $((900 + (i * 37) % 2400)) $((60000 + (i * 911) % 90000)) $((1500 + (i * 53) % 6000)) >>"$file"
	done
}

# codex_rollout SESSION_ID MODEL REQUESTS SPAN_SECONDS
codex_rollout() {
	local id=$1 model=$2 n=$3 span=$4 i ts input cached
	local day; day=$(date -u -d "@$((now - span))" +%Y/%m/%d)
	local file="$home/.codex/sessions/$day/rollout-$(date -u -d "@$((now - span))" +%Y-%m-%dT%H-%M-%S)-$id.jsonl"
	mkdir -p "$(dirname "$file")"
	printf '{"type":"turn_context","timestamp":"%s","payload":{"model":"%s"}}\n' "$(iso $((now - span)))" "$model" >"$file"
	for ((i = n; i >= 1; i--)); do
		ts=$((now - span * i / n))
		cached=$((70000 + (i * 733) % 80000))
		input=$((cached + 300 + i % 500))
		printf '{"type":"event_msg","timestamp":"%s","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":%d,"cached_input_tokens":%d,"cache_write_input_tokens":0,"output_tokens":%d,"total_tokens":%d}}}}\n' \
			"$(iso "$ts")" "$input" "$cached" $((700 + (i * 29) % 1800)) $((input + 700)) >>"$file"
	done
}

# Interactive sessions tracked by hooks: per-session cost comes from the
# transcript named after the session ID.
claude_transcript api-server 3f2b9c1e-5a7d-4e8f-9b10-2c3d4e5f6a7b claude-opus-5-5 180 7200
claude_transcript docs-site 8d4e2f60-1b3c-4d5e-8f70-9a1b2c3d4e5f claude-sonnet-5-5 120 5400
codex_rollout 0a1b2c3d-4e5f-4061-8728-394a5b6c7d8e gpt-6-sol 90 3600
# Background history for the week's totals.
claude_transcript billing-svc 5c6d7e8f-9a0b-4c1d-8e2f-3a4b5c6d7e8f claude-sonnet-5-5 400 172800
claude_transcript data-pipeline 6d7e8f9a-0b1c-4d2e-9f3a-4b5c6d7e8f9a claude-haiku-4-5 300 129600
codex_rollout 7e8f9a0b-1c2d-4e3f-8a4b-5c6d7e8f9a0b gpt-6-luna 500 158400
codex_rollout 8f9a0b1c-2d3e-4f4a-9b5c-6d7e8f9a0b1c gpt-5.6-sol 160 86400

hook() { # hook ID HARNESS TMUX_NAME PROJECT STATUS
	mkdir -p "$home/.ccdash/sessions"
	printf '{"session_id":"%s","source":"%s","project_dir":"/home/demo/%s","tmux_session_name":"%s","started_at":"%s","last_activity":"%s","status":"%s"}\n' \
		"$1" "$2" "$4" "$3" "$(iso $((now - 7200)))" "$(iso "$now")" "$5" >"$home/.ccdash/sessions/$1.json"
}
hook 3f2b9c1e-5a7d-4e8f-9b10-2c3d4e5f6a7b claude api-server api-server working
hook 8d4e2f60-1b3c-4d5e-8f70-9a1b2c3d4e5f claude docs-site docs-site asking
hook 0a1b2c3d-4e5f-4061-8728-394a5b6c7d8e codex infra-review infra-review working

# Fake agents. The shebang names bash directly so the process keeps the
# agent's name (ccdash identifies agent sessions by process name).
bash_path=$(command -v bash)
agent() { # agent NAME
	cat >"$bin/$1" <<SCRIPT
#!$bash_path
# Demo stand-in for \$0: draws a busy agent screen and keeps redrawing.
mode=\${1:-working}
while true; do
	clear
	if [ "\$mode" = working ]; then
		printf '● Running the test suite\n\n  ⎿  ok  ./internal/metrics  4.1s\n\n✻ Thinking… (esc to interrupt)\n'
	else
		printf '● Ready.\n\n> \n'
	fi
	sleep 2
done
SCRIPT
	chmod +x "$bin/$1"
}
agent claude
agent codex

tmux kill-server 2>/dev/null || true
start() { tmux new-session -d -s "$1" -x 120 -y 30 "$2"; }
start api-server "$bin/claude working"
start docs-site "$bin/claude idle"
start infra-review "$bin/codex working"
start claude-code-sonnet-alpha "$bin/claude working"
start claude-code-sonnet-bravo "$bin/claude idle"
start claude-code-glm-charlie "$bin/claude working"
start opencode-zen-delta "$bin/claude idle"

echo "Demo environment ready in $dir"
echo "Run: env -u TMUX HOME=$home TMUX_TMPDIR=$TMUX_TMPDIR PATH=$bin:\$PATH ccdash"
