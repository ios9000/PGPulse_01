//go:build integration

package collector_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// testDB wraps a running PostgreSQL test container.
// Use newTestDB to create one; call connect() to open additional connections.
type testDB struct {
	container *postgres.PostgresContainer
	connStr   string
	t         *testing.T
}

// newTestDB starts a PostgreSQL container for testing.
// The container is terminated automatically when the test ends.
// Additional ContainerCustomizer options are merged after the defaults.
func newTestDB(t *testing.T, pgVersion string, opts ...testcontainers.ContainerCustomizer) *testDB {
	t.Helper()
	ctx := context.Background()

	baseOpts := []testcontainers.ContainerCustomizer{
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
	}
	baseOpts = append(baseOpts, opts...)

	container, err := postgres.Run(ctx, "docker.io/postgres:"+pgVersion, baseOpts...)
	if err != nil {
		t.Fatalf("failed to start postgres:%s container: %v", pgVersion, err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate postgres container: %v", err)
		}
	})

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	return &testDB{container: container, connStr: connStr, t: t}
}

// connect opens a new pgx connection to the test database.
// The connection is closed automatically when the test ends.
func (d *testDB) connect() *pgx.Conn {
	d.t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, d.connStr)
	if err != nil {
		d.t.Fatalf("failed to connect to test postgres: %v", err)
	}
	d.t.Cleanup(func() { conn.Close(ctx) })
	return conn
}

// setupPG starts a PostgreSQL container and returns a single connected pgx.Conn.
func setupPG(t *testing.T, pgVersion string) *pgx.Conn {
	t.Helper()
	return newTestDB(t, pgVersion).connect()
}

// setupPGWithStatements starts PostgreSQL with pg_stat_statements preloaded.
// PostgreSQL's docker-entrypoint.sh treats args starting with '-' as postgres flags,
// so Cmd:[]string{"-c", "shared_preload_libraries=pg_stat_statements"} correctly
// starts the server with that configuration.
func setupPGWithStatements(t *testing.T, pgVersion string) *pgx.Conn {
	t.Helper()
	db := newTestDB(t, pgVersion,
		testcontainers.CustomizeRequest(testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Cmd: []string{"-c", "shared_preload_libraries=pg_stat_statements"},
			},
		}),
	)
	conn := db.connect()
	ctx := context.Background()
	if _, err := conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pg_stat_statements"); err != nil {
		t.Fatalf("failed to create pg_stat_statements extension: %v", err)
	}
	return conn
}

// metricNames extracts the Metric field from each MetricPoint.
func metricNames(points []collector.MetricPoint) []string {
	names := make([]string, len(points))
	for i, p := range points {
		names[i] = p.Metric
	}
	return names
}

// findMetric returns the first MetricPoint with the given name, or nil.
func findMetric(points []collector.MetricPoint, name string) *collector.MetricPoint {
	for i := range points {
		if points[i].Metric == name {
			return &points[i]
		}
	}
	return nil
}

// findMetricWithLabel returns the first MetricPoint matching name and label key=value, or nil.
func findMetricWithLabel(points []collector.MetricPoint, name, labelKey, labelValue string) *collector.MetricPoint {
	for i := range points {
		if points[i].Metric == name && points[i].Labels[labelKey] == labelValue {
			return &points[i]
		}
	}
	return nil
}
