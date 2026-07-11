package workflow

import "testing"

func TestParseQAResponse_Pass(t *testing.T) {
	raw := "STATUS: PASS\nSEVERITY: None\nFINDINGS: Không có vấn đề"
	got := parseQAResponse(raw)
	if got.Status != "PASS" {
		t.Errorf("Status = %q, want PASS", got.Status)
	}
	if got.Severity != "" {
		t.Errorf("Severity = %q, want empty on PASS", got.Severity)
	}
	if got.Findings != "Không có vấn đề" {
		t.Errorf("Findings = %q", got.Findings)
	}
}

func TestParseQAResponse_FailBlocking(t *testing.T) {
	raw := "STATUS: FAIL\nSEVERITY: critical\nFINDINGS: null pointer dereference"
	got := parseQAResponse(raw)
	if !got.IsFailing() {
		t.Error("expected failing")
	}
	if !got.IsBlockingSeverity() {
		t.Error("expected blocking severity")
	}
	if got.Severity != "Critical" {
		t.Errorf("Severity = %q, want Critical", got.Severity)
	}
}

func TestParseQAResponse_SeverityNormalization(t *testing.T) {
	cases := map[string]string{
		"HIGH":     "High",
		"medium":   "Medium",
		"Moderate": "Medium",
		"low":      "Low",
		"none":     "None",
	}
	for in, want := range cases {
		got := parseQAResponse("STATUS: FAIL\nSEVERITY: " + in + "\nFINDINGS: x")
		if got.Severity != want {
			t.Errorf("SEVERITY %q -> %q, want %q", in, got.Severity, want)
		}
	}
}

func TestParseQAResponse_MalformedDefaultsToFailCritical(t *testing.T) {
	got := parseQAResponse("here is some free text without the expected format")
	if !got.IsFailing() {
		t.Error("malformed response must default to FAIL")
	}
	if !got.IsBlockingSeverity() {
		t.Error("malformed response must default to Critical")
	}
}

func TestParseQAResponse_FindingsStopsAtBlankLine(t *testing.T) {
	raw := "STATUS: FAIL\nSEVERITY: High\nFINDINGS: missing error handling\n\nBTW you could also refactor this later"
	got := parseQAResponse(raw)
	want := "missing error handling"
	if got.Findings != want {
		t.Errorf("Findings = %q, want %q", got.Findings, want)
	}
}
