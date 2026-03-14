package alert

import (
	"strings"
	"testing"
	"time"
)

func newTestEvent() AlertEvent {
	return AlertEvent{
		RuleID:     "rule-conn-count",
		RuleName:   "High Connection Count",
		InstanceID: "prod-db-01",
		Severity:   SeverityWarning,
		Value:      95.50,
		Threshold:  80.00,
		Operator:   OpGreater,
		Metric:     "pg_connections_active",
		FiredAt:    time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestFormatSubject_Fire(t *testing.T) {
	tests := []struct {
		name     string
		severity Severity
		want     string
	}{
		{"warning", SeverityWarning, "[WARNING] High Connection Count \u2014 prod-db-01"},
		{"critical", SeverityCritical, "[CRITICAL] High Connection Count \u2014 prod-db-01"},
		{"info", SeverityInfo, "[INFO] High Connection Count \u2014 prod-db-01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := newTestEvent()
			ev.Severity = tt.severity
			got := FormatSubject(ev)
			if got != tt.want {
				t.Errorf("FormatSubject() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatSubject_Resolution(t *testing.T) {
	ev := newTestEvent()
	ev.IsResolution = true
	resolvedAt := ev.FiredAt.Add(5 * time.Minute)
	ev.ResolvedAt = &resolvedAt

	got := FormatSubject(ev)
	want := "[RESOLVED] High Connection Count \u2014 prod-db-01"
	if got != want {
		t.Errorf("FormatSubject() = %q, want %q", got, want)
	}
}

func TestRenderHTMLTemplate_Fire(t *testing.T) {
	ev := newTestEvent()
	html, err := RenderHTMLTemplate(ev, "", nil)
	if err != nil {
		t.Fatalf("RenderHTMLTemplate() error = %v", err)
	}

	checks := []struct {
		label string
		want  string
	}{
		{"rule name", "High Connection Count"},
		{"instance", "prod-db-01"},
		{"metric", "pg_connections_active"},
		{"value formatted", "95.50"},
		{"threshold formatted", "80.00"},
		{"severity badge", "warning"},
		{"fired-at timestamp", "2026-03-01T12:00:00Z"},
		{"engine footer", "PGPulse Alert Engine"},
		{"severity color", severityColor(SeverityWarning)},
	}

	for _, c := range checks {
		if !strings.Contains(html, c.want) {
			t.Errorf("HTML missing %s: want substring %q", c.label, c.want)
		}
	}
}

func TestRenderHTMLTemplate_Resolution(t *testing.T) {
	ev := newTestEvent()
	ev.IsResolution = true
	resolvedAt := ev.FiredAt.Add(10 * time.Minute)
	ev.ResolvedAt = &resolvedAt

	html, err := RenderHTMLTemplate(ev, "", nil)
	if err != nil {
		t.Fatalf("RenderHTMLTemplate() error = %v", err)
	}

	checks := []struct {
		label string
		want  string
	}{
		{"resolved header", "Resolved:"},
		{"green color", "#16a34a"},
		{"resolved-at time", resolvedAt.Format(time.RFC3339)},
		{"duration", "10m0s"},
		{"checkmark emoji", "\xe2\x9c\x85"},
	}

	for _, c := range checks {
		if !strings.Contains(html, c.want) {
			t.Errorf("Resolution HTML missing %s: want substring %q", c.label, c.want)
		}
	}
}

func TestRenderTextTemplate_Fire(t *testing.T) {
	ev := newTestEvent()
	text, err := RenderTextTemplate(ev, "", nil)
	if err != nil {
		t.Fatalf("RenderTextTemplate() error = %v", err)
	}

	checks := []struct {
		label string
		want  string
	}{
		{"severity uppercase", "[WARNING]"},
		{"rule name", "High Connection Count"},
		{"instance", "prod-db-01"},
		{"metric", "pg_connections_active"},
		{"value formatted", "95.50"},
		{"threshold", "80.00"},
		{"fired-at", "2026-03-01T12:00:00Z"},
		{"engine footer", "PGPulse Alert Engine"},
	}

	for _, c := range checks {
		if !strings.Contains(text, c.want) {
			t.Errorf("Text missing %s: want substring %q", c.label, c.want)
		}
	}
}

func TestRenderTextTemplate_Resolution(t *testing.T) {
	ev := newTestEvent()
	ev.IsResolution = true
	resolvedAt := ev.FiredAt.Add(5 * time.Minute)
	ev.ResolvedAt = &resolvedAt

	text, err := RenderTextTemplate(ev, "", nil)
	if err != nil {
		t.Fatalf("RenderTextTemplate() error = %v", err)
	}

	checks := []struct {
		label string
		want  string
	}{
		{"resolved tag", "[RESOLVED]"},
		{"resolved-at", resolvedAt.Format(time.RFC3339)},
		{"duration", "5m0s"},
	}

	for _, c := range checks {
		if !strings.Contains(text, c.want) {
			t.Errorf("Resolution text missing %s: want substring %q", c.label, c.want)
		}
	}
}

func TestRenderHTMLTemplate_WithLabels(t *testing.T) {
	ev := newTestEvent()
	ev.Labels = map[string]string{
		"database": "mydb",
		"app":      "pgpulse",
	}

	html, err := RenderHTMLTemplate(ev, "", nil)
	if err != nil {
		t.Fatalf("RenderHTMLTemplate() error = %v", err)
	}

	// Labels should appear sorted by key: app before database.
	appIdx := strings.Index(html, "app")
	dbIdx := strings.Index(html, "database")
	if appIdx < 0 || dbIdx < 0 {
		t.Fatalf("HTML missing label keys: app=%d, database=%d", appIdx, dbIdx)
	}
	if appIdx >= dbIdx {
		t.Errorf("labels not sorted by key: app at %d, database at %d", appIdx, dbIdx)
	}

	if !strings.Contains(html, "pgpulse") {
		t.Error("HTML missing label value 'pgpulse'")
	}
	if !strings.Contains(html, "mydb") {
		t.Error("HTML missing label value 'mydb'")
	}
}

func TestRenderHTMLTemplate_WithDashboardURL(t *testing.T) {
	ev := newTestEvent()
	html, err := RenderHTMLTemplate(ev, "https://monitor.example.com", nil)
	if err != nil {
		t.Fatalf("RenderHTMLTemplate() error = %v", err)
	}

	wantLink := "https://monitor.example.com/instances/prod-db-01"
	if !strings.Contains(html, wantLink) {
		t.Errorf("HTML missing dashboard link %q", wantLink)
	}
	if !strings.Contains(html, "View in Dashboard") {
		t.Error("HTML missing 'View in Dashboard' text")
	}
}

func TestRenderHTMLTemplate_NoDashboardURL(t *testing.T) {
	ev := newTestEvent()
	html, err := RenderHTMLTemplate(ev, "", nil)
	if err != nil {
		t.Fatalf("RenderHTMLTemplate() error = %v", err)
	}

	if strings.Contains(html, "View in Dashboard") {
		t.Error("HTML should not contain dashboard link when dashboardURL is empty")
	}
}
