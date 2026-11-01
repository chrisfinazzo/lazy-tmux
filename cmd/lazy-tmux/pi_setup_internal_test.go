package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/integration/setup"
)

func TestPiAutomaticCleanInstallAndOptOut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(home, "custom-pi"))
	t.Setenv("LAZY_TMUX_CONFIG", filepath.Join(home, "config.toml"))
	var out bytes.Buffer
	var stdout bytes.Buffer
	if code := runCLI([]string{cmdSetup}, &stdout, &out); code != 0 {
		t.Fatalf("clean CLI setup failed: %d %s", code, &out)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if installed, err := setup.Check("pi", agentHome(cfg, "pi"), ""); err != nil || !installed {
		t.Fatalf("not installed: %v %v", installed, err)
	}
	if out.Len() == 0 {
		t.Fatal("missing pre-existing-client notice")
	}
	out.Reset()
	if err := preparePi(cmdDaemon, nil, &out); err != nil || out.Len() != 0 {
		t.Fatalf("not idempotent: %v %s", err, &out)
	}
	if code, _, errOut := run(t, "integrations", "setup", "pi", "--uninstall"); code != 0 {
		t.Fatal(errOut)
	}
	if err := preparePi(cmdBootstrap, nil, &out); err != nil {
		t.Fatal(err)
	}
	if installed, _ := setup.Check("pi", agentHome(cfg, "pi"), ""); installed {
		t.Fatal("automatic setup ignored uninstall")
	}
}

func TestPiReadOnlyAndDisabledDoNotInstall(t *testing.T) {
	home := t.TempDir()
	piHome := filepath.Join(home, "pi")
	t.Setenv("HOME", home)
	t.Setenv("PI_CODING_AGENT_DIR", piHome)
	conf := filepath.Join(home, "config.toml")
	t.Setenv("LAZY_TMUX_CONFIG", conf)
	var out bytes.Buffer
	for _, command := range []string{cmdVersion, "integrations", cmdHook, cmdList} {
		if err := preparePi(command, nil, &out); err != nil {
			t.Fatal(err)
		}
	}
	if err := preparePi(cmdSave, []string{"--help"}, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(piHome); !os.IsNotExist(err) {
		t.Fatal("read-only command wrote Pi home")
	}
	if err := os.WriteFile(conf, []byte("[integrations.pi]\nenabled=false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := preparePi(cmdBootstrap, nil, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(piHome); !os.IsNotExist(err) {
		t.Fatal("disabled integration wrote Pi home")
	}
}
