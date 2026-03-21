//go:build integration

package rca

import (
	"testing"
)

func TestPGIncidentStore_CreateGet(t *testing.T) {
	t.Skip("integration test: requires PostgreSQL with rca_incidents table")
}

func TestPGIncidentStore_ListByInstance(t *testing.T) {
	t.Skip("integration test: requires PostgreSQL with rca_incidents table")
}

func TestPGIncidentStore_Cleanup(t *testing.T) {
	t.Skip("integration test: requires PostgreSQL with rca_incidents table")
}
