package metrics

import (
	"encoding/json"
	"testing"
	"time"
)

func TestExplicitTaskStateSurvivesIdleAndNativeRequestWins(t *testing.T) {
	for state, want := range map[string]SessionStatus{
		"working": StatusWorking, "waiting_external": StatusWaitingExternal,
		"needs_decision": StatusAsking, "complete": StatusComplete,
		"resumable": StatusResumable, "paused": StatusPaused, "": StatusReady,
		"unrecognized": StatusReady,
	} {
		t.Run(state, func(t *testing.T) {
			collector := newTestHookSessionCollector(t, t.TempDir())
			hs := HookSession{SessionID: "task", Status: "stopped", TaskState: state,
				TaskID: "bead-1", TaskDecision: "Which region?", LastActivity: time.Now().Add(-24 * time.Hour)}
			writeHookSessionFixture(t, collector, hs)
			sessions, err := collector.CollectSessions()
			if err != nil || len(sessions) != 1 {
				t.Fatalf("collect: %v %+v", err, sessions)
			}
			got := sessions[0].ToTmuxSession()
			if got.Status != want || got.TaskID != "bead-1" {
				t.Fatalf("got %+v, want %s", got, want)
			}
			if state == "needs_decision" && got.AttentionReason != "Which region?" {
				t.Fatalf("decision lost: %+v", got)
			}
			hs.Status = "waiting"
			hs.AttentionReason = "Approve tool: Bash"
			if hs.EffectiveStatus() != StatusAsking || hs.HumanAttentionReason() != hs.AttentionReason {
				t.Fatalf("native request hidden: %+v", hs)
			}
		})
	}
}

func TestTaskHookLifecycle(t *testing.T) {
	for _, harness := range []string{"claude", "codex"} {
		t.Run(harness, func(t *testing.T) {
			home := t.TempDir()
			collector := newTestHookSessionCollector(t, home)
			prefix := ""
			if harness == "codex" {
				prefix = "codex-"
				if err := newCodexHookInstaller(home).InstallHooks(); err != nil {
					t.Fatal(err)
				}
			}
			event := `{"session_id":"task","cwd":"/tmp/project"}`
			run := func(name, input string) { runHarnessHook(t, harness, home, prefix+name+".sh", input) }
			read := func() HookSession {
				ss, err := collector.CollectSessions()
				if err != nil || len(ss) != 1 {
					t.Fatalf("collect: %v %+v", err, ss)
				}
				return ss[0]
			}
			run("session-start", event)
			for _, state := range []string{"waiting_external", "needs_decision", "complete", "resumable", "paused"} {
				hs := read()
				hs.Status = "working"
				hs.TaskID = "bead-1"
				hs.TaskState = state
				hs.TaskSummary = "CI pending"
				hs.TaskDecision = "Which region?"
				hs.TaskNextAction = "Inspect result"
				writeHookSessionFixture(t, collector, hs)
				run("post-tool-use", event)
				run("stop", event)
				got := read()
				if got.TaskState != state || got.TaskDecision != hs.TaskDecision {
					t.Fatalf("report did not survive tool/stop: %+v", got)
				}
				run("session-start", event)
				if read().TaskState != state {
					t.Fatal("SessionStart/compaction erased task report")
				}
				if harness == "claude" {
					run("notification", `{"session_id":"task","notification_type":"idle_prompt"}`)
					if read().TaskState != state || read().Status != "stopped" {
						t.Fatal("idle notification changed outcome")
					}
				}

				if state == "needs_decision" {
					run("pre-tool-use", event)
					run("post-tool-use", event)
					run("stop", event)
					if read().EffectiveStatus() != StatusAsking {
						t.Fatal("independent work erased explicit decision")
					}
				}
				run("prompt-submit", event)
				got = read()
				if got.TaskID != "bead-1" || got.TaskState != "working" || got.TaskDecision != "" || got.TaskSummary != "" || got.TaskNextAction != "" {
					t.Fatalf("stale report after prompt: %+v", got)
				}
			}
			hs := read()
			hs.TaskState = "complete"
			writeHookSessionFixture(t, collector, hs)
			run("pre-tool-use", event)
			if read().TaskState != "working" {
				t.Fatal("tool activity retained completion")
			}
			question := `{"session_id":"task","tool_name":"AskUserQuestion","tool_input":{"questions":[{"question":"Which region?"}]}}`
			if harness == "codex" {
				question = `{"session_id":"task","tool_name":"functions.request_user_input","tool_input":{"questions":[{"question":"Which region?"}]}}`
			}
			run("pre-tool-use", question)
			run("stop", event)
			got := read()
			if got.EffectiveStatus() != StatusAsking || got.HumanAttentionReason() != "Which region?" {
				t.Fatalf("native question lost: %+v", got)
			}
			run("post-tool-use", question)
			if read().EffectiveStatus() != StatusWorking || read().AttentionReason != "" {
				t.Fatal("answered question remained in attention")
			}
			run("permission-request", `{"session_id":"task","tool_name":"Bash","tool_input":{"command":"SECRET-DO-NOT-COPY"}}`)
			got = read()
			if got.AttentionReason != "Response required: Bash" {
				t.Fatalf("unexpected permission summary %q", got.AttentionReason)
			}
		})
	}
}

func TestTaskMetadataJSONIsAdditive(t *testing.T) {
	var hs HookSession
	if err := json.Unmarshal([]byte(`{"session_id":"old","status":"stopped"}`), &hs); err != nil {
		t.Fatal(err)
	}
	got := hs.ToTmuxSession()
	if got.Status != StatusReady || got.TaskState != "" || got.AttentionReason != "" {
		t.Fatalf("legacy idle inferred outcome: %+v", got)
	}
}

func TestNativeAsyncDecisionSurvivesIndependentWork(t *testing.T) {
	for _, harness := range []string{"claude", "codex"} {
		t.Run(harness, func(t *testing.T) {
			home := t.TempDir()
			collector := newTestHookSessionCollector(t, home)
			prefix := ""
			if harness == "codex" {
				prefix = "codex-"
				if err := newCodexHookInstaller(home).InstallHooks(); err != nil {
					t.Fatal(err)
				}
			}
			run := func(name, input string) { runHarnessHook(t, harness, home, prefix+name+".sh", input) }
			read := func() HookSession {
				ss, err := collector.CollectSessions()
				if err != nil || len(ss) != 1 {
					t.Fatalf("collect: %v %+v", err, ss)
				}
				return ss[0]
			}
			event := `{"session_id":"async","cwd":"/tmp/project"}`
			run("session-start", event)
			hs := read()
			hs.TaskID = "bead-1"
			hs.TaskState = "working"
			writeHookSessionFixture(t, collector, hs)
			question := `{"session_id":"async","tool_name":"functions.request_user_input_async","tool_input":{"questions":[{"title":"Which region?"}]}}`
			run("pre-tool-use", question)
			run("post-tool-use", question)
			independent := `{"session_id":"async","tool_name":"Bash"}`
			run("pre-tool-use", independent)
			run("post-tool-use", independent)
			run("stop", event)
			hs = read()
			if hs.EffectiveStatus() != StatusAsking || hs.HumanAttentionReason() != "Which region?" {
				t.Fatalf("async request disappeared: %+v", hs)
			}
			run("prompt-submit", event)
			if read().EffectiveStatus() != StatusWorking {
				t.Fatal("actual user input failed to clear native question")
			}
			run("stop", event)
			if read().EffectiveStatus() != StatusResumable {
				t.Fatal("unfinished enrolled task should be resumable after Stop")
			}
			permission := `{"session_id":"async","tool_name":"Bash"}`
			run("permission-request", permission)
			run("post-tool-use", `{"session_id":"async","tool_name":"Read"}`)
			if read().EffectiveStatus() != StatusAsking {
				t.Fatal("unrelated completed tool cleared permission")
			}
			run("post-tool-use", permission)
			if read().EffectiveStatus() != StatusWorking {
				t.Fatal("approved tool failed to clear permission")
			}
		})
	}
}
