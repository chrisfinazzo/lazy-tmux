package tmux

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func (processes processSnapshot) attachPiBinding(pane snapshot.Pane, encoded string) snapshot.Pane {
	var binding struct {
		PID       int    `json:"pid"`
		File      string `json:"file"`
		Version   int    `json:"version"`
		StartedAt string `json:"started_at"`
		SessionID string `json:"session_id"`
		State     string `json:"state"`
	}
	if json.Unmarshal([]byte(encoded), &binding) != nil || binding.Version != 1 ||
		binding.StartedAt == "" ||
		(binding.File != "" && !filepath.IsAbs(binding.File)) {
		return pane
	}
	candidates := processes.descendants(pane.ProcessPID)
	if root, ok := processes.processes[pane.ProcessPID]; ok {
		candidates = append(candidates, root)
	}
	for _, process := range candidates {
		if process.pid != binding.PID || !piForegroundOwner(process) {
			continue
		}
		argv, err := processArgv(process.pid)
		if err != nil || agent.LaunchKind(argv) != "pi" ||
			processStartTime(binding.PID) != binding.StartedAt {
			return pane
		}
		if pane.Meta == nil {
			pane.Meta = make(map[string]string)
		}
		delete(pane.Meta, agent.PendingKey)
		delete(pane.Meta, "pi.session_id")
		if binding.File == "" {
			delete(pane.Meta, "pi.session_file")
			pane.ProcessArgv = []string{"pi", "--no-session"}
			pane.RestoreCmd = "pi --no-session"

			return pane
		}
		pane.Meta["pi.session_file"] = binding.File
		pane.LiveStatus = binding.State
		pane.LiveStatusID = binding.SessionID
		pane.ProcessArgv = []string{"pi", "--session", binding.File}
		pane.RestoreCmd = "pi"

		return pane
	}

	return pane
}

func piForegroundOwner(process psProcess) bool {
	return strings.Contains(process.stat, "+") && !strings.ContainsAny(process.stat, "TZ")
}

const piStatusFields = 4

// CapturePiPanes takes one process snapshot per picker refresh, independent of
// stored snapshots and autosave cadence. attachPiBinding performs owner checks.
func (client *Client) CapturePiPanes() ([]snapshot.Pane, error) {
	output, err := client.Output("list-panes", "-a", "-F",
		"#{pane_pid}|#{pane_current_command}|#{pane_current_path}|#{@lazy_tmux_pi_binding}")
	if err != nil {
		return nil, err
	}
	processes, err := loadProcessSnapshot()
	if err != nil {
		return nil, err
	}
	var panes []snapshot.Pane
	for _, line := range splitLines(output) {
		fields := splitFieldsN(line, piStatusFields)
		if len(fields) != piStatusFields || fields[3] == "" {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		pane := processes.attachPiBinding(snapshot.Pane{
			ProcessPID:   pid,
			CurrentCmd:   fields[1],
			CurrentPath:  fields[2],
			Index:        0,
			RestoreCmd:   "",
			Scrollback:   nil,
			IsActive:     false,
			ProcessArgv:  nil,
			LiveStatus:   "",
			LiveStatusID: "",
			Meta:         nil,
			Agent:        nil,
		}, fields[3])
		if pane.Meta["pi.session_file"] != "" {
			panes = append(panes, pane)
		}
	}

	return panes, nil
}
