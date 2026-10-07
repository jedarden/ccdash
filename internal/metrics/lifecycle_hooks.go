package metrics

// Scripts never infer permission or completion from assistant prose. Only native
// harness events and explicit task reports change the corresponding state.
const sessionHookCommon = `#!/usr/bin/env bash
set -eEo pipefail

CCDASH_DIR="$HOME/.ccdash"
SESSIONS_DIR="$CCDASH_DIR/sessions"
hook_error() {
    local status="$1" line="$2"
    trap - ERR
    (umask 077; printf '%s hook=%s exit=%s line=%s\n' \
        "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "${0##*/}" "$status" "$line" \
        >> "$CCDASH_DIR/$HARNESS-hook-errors.log") 2>/dev/null || true
    exit "$status"
}
trap 'hook_error "$?" "$LINENO"' ERR
INPUT=$(cat)
SESSION_ID=$(printf '%s' "$INPUT" | jq -r '.session_id // empty')
case "$SESSION_ID" in ""|*[!a-zA-Z0-9_.-]*) exit 0 ;; esac
SESSION_FILE="$SESSIONS_DIR/${SESSION_ID}.json"
TMP_FILE=""
trap 'if [ -n "$TMP_FILE" ]; then rm -f -- "$TMP_FILE"; fi' EXIT

lock_sessions() {
    mkdir -p "$SESSIONS_DIR"
    if command -v flock >/dev/null 2>&1; then
        exec {LOCK_FD}> "$SESSIONS_DIR/.session.lock"
        flock -x "$LOCK_FD"
    fi
}
new_session_temp() { TMP_FILE=$(mktemp "$SESSIONS_DIR/.session.XXXXXXXX"); }
commit_session_temp() {
    mv -f -- "$TMP_FILE" "$SESSION_FILE"
    TMP_FILE=""
}
update_session() {
    local filter="$1"
    lock_sessions
    if [ ! -f "$SESSION_FILE" ]; then return 0; fi
    new_session_temp
    printf '%s' "$INPUT" | jq --arg now "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        --slurpfile session "$SESSION_FILE" \
        '. as $event | $session[0] | '"$filter" > "$TMP_FILE"
    commit_session_temp
}
`

// Preserve continuity identity while removing the old report when new work starts.
const resumeTaskFilter = `del(.task_summary, .task_next_action, .task_decision) |
    if (.task_id // "") != "" then .task_state="working" else del(.task_state) end`

// Retain only the question text or tool name, never raw command/argument values.
const nativeQuestionFilter = `([$event.tool_input.questions[]? | (.question // .prompt // .title // empty)] | join("; ")) as $question |
    .attention_reason=(if $question != "" then $question else "Response required: " + ($event.tool_name // "agent question") end)`

func makeSessionHookScripts(harness string) map[string]string {
	// Set HARNESS before the shared error handler without changing the shebang.
	common := "#!/usr/bin/env bash\nHARNESS=" + harness + "\n" + sessionHookCommon[len("#!/usr/bin/env bash\n"):]
	scripts := map[string]string{
		"session-start.sh": common + `
CWD=$(printf '%s' "$INPUT" | jq -r '.cwd // empty')
TMUX_SESSION=""
if [ -n "${TMUX:-}" ]; then
    TMUX_SESSION=$(tmux display-message -p '#S' 2>/dev/null || echo "")
fi
AGENT_PID="$PPID"
CURRENT_PID=$PPID
while [ -n "$CURRENT_PID" ] && [ "$CURRENT_PID" != "1" ]; do
    PROC_NAME=$(ps -p "$CURRENT_PID" -o comm= 2>/dev/null || echo "")
    if [ "$PROC_NAME" = "$HARNESS" ]; then AGENT_PID="$CURRENT_PID"; break; fi
    CURRENT_PID=$(ps -p "$CURRENT_PID" -o ppid= 2>/dev/null | tr -d ' ' || echo "")
done
lock_sessions
new_session_temp
PREVIOUS_FILE="$SESSION_FILE"
if [ ! -f "$PREVIOUS_FILE" ]; then PREVIOUS_FILE=/dev/null; fi
jq -n --slurpfile previous "$PREVIOUS_FILE" --arg id "$SESSION_ID" --arg source "$HARNESS" \
    --arg cwd "$CWD" --arg tmux "$TMUX_SESSION" --arg now "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --argjson pid "$AGENT_PID" \
    '($previous[0] // {}) as $old | $old + {session_id:$id, source:$source, project_dir:$cwd, tmux_session_name:$tmux,
      started_at:($old.started_at // $now), last_activity:$now, pid:$pid, status:($old.status // "active")}' \
    > "$TMP_FILE"
commit_session_temp
`,
		"session-end.sh": common + `
lock_sessions
rm -f -- "$SESSION_FILE"
`,
		"prompt-submit.sh": common + `
update_session '.last_activity=$now | .status="working" | del(.attention_reason, .attention_kind, .attention_tool) | ` + resumeTaskFilter + `'
`,
		"pre-tool-use.sh": common + `
update_session '.last_activity=$now |
    if ($event.tool_name == "request_user_input_async" or $event.tool_name == "functions.request_user_input_async") then
        .status="waiting" | .attention_kind="async_question" | .attention_tool=$event.tool_name | ` + nativeQuestionFilter + `
    elif ($event.tool_name == "AskUserQuestion" or $event.tool_name == "ExitPlanMode" or
        $event.tool_name == "request_user_input" or $event.tool_name == "functions.request_user_input") then
        .status="waiting" | .attention_kind="question" | .attention_tool=$event.tool_name | ` + nativeQuestionFilter + `
    elif (.status == "waiting" or .status == "asking" or .task_state == "needs_decision") then .
    else ` + resumeTaskFilter + ` end'
`,
		"post-tool-use.sh": common + `
update_session '.last_activity=$now |
    if (.status == "waiting" or .status == "asking") then
        if .attention_kind == "async_question" or
            ((.attention_tool // "") != "" and .attention_tool != $event.tool_name) then .
        else .status="working" | del(.attention_reason, .attention_kind, .attention_tool) | ` + resumeTaskFilter + ` end
    else .status="working" | del(.attention_reason, .attention_kind, .attention_tool) end'
`,
		"permission-request.sh": common + `
update_session '.last_activity=$now | .status="waiting" | .attention_kind="permission" | .attention_tool=($event.tool_name // "") | ` + nativeQuestionFilter + `'
`,
		"stop.sh": common + `
update_session '.last_activity=$now | .last_stop=$now |
    if (.status == "waiting" or .status == "asking") then .
    else .status="stopped" |
        if .task_state == "working" and (.task_id // "") != "" then .task_state="resumable" else . end
    end'
`,
		"notification.sh": common + `
# Idle notifications are lifecycle noise, not evidence of a human decision.
TYPE=$(printf '%s' "$INPUT" | jq -r '.notification_type // empty')
case "$TYPE" in permission_prompt|elicitation_dialog) ;; *) exit 0 ;; esac
update_session '.last_activity=$now | .status="waiting" |
    if (.attention_reason // "") == "" then .attention_reason="Permission or user response required" | .attention_kind="permission" else . end'
`,
	}
	if harness == "codex" {
		prefixed := make(map[string]string, len(scripts)-1)
		for name, script := range scripts {
			if name != "notification.sh" {
				prefixed["codex-"+name] = script
			}
		}
		return prefixed
	}
	return scripts
}
