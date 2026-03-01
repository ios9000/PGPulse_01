package storage

import (
	"context"
	"testing"
)

func TestNewPool_InvalidDSN(t *testing.T) {
	// "garbage://" is not a recognized PostgreSQL URL scheme; ParseConfig
	// returns an error immediately without any network activity.
	ctx := context.Background()
	_, err := NewPool(ctx, "garbage://definitely-not-valid")
	if err == nil {
		t.Error("NewPool() expected error for invalid DSN, got nil")
	}
}
