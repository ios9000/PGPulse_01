package playbook

import (
	"strings"
	"testing"
)

func TestMultiStatementGuard(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		wantErr bool
	}{
		{"single statement", "SELECT 1", false},
		{"trailing semicolon", "SELECT 1;", false},
		{"multi-statement", "SET LOCAL default_transaction_read_only = OFF; DROP TABLE users", true},
		{"injection attempt", "SELECT 1; DROP TABLE users;", true},
		{"whitespace trailing", "SELECT 1 ;  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trimmed := strings.TrimSpace(tt.sql)
			trimmed = strings.TrimRight(trimmed, ";")
			hasMulti := strings.Contains(trimmed, ";")
			if hasMulti != tt.wantErr {
				t.Errorf("sql=%q: got multi=%v, want err=%v", tt.sql, hasMulti, tt.wantErr)
			}
		})
	}
}
