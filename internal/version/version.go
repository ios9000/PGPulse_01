package version

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// PGVersion represents a parsed PostgreSQL version.
type PGVersion struct {
	Major int    // e.g., 16
	Minor int    // e.g., 4
	Num   int    // e.g., 160004 (raw server_version_num)
	Full  string // e.g., "16.4 (Ubuntu 16.4-1.pgdg22.04+1)"
}

// String returns the version as "Major.Minor".
func (v PGVersion) String() string {
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

// AtLeast returns true if this version is >= the given major.minor.
func (v PGVersion) AtLeast(major, minor int) bool {
	if v.Major != major {
		return v.Major > major
	}
	return v.Minor >= minor
}

// Detect queries the PostgreSQL server for its version.
func Detect(ctx context.Context, conn *pgx.Conn) (PGVersion, error) {
	var numStr string
	var full string

	err := conn.QueryRow(ctx, "SHOW server_version_num").Scan(&numStr)
	if err != nil {
		return PGVersion{}, fmt.Errorf("detect PG version num: %w", err)
	}

	err = conn.QueryRow(ctx, "SHOW server_version").Scan(&full)
	if err != nil {
		return PGVersion{}, fmt.Errorf("detect PG version full: %w", err)
	}

	num, err := strconv.Atoi(numStr)
	if err != nil {
		return PGVersion{}, fmt.Errorf("parse server_version_num %q: %w", numStr, err)
	}

	return PGVersion{
		Major: num / 10000,
		Minor: num % 10000 / 100,
		Num:   num,
		Full:  full,
	}, nil
}
