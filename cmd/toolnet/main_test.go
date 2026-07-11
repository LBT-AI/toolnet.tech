package main

import (
	"os"
	"strings"
	"testing"
)

func captureStdout(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	f()
	w.Close()
	os.Stdout = old
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	return string(buf[:n])
}

func TestPrintChatRendersBox(t *testing.T) {
	for _, step := range []string{"COO_ANALYZE", "PM_AUDIT", "DEV_IMPLEMENT", "QA_VERIFY"} {
		out := captureStdout(func() { printChat(step, "hello world this is a chat bubble") })
		if !strings.Contains(out, "┌─ "+step) {
			t.Errorf("printChat(%q) missing title bar, got:\n%s", step, out)
		}
		if !strings.Contains(out, "│ ") {
			t.Errorf("printChat(%q) missing box body border", step)
		}
		if !strings.Contains(out, "└") {
			t.Errorf("printChat(%q) missing bottom border", step)
		}
		if !strings.Contains(out, "hello world") {
			t.Errorf("printChat(%q) missing wrapped body", step)
		}
	}
}

func TestWrapLines(t *testing.T) {
	lines := wrapLines("the quick brown fox jumps over the lazy dog", 10)
	for _, l := range lines {
		if len(l) > 10 {
			t.Errorf("wrapLines produced line longer than width: %q", l)
		}
	}
	if len(lines) < 5 {
		t.Errorf("expected word wrapping into multiple lines, got %d: %v", len(lines), lines)
	}
}
