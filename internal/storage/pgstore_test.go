package storage

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// storageDiscardLogger returns a logger that drops all output.
func storageDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- buildQuery tests (no DB required) ---

func TestBuildQuery_Empty(t *testing.T) {
	sql, args := buildQuery(collector.MetricQuery{})
	want := "SELECT time, instance_id, metric, value, labels FROM metrics ORDER BY time DESC"
	if sql != want {
		t.Errorf("sql =\n  %q\nwant\n  %q", sql, want)
	}
	if len(args) != 0 {
		t.Errorf("args len = %d, want 0: %v", len(args), args)
	}
}

func TestBuildQuery_InstanceOnly(t *testing.T) {
	sql, args := buildQuery(collector.MetricQuery{InstanceID: "prod-primary"})
	if !strings.Contains(sql, "instance_id = $1") {
		t.Errorf("sql %q missing instance_id = $1", sql)
	}
	if len(args) != 1 || args[0] != "prod-primary" {
		t.Errorf("args = %v, want [prod-primary]", args)
	}
}

func TestBuildQuery_MetricPrefix(t *testing.T) {
	sql, args := buildQuery(collector.MetricQuery{Metric: "pgpulse.connections"})
	if !strings.Contains(sql, "metric LIKE $1") {
		t.Errorf("sql %q missing metric LIKE $1", sql)
	}
	if len(args) != 1 || args[0] != "pgpulse.connections%" {
		t.Errorf("args = %v, want [pgpulse.connections%%]", args)
	}
}

func TestBuildQuery_TimeRange(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	sql, args := buildQuery(collector.MetricQuery{Start: start, End: end})
	if !strings.Contains(sql, "time >= $1") {
		t.Errorf("sql %q missing time >= $1", sql)
	}
	if !strings.Contains(sql, "time <= $2") {
		t.Errorf("sql %q missing time <= $2", sql)
	}
	if len(args) != 2 {
		t.Errorf("args len = %d, want 2: %v", len(args), args)
	}
}

func TestBuildQuery_WithLabels(t *testing.T) {
	sql, args := buildQuery(collector.MetricQuery{Labels: map[string]string{"db_name": "mydb"}})
	if !strings.Contains(sql, "labels @>") {
		t.Errorf("sql %q missing labels @> filter", sql)
	}
	if len(args) != 1 {
		t.Fatalf("args len = %d, want 1", len(args))
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(args[0].(string)), &got); err != nil {
		t.Fatalf("labels arg not valid JSON: %v", err)
	}
	if got["db_name"] != "mydb" {
		t.Errorf("labels arg db_name = %q, want %q", got["db_name"], "mydb")
	}
}

func TestBuildQuery_WithLimit(t *testing.T) {
	sql, args := buildQuery(collector.MetricQuery{Limit: 100})
	if !strings.Contains(sql, "LIMIT $1") {
		t.Errorf("sql %q missing LIMIT $1", sql)
	}
	if len(args) != 1 || args[0] != 100 {
		t.Errorf("args = %v, want [100]", args)
	}
}

func TestBuildQuery_AllFilters(t *testing.T) {
	q := collector.MetricQuery{
		InstanceID: "prod",
		Metric:     "pgpulse.connections",
		Start:      time.Now().Add(-1 * time.Hour),
		End:        time.Now(),
		Labels:     map[string]string{"state": "active"},
		Limit:      50,
	}
	_, args := buildQuery(q)
	// instance_id($1) + metric($2) + start($3) + end($4) + labels($5) + limit($6)
	if len(args) != 6 {
		t.Errorf("args len = %d, want 6", len(args))
	}
}

// --- PGStore unit tests (no DB required) ---

func TestPGStore_Write_EmptySlice(t *testing.T) {
	// Empty input is a no-op; pool is never accessed so nil is safe here.
	store := &PGStore{pool: nil, logger: storageDiscardLogger()}

	if err := store.Write(context.Background(), nil); err != nil {
		t.Errorf("Write(nil) error = %v, want nil", err)
	}
	if err := store.Write(context.Background(), []collector.MetricPoint{}); err != nil {
		t.Errorf("Write([]) error = %v, want nil", err)
	}
}

func TestPGStore_Write_NilLabels(t *testing.T) {
	// Verify that nil Labels are converted to {} before marshaling.
	p := collector.MetricPoint{
		InstanceID: "test",
		Metric:     "pgpulse.test",
		Value:      1.0,
		Labels:     nil,
	}
	if p.Labels == nil {
		p.Labels = map[string]string{}
	}
	b, err := json.Marshal(p.Labels)
	if err != nil {
		t.Fatalf("marshal nil labels: %v", err)
	}
	if string(b) != "{}" {
		t.Errorf("nil labels marshaled as %q, want {}", b)
	}
}
