package setup

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPiInstallUpgradeUninstall(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	foreign := filepath.Join(home, "extensions", "other.ts")
	if err := os.MkdirAll(filepath.Dir(foreign), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}
	for n := range 2 {
		changed, err := EnsurePi(home)
		if err != nil || changed != (n == 0) {
			t.Fatalf("ensure %d: %v %v", n, changed, err)
		}
	}
	if ok, err := Check("pi", home, ""); !ok || err != nil {
		t.Fatalf("check: %v %v", ok, err)
	}
	path := filepath.Join(home, "extensions", piExtensionName)
	old := []byte(piMarker + "// old release\n")
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	if changed, err := EnsurePi(home); err != nil || !changed {
		t.Fatalf("upgrade: %v %v", changed, err)
	}
	backup, err := os.ReadFile(path + ".lazy-tmux.bak")
	if err != nil || !bytes.Equal(backup, old) {
		t.Fatalf("backup: %q %v", backup, err)
	}
	if changed, err := Apply("pi", home, "", true); err != nil || !changed {
		t.Fatalf("remove: %v %v", changed, err)
	}
	if changed, err := EnsurePi(home); err != nil || changed {
		t.Fatalf("sticky opt-out: %v %v", changed, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("reinstalled after opt-out")
	}
	if _, err := Apply("pi", home, "", false); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "unrelated" {
		t.Fatal("modified foreign extension")
	}
}

func TestPiRefusesForeignAndSymlinkFiles(t *testing.T) {
	t.Parallel()
	for _, symlink := range []bool{false, true} {
		home := t.TempDir()
		dir := filepath.Join(home, "extensions")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, piExtensionName)
		if symlink {
			target := filepath.Join(home, "target")
			if err := os.WriteFile(target, piExtension, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.WriteFile(path, []byte("user extension"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := EnsurePi(home); err == nil {
			t.Fatal("overwrote unowned file")
		}
	}
}

func TestPiSupportsSymlinkExtensionDirectory(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), t.TempDir()
	if err := os.Symlink(dir, filepath.Join(home, "extensions")); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsurePi(home); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, piExtensionName)); err != nil {
		t.Fatal(err)
	}
}

func TestPiExtensionLifecycleStatus(t *testing.T) {
	t.Parallel()
	if os.Getenv("ENABLE_REAL_PI_TESTS") != "true" {
		t.Skip("requires the real-Pi integration environment")
	}
	packageDir := os.Getenv("PI_TEST_PACKAGE_DIR")
	if packageDir == "" {
		packageDir = "/usr/local/lib/node_modules/@mariozechner/pi-coding-agent"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "node", "testdata/pi-status.cjs", packageDir).
		CombinedOutput()
	if err != nil {
		t.Fatalf("Pi lifecycle harness: %v\n%s", err, output)
	}
}
