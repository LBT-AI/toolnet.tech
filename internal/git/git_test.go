package git

import "testing"

func TestExtractUnifiedDiffFromMarkdown(t *testing.T) {
	raw := "Here is the patch:\n```diff\ndiff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n```\nDone"
	got := extractUnifiedDiff(raw)
	want := "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestExtractUnifiedDiffRejectsProse(t *testing.T) {
	if got := extractUnifiedDiff("I changed the file."); got != "" {
		t.Fatalf("expected empty patch, got %q", got)
	}
}
