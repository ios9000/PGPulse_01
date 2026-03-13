package alert

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/mlerrors"
)

// --- Mock implementations for forecast tests ---

type mockForecastProvider struct {
	mu     sync.Mutex
	points []ForecastPoint
	err    error
	calls  int
}

func (m *mockForecastProvider) ForecastForAlert(_ context.Context, _, _ string, _ int) ([]ForecastPoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return m.points, m.err
}

type mockForecastRuleStore struct {
	rules []Rule
}

func (m *mockForecastRuleStore) List(_ context.Context) ([]Rule, error) {
	return m.rules, nil
}

func (m *mockForecastRuleStore) ListEnabled(_ context.Context) ([]Rule, error) {
	var enabled []Rule
	for _, r := range m.rules {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	return enabled, nil
}

func (m *mockForecastRuleStore) Get(_ context.Context, id string) (*Rule, error) {
	for i := range m.rules {
		if m.rules[i].ID == id {
			return &m.rules[i], nil
		}
	}
	return nil, ErrRuleNotFound
}

func (m *mockForecastRuleStore) Create(_ context.Context, rule *Rule) error {
	m.rules = append(m.rules, *rule)
	return nil
}

func (m *mockForecastRuleStore) Update(_ context.Context, rule *Rule) error {
	for i := range m.rules {
		if m.rules[i].ID == rule.ID {
			m.rules[i] = *rule
			return nil
		}
	}
	return ErrRuleNotFound
}

func (m *mockForecastRuleStore) Delete(_ context.Context, id string) error {
	for i := range m.rules {
		if m.rules[i].ID == id {
			m.rules = append(m.rules[:i], m.rules[i+1:]...)
			return nil
		}
	}
	return ErrRuleNotFound
}

func (m *mockForecastRuleStore) UpsertBuiltin(_ context.Context, rule *Rule) error {
	for i := range m.rules {
		if m.rules[i].ID == rule.ID {
			m.rules[i] = *rule
			return nil
		}
	}
	m.rules = append(m.rules, *rule)
	return nil
}

type mockForecastHistoryStore struct {
	mu       sync.Mutex
	recorded []*AlertEvent
}

func (m *mockForecastHistoryStore) Record(_ context.Context, event *AlertEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recorded = append(m.recorded, event)
	return nil
}

func (m *mockForecastHistoryStore) Resolve(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}

func (m *mockForecastHistoryStore) ListUnresolved(_ context.Context) ([]AlertEvent, error) {
	return nil, nil
}

func (m *mockForecastHistoryStore) Query(_ context.Context, _ AlertHistoryQuery) ([]AlertEvent, error) {
	return nil, nil
}

func (m *mockForecastHistoryStore) Cleanup(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

func (m *mockForecastHistoryStore) recordedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.recorded)
}

// baseForecastRule returns a forecast_threshold rule with sensible defaults.
func baseForecastRule() Rule {
	return Rule{
		ID:                        "fc_test",
		Name:                      "Forecast Test Rule",
		Metric:                    "pg.connections.utilization_pct",
		Operator:                  OpGreater,
		Threshold:                 90,
		Severity:                  SeverityWarning,
		Type:                      RuleTypeForecastThreshold,
		ConsecutivePointsRequired: 3,
		CooldownMinutes:           15,
		Enabled:                   true,
		Source:                    SourceCustom,
	}
}

// dummyPoint returns a MetricPoint that triggers runForecastAlerts for the
// given instanceID. The metric matches the forecast rule so uniqueInstances
// picks it up, but Evaluate's standard threshold path ignores it because
// the rule type is forecast_threshold (no matching standard rule).
func dummyPoint(instanceID string) collector.MetricPoint {
	return collector.MetricPoint{
		InstanceID: instanceID,
		Metric:     "pg.connections.utilization_pct",
		Value:      50, // below threshold; only forecast path matters
		Timestamp:  time.Now(),
	}
}

func TestForecastAlerts(t *testing.T) {
	tests := []struct {
		name           string
		providerNil    bool
		providerPoints []ForecastPoint
		providerErr    error
		rule           Rule
		evalCalls      int // how many times to call Evaluate
		wantRecorded   int // expected number of recorded alert events
	}{
		{
			name:        "NilProvider",
			providerNil: true,
			rule:        baseForecastRule(),
			evalCalls:   1,
			wantRecorded: 0,
		},
		{
			name:        "NotBootstrapped",
			providerErr: mlerrors.ErrNotBootstrapped,
			rule:        baseForecastRule(),
			evalCalls:   1,
			wantRecorded: 0,
		},
		{
			name:        "NoBaseline",
			providerErr: mlerrors.ErrNoBaseline,
			rule:        baseForecastRule(),
			evalCalls:   1,
			wantRecorded: 0,
		},
		{
			name: "InsufficientCrossings",
			providerPoints: []ForecastPoint{
				{Offset: 1, Value: 95, Lower: 93, Upper: 97},  // crosses
				{Offset: 2, Value: 95, Lower: 93, Upper: 97},  // crosses
				{Offset: 3, Value: 80, Lower: 78, Upper: 82},  // does not cross
				{Offset: 4, Value: 80, Lower: 78, Upper: 82},  // does not cross
				{Offset: 5, Value: 95, Lower: 93, Upper: 97},  // crosses
				{Offset: 6, Value: 80, Lower: 78, Upper: 82},  // does not cross
				{Offset: 7, Value: 80, Lower: 78, Upper: 82},  // does not cross
			},
			rule:         baseForecastRule(),
			evalCalls:    1,
			wantRecorded: 0,
		},
		{
			name: "ExactlyRequired",
			providerPoints: []ForecastPoint{
				{Offset: 1, Value: 95, Lower: 93, Upper: 97},
				{Offset: 2, Value: 95, Lower: 93, Upper: 97},
				{Offset: 3, Value: 95, Lower: 93, Upper: 97},
				{Offset: 4, Value: 80, Lower: 78, Upper: 82},
				{Offset: 5, Value: 80, Lower: 78, Upper: 82},
				{Offset: 6, Value: 80, Lower: 78, Upper: 82},
				{Offset: 7, Value: 80, Lower: 78, Upper: 82},
			},
			rule:         baseForecastRule(),
			evalCalls:    1,
			wantRecorded: 1,
		},
		{
			name: "CooldownRespected",
			providerPoints: []ForecastPoint{
				{Offset: 1, Value: 95, Lower: 93, Upper: 97},
				{Offset: 2, Value: 95, Lower: 93, Upper: 97},
				{Offset: 3, Value: 95, Lower: 93, Upper: 97},
				{Offset: 4, Value: 95, Lower: 93, Upper: 97},
				{Offset: 5, Value: 95, Lower: 93, Upper: 97},
				{Offset: 6, Value: 95, Lower: 93, Upper: 97},
				{Offset: 7, Value: 95, Lower: 93, Upper: 97},
			},
			rule:         baseForecastRule(),
			evalCalls:    2, // second call within cooldown
			wantRecorded: 1, // only the first fires
		},
		{
			name: "UseLowerBound_True",
			providerPoints: []ForecastPoint{
				// Value is below threshold, but Lower is above threshold
				{Offset: 1, Value: 85, Lower: 91, Upper: 99},
				{Offset: 2, Value: 85, Lower: 91, Upper: 99},
				{Offset: 3, Value: 85, Lower: 91, Upper: 99},
				{Offset: 4, Value: 85, Lower: 91, Upper: 99},
				{Offset: 5, Value: 85, Lower: 91, Upper: 99},
				{Offset: 6, Value: 85, Lower: 91, Upper: 99},
				{Offset: 7, Value: 85, Lower: 91, Upper: 99},
			},
			rule: func() Rule {
				r := baseForecastRule()
				r.ID = "fc_lower_true"
				r.UseLowerBound = true
				return r
			}(),
			evalCalls:    1,
			wantRecorded: 1,
		},
		{
			name: "UseLowerBound_False",
			providerPoints: []ForecastPoint{
				// Value is below threshold, Lower is above threshold
				{Offset: 1, Value: 85, Lower: 91, Upper: 99},
				{Offset: 2, Value: 85, Lower: 91, Upper: 99},
				{Offset: 3, Value: 85, Lower: 91, Upper: 99},
				{Offset: 4, Value: 85, Lower: 91, Upper: 99},
				{Offset: 5, Value: 85, Lower: 91, Upper: 99},
				{Offset: 6, Value: 85, Lower: 91, Upper: 99},
				{Offset: 7, Value: 85, Lower: 91, Upper: 99},
			},
			rule: func() Rule {
				r := baseForecastRule()
				r.ID = "fc_lower_false"
				r.UseLowerBound = false // default: compare Value
				return r
			}(),
			evalCalls:    1,
			wantRecorded: 0, // Value 85 < 90, no crossing
		},
		{
			name: "CounterResetOnNonCrossing",
			providerPoints: []ForecastPoint{
				{Offset: 1, Value: 95, Lower: 93, Upper: 97},  // cross 1
				{Offset: 2, Value: 95, Lower: 93, Upper: 97},  // cross 2
				{Offset: 3, Value: 80, Lower: 78, Upper: 82},  // reset
				{Offset: 4, Value: 95, Lower: 93, Upper: 97},  // cross 1
				{Offset: 5, Value: 95, Lower: 93, Upper: 97},  // cross 2
				{Offset: 6, Value: 95, Lower: 93, Upper: 97},  // cross 3 → fires
				{Offset: 7, Value: 95, Lower: 93, Upper: 97},
			},
			rule:         baseForecastRule(),
			evalCalls:    1,
			wantRecorded: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ruleStore := &mockForecastRuleStore{rules: []Rule{tt.rule}}
			histStore := &mockForecastHistoryStore{}
			logger := slog.Default()

			eval := NewEvaluator(ruleStore, histStore, logger)
			if err := eval.LoadRules(context.Background()); err != nil {
				t.Fatalf("LoadRules: %v", err)
			}

			if tt.providerNil {
				eval.SetForecastProvider(nil, 3)
			} else {
				provider := &mockForecastProvider{
					points: tt.providerPoints,
					err:    tt.providerErr,
				}
				eval.SetForecastProvider(provider, 3)
			}

			ctx := context.Background()
			instanceID := "inst-1"
			points := []collector.MetricPoint{dummyPoint(instanceID)}

			for i := 0; i < tt.evalCalls; i++ {
				if _, err := eval.Evaluate(ctx, points); err != nil {
					t.Fatalf("Evaluate call %d: %v", i+1, err)
				}
			}

			got := histStore.recordedCount()
			if got != tt.wantRecorded {
				t.Errorf("recorded events = %d, want %d", got, tt.wantRecorded)
			}
		})
	}
}
