package playbook

import (
	"testing"
)

func TestInterpret(t *testing.T) {
	tests := []struct {
		name        string
		spec        InterpretationSpec
		columns     []string
		rows        [][]any
		rowCount    int
		wantVerdict string
		wantMsg     string
	}{
		{
			name: "row count rule fires",
			spec: InterpretationSpec{
				RowCountRules: []RowCountRule{
					{Operator: ">", Value: 100, Verdict: "red", Message: "Too many rows: {{row_count}}"},
				},
				DefaultVerdict: "green",
				DefaultMessage: "OK",
			},
			columns:     []string{"x"},
			rows:        [][]any{{1}},
			rowCount:    200,
			wantVerdict: "red",
			wantMsg:     "Too many rows: 200",
		},
		{
			name: "column rule fires",
			spec: InterpretationSpec{
				Rules: []InterpretationRule{
					{Column: "failed_count", Operator: ">", Value: 0, Verdict: "red",
						Message: "{{failed_count}} failures detected"},
				},
				DefaultVerdict: "green",
				DefaultMessage: "OK",
			},
			columns:     []string{"failed_count"},
			rows:        [][]any{{int64(5)}},
			rowCount:    1,
			wantVerdict: "red",
			wantMsg:     "5 failures detected",
		},
		{
			name: "default verdict",
			spec: InterpretationSpec{
				Rules: []InterpretationRule{
					{Column: "val", Operator: ">", Value: 100, Verdict: "red", Message: "high"},
				},
				DefaultVerdict: "yellow",
				DefaultMessage: "needs review",
			},
			columns:     []string{"val"},
			rows:        [][]any{{int64(50)}},
			rowCount:    1,
			wantVerdict: "yellow",
			wantMsg:     "needs review",
		},
		{
			name: "empty rows uses default",
			spec: InterpretationSpec{
				DefaultVerdict: "green",
				DefaultMessage: "no data",
			},
			columns:     nil,
			rows:        nil,
			rowCount:    0,
			wantVerdict: "green",
			wantMsg:     "no data",
		},
		{
			name: "is_null operator",
			spec: InterpretationSpec{
				Rules: []InterpretationRule{
					{Column: "val", Operator: "is_null", Verdict: "yellow", Message: "null detected"},
				},
				DefaultVerdict: "green",
				DefaultMessage: "OK",
			},
			columns:     []string{"val"},
			rows:        [][]any{{nil}},
			rowCount:    1,
			wantVerdict: "yellow",
			wantMsg:     "null detected",
		},
		{
			name: "== operator with zero",
			spec: InterpretationSpec{
				Rules: []InterpretationRule{
					{Column: "count", Operator: "==", Value: 0, Verdict: "green", Message: "all clear"},
				},
				DefaultVerdict: "yellow",
				DefaultMessage: "needs review",
			},
			columns:     []string{"count"},
			rows:        [][]any{{int64(0)}},
			rowCount:    1,
			wantVerdict: "green",
			wantMsg:     "all clear",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict, msg := Interpret(tt.spec, tt.columns, tt.rows, tt.rowCount)
			if verdict != tt.wantVerdict {
				t.Errorf("verdict: got %q, want %q", verdict, tt.wantVerdict)
			}
			if msg != tt.wantMsg {
				t.Errorf("message: got %q, want %q", msg, tt.wantMsg)
			}
		})
	}
}

func TestEvaluateBranch(t *testing.T) {
	step := Step{
		BranchRules: []BranchRule{
			{
				Condition: BranchCondition{Column: "failed_count", Operator: ">", Value: 0},
				GotoStep:  3,
				Reason:    "failures detected",
			},
			{
				Condition: BranchCondition{Verdict: "green"},
				GotoStep:  5,
				Reason:    "healthy",
			},
		},
		NextStepDefault: intPtr(2),
	}

	t.Run("column condition match", func(t *testing.T) {
		next := EvaluateBranch(step, "red", []string{"failed_count"}, [][]any{{int64(5)}})
		if next != 3 {
			t.Errorf("got %d, want 3", next)
		}
	})

	t.Run("verdict condition match", func(t *testing.T) {
		next := EvaluateBranch(step, "green", []string{"failed_count"}, [][]any{{int64(0)}})
		if next != 5 {
			t.Errorf("got %d, want 5", next)
		}
	})

	t.Run("no match uses default", func(t *testing.T) {
		next := EvaluateBranch(step, "yellow", []string{"failed_count"}, [][]any{{int64(0)}})
		if next != 2 {
			t.Errorf("got %d, want 2", next)
		}
	})
}
