package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSavePatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path, err := SavePatch("task_123", "diff --git a/a b/a\n")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".toolnet", "patches", "task_123.patch")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestSavePatchRejectsUnsafeTaskID(t *testing.T) {
	if _, err := SavePatch("../escape", "patch"); err == nil {
		t.Fatal("expected unsafe task id to fail")
	}
}
