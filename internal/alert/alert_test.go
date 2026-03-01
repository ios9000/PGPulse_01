package alert

import "testing"

func TestOperatorCompare(t *testing.T) {
	tests := []struct {
		name      string
		op        Operator
		value     float64
		threshold float64
		want      bool
	}{
		// OpGreater
		{"gt_true", OpGreater, 10, 5, true},
		{"gt_false", OpGreater, 5, 10, false},
		{"gt_equal", OpGreater, 5, 5, false},
		// OpGreaterEqual
		{"gte_true", OpGreaterEqual, 10, 5, true},
		{"gte_equal", OpGreaterEqual, 5, 5, true},
		{"gte_false", OpGreaterEqual, 4, 5, false},
		// OpLess
		{"lt_true", OpLess, 3, 5, true},
		{"lt_equal", OpLess, 5, 5, false},
		{"lt_false", OpLess, 7, 5, false},
		// OpLessEqual
		{"lte_true", OpLessEqual, 3, 5, true},
		{"lte_equal", OpLessEqual, 5, 5, true},
		{"lte_false", OpLessEqual, 6, 5, false},
		// OpEqual
		{"eq_true", OpEqual, 5, 5, true},
		{"eq_false", OpEqual, 5, 6, false},
		// OpNotEqual
		{"neq_true", OpNotEqual, 5, 6, true},
		{"neq_false", OpNotEqual, 5, 5, false},
		// Edge cases
		{"gt_zero_values", OpGreater, 0, 0, false},
		{"lt_negative", OpLess, -10, -5, true},
		{"gt_negative", OpGreater, -5, -10, true},
		{"unknown_op", Operator("??"), 5, 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.op.Compare(tt.value, tt.threshold)
			if got != tt.want {
				t.Errorf("Operator(%q).Compare(%v, %v) = %v, want %v",
					tt.op, tt.value, tt.threshold, got, tt.want)
			}
		})
	}
}

func TestSeverityLevel(t *testing.T) {
	tests := []struct {
		severity Severity
		want     int
	}{
		{SeverityInfo, 1},
		{SeverityWarning, 2},
		{SeverityCritical, 3},
		{Severity("unknown"), 0},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			got := severityLevel(tt.severity)
			if got != tt.want {
				t.Errorf("severityLevel(%q) = %d, want %d", tt.severity, got, tt.want)
			}
		})
	}

	// Verify ordering
	if severityLevel(SeverityInfo) >= severityLevel(SeverityWarning) {
		t.Error("info should be less severe than warning")
	}
	if severityLevel(SeverityWarning) >= severityLevel(SeverityCritical) {
		t.Error("warning should be less severe than critical")
	}
}
