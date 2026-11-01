package main

import (
	"fmt"
	"io"
	"slices"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/integration/setup"
)

// Bootstrap precedes restored Pi launches. No changes on help, diagnostic, hook,
// or version commands. Existing clients are never reloaded or restarted here.
func preparePi(command string, args []string, stderr io.Writer) error {
	if !slices.Contains(
		[]string{
			cmdSetup,
			cmdBootstrap,
			cmdDaemon,
			cmdSave,
			cmdRestore,
			cmdWakeup,
			cmdSleep,
			cmdPicker,
		},
		command,
	) ||
		slices.Contains(args, "--help") ||
		slices.Contains(args, "-h") {
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if !cfg.Integrations.Enabled || !cfg.Integrations.Pi.Enabled {
		return nil
	}
	changed, err := setup.EnsurePi(agentHome(cfg, "pi"))
	if err != nil {
		return fmt.Errorf(
			"install Pi integration (disable with [integrations.pi] enabled=false): %w",
			err,
		)
	}
	if changed {
		_, _ = fmt.Fprintln(
			stderr,
			"lazy-tmux: installed Pi identity extension; new Pi clients load it automatically; "+
				"pre-existing clients need restart or /reload when idle",
		)
	}

	return nil
}
