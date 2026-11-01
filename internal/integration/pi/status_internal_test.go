package pi

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func statusFixture(t *testing.T) (*Integration, snapshot.Pane, snapshot.Pane) {
	t.Helper()
	root := t.TempDir()
	file := writeSession(t, root, "session-a", root)
	saved := snapshot.Pane{
		CurrentCmd: "node", RestoreCmd: "pi", CurrentPath: root,
		Meta: map[string]string{"pi.session_file": file, "pi.session_id": "session-a"},
	}
	live := snapshot.Pane{
		CurrentPath: root, LiveStatusID: "session-a", LiveStatus: "working",
		Meta: map[string]string{"pi.session_file": file},
	}

	return New(root, root), saved, live
}

func TestPiLiveStatuses(t *testing.T) {
	t.Parallel()
	for state, want := range map[string]integration.Status{
		"working": integration.StatusWorking, "idle": integration.StatusIdle,
		"awaiting_input":    integration.StatusAwaitingInput,
		"awaiting_decision": integration.StatusAwaitingDecision, "error": integration.StatusError,
	} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			client, saved, live := statusFixture(t)
			live.LiveStatus = state
			client.WithStatusLoader(
				func() ([]snapshot.Pane, error) { return []snapshot.Pane{live}, nil },
			)
			got, ok := client.Status(saved)
			if !ok || got != want {
				t.Fatalf("got %v %v, want %v", got, ok, want)
			}
		})
	}
}

func TestPiRejectsUnverifiedAndStaleStatuses(t *testing.T) {
	t.Parallel()
	for _, test := range []string{
		"missing owner", "different ID", "different file", "pending", "invalid state",
		"legacy extension", "missing file", "wrong cwd", "duplicate clients", "no session", "offline status",
	} {
		t.Run(test, func(t *testing.T) {
			t.Parallel()
			client, saved, live := statusFixture(t)
			panes := make([]snapshot.Pane, 1, 2)
			panes[0] = live
			switch test {
			case "missing owner":
				panes = nil
			case "different ID":
				panes[0].LiveStatusID = "session-b"
			case "different file":
				panes[0].Meta["pi.session_file"] = "/different/history.jsonl"
			case "pending":
				saved.Meta["agent.restore_pending"] = "1"
			case "invalid state":
				panes[0].LiveStatus = "invented"
			case "legacy extension":
				panes[0].LiveStatusID = ""
				panes[0].LiveStatus = ""
			case "missing file":
				saved.Meta["pi.session_file"] = "/absent/history.jsonl"
				panes[0].Meta["pi.session_file"] = "/absent/history.jsonl"
			case "wrong cwd":
				panes[0].CurrentPath = "/another/project"
			case "duplicate clients":
				panes = append(panes, live)
			case "no session":
				saved.ProcessArgv = []string{"pi", "--no-session"}
			case "offline status":
				saved.LiveStatus = "working"
				saved.LiveStatusID = "session-a"
				saved.Meta["pi.status"] = "working"
				panes = nil
			}
			client.WithStatusLoader(func() ([]snapshot.Pane, error) { return panes, nil })
			if got, ok := client.Status(saved); ok || got != integration.StatusUnknown {
				t.Fatalf("accepted %s: %v %v", test, got, ok)
			}
		})
	}
}

func TestPiStatusRefreshScopeAndFailure(t *testing.T) {
	t.Parallel()
	client, saved, live := statusFixture(t)
	calls := 0
	client.WithStatusLoader(
		func() ([]snapshot.Pane, error) {
			calls++

			return []snapshot.Pane{live}, nil
		},
	)
	scoped := statusScope(t, client)
	if status, ok := scoped.Status(saved); !ok || status != integration.StatusWorking {
		t.Fatal("initial state not working")
	}
	live.LiveStatus = "awaiting_input"
	if status, _ := scoped.Status(saved); status != integration.StatusWorking {
		t.Fatal("scope did not keep a consistent snapshot")
	}
	if calls != 1 {
		t.Fatalf("queried tmux %d times per scope", calls)
	}
	fresh := statusScope(t, client)
	if status, _ := fresh.Status(saved); status != integration.StatusAwaitingInput {
		t.Fatal("next refresh did not see live change")
	}
	// An unscoped caller must never freeze its first status indefinitely.
	if status, _ := client.Status(saved); status != integration.StatusAwaitingInput {
		t.Fatal("unscoped refresh failed")
	}
	live.LiveStatus = "working"
	if status, _ := client.Status(saved); status != integration.StatusWorking {
		t.Fatal("unscoped status cached forever")
	}
	client.WithStatusLoader(
		func() ([]snapshot.Pane, error) { return nil, io.EOF },
	)
	if status, ok := statusScope(t, client).Status(
		saved,
	); ok ||
		status != integration.StatusUnknown {
		t.Fatal("stopped tmux retained working status")
	}
}

func TestPiRuntimeStatusIsNotSerialized(t *testing.T) {
	t.Parallel()
	_, _, live := statusFixture(t)
	data, err := json.Marshal(live)
	if err != nil {
		t.Fatal(err)
	}
	var restored snapshot.Pane
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.LiveStatus != "" || restored.LiveStatusID != "" {
		t.Fatal("runtime status leaked into saved snapshot")
	}
}

func statusScope(t *testing.T, client *Integration) *Integration {
	t.Helper()
	reporter, ok := client.Scope().(*Integration)
	if !ok {
		t.Fatal("scoped integration lost StatusReporter")
	}

	return reporter
}
