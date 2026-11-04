package version

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestDetectLive runs Detect against real servers listed in
// PGPULSE_VERSION_TEST_DSNS (space-separated postgres:// URLs). It is skipped
// when the variable is unset; the conformance workflow sets it.
//
// It pins Detect's current behaviour. Note that Minor is derived with the
// pre-PG10 encoding (num % 10000 / 100), so for PG >= 10 it is 0 for any
// minor release below 100. See docs/conformance/README.md.
func TestDetectLive(t *testing.T) {
	dsns := strings.Fields(os.Getenv("PGPULSE_VERSION_TEST_DSNS"))
	if len(dsns) == 0 {
		t.Skip("PGPULSE_VERSION_TEST_DSNS not set")
	}
	for _, dsn := range dsns {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		conn, err := pgx.Connect(ctx, dsn)
		if err != nil {
			cancel()
			t.Fatalf("connect: %v", err)
		}
		v, err := Detect(ctx, conn)
		if err != nil {
			t.Fatalf("Detect: %v", err)
		}
		var numStr string
		if err := conn.QueryRow(ctx, "SHOW server_version_num").Scan(&numStr); err != nil {
			t.Fatalf("server_version_num: %v", err)
		}
		_ = conn.Close(ctx)
		cancel()

		num, _ := strconv.Atoi(numStr)
		if v.Num != num {
			t.Errorf("Num = %d, want %d", v.Num, num)
		}
		if v.Major != num/10000 {
			t.Errorf("Major = %d, want %d", v.Major, num/10000)
		}
		if v.Minor != num%10000/100 {
			t.Errorf("Minor = %d, want %d (current encoding)", v.Minor, num%10000/100)
		}
		if !strings.HasPrefix(v.Full, strconv.Itoa(v.Major)) {
			t.Errorf("Full = %q does not start with major %d", v.Full, v.Major)
		}
		t.Logf("%s: Major=%d Minor=%d Num=%d (actual minor release %d)", v.Full, v.Major, v.Minor, v.Num, num%10000)
	}
}
