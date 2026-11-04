package mlerrors

import (
	"errors"
	"fmt"
	"testing"
)

// customErr is a typed error that wraps a sentinel, used to verify that
// errors.As finds the wrapper while errors.Is still finds the sentinel.
type customErr struct {
	Metric string
	Err    error
}

func (e *customErr) Error() string { return "metric " + e.Metric + ": " + e.Err.Error() }
func (e *customErr) Unwrap() error { return e.Err }

func TestSentinels_AreDistinctAndNonNil(t *testing.T) {
	if ErrNotBootstrapped == nil || ErrNoBaseline == nil {
		t.Fatal("sentinel errors must be non-nil")
	}
	if errors.Is(ErrNotBootstrapped, ErrNoBaseline) || errors.Is(ErrNoBaseline, ErrNotBootstrapped) {
		t.Fatal("ErrNotBootstrapped and ErrNoBaseline must not match each other")
	}
	// Messages are surfaced to API callers / logs; changing them is a visible change.
	if got, want := ErrNotBootstrapped.Error(), "ml detector not yet bootstrapped"; got != want {
		t.Errorf("ErrNotBootstrapped.Error() = %q, want %q", got, want)
	}
	if got, want := ErrNoBaseline.Error(), "no fitted baseline for this metric"; got != want {
		t.Errorf("ErrNoBaseline.Error() = %q, want %q", got, want)
	}
}

func TestSentinels_WrappingPreservesIdentity(t *testing.T) {
	sentinels := []struct {
		name  string
		err   error
		other error
	}{
		{"ErrNotBootstrapped", ErrNotBootstrapped, ErrNoBaseline},
		{"ErrNoBaseline", ErrNoBaseline, ErrNotBootstrapped},
	}

	wrappers := []struct {
		name string
		wrap func(error) error
	}{
		{"bare", func(e error) error { return e }},
		{"fmt %w once", func(e error) error { return fmt.Errorf("forecast: %w", e) }},
		{"fmt %w twice", func(e error) error {
			return fmt.Errorf("alert evaluator: %w", fmt.Errorf("ml detector: %w", e))
		}},
		{"errors.Join", func(e error) error { return errors.Join(errors.New("unrelated"), e) }},
		{"multi %w", func(e error) error { return fmt.Errorf("a: %w; b: %w", errors.New("x"), e) }},
		{"typed wrapper", func(e error) error { return &customErr{Metric: "pg.connections.active", Err: e} }},
		{"fmt over typed", func(e error) error {
			return fmt.Errorf("outer: %w", &customErr{Metric: "m", Err: e})
		}},
	}

	for _, s := range sentinels {
		for _, w := range wrappers {
			t.Run(s.name+"/"+w.name, func(t *testing.T) {
				wrapped := w.wrap(s.err)
				if !errors.Is(wrapped, s.err) {
					t.Errorf("errors.Is(%v, %s) = false, want true", wrapped, s.name)
				}
				if errors.Is(wrapped, s.other) {
					t.Errorf("errors.Is(%v, other sentinel) = true, want false", wrapped)
				}
			})
		}
	}
}

func TestSentinels_NonWrappingLosesIdentity(t *testing.T) {
	// %v (not %w) and a fresh errors.New with the same text must NOT match:
	// callers rely on identity, not on message equality.
	cases := []struct {
		name string
		err  error
	}{
		{"fmt %v", fmt.Errorf("forecast: %v", ErrNotBootstrapped)},
		{"same message", errors.New(ErrNotBootstrapped.Error())},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if errors.Is(tc.err, ErrNotBootstrapped) {
				t.Errorf("errors.Is(%v, ErrNotBootstrapped) = true, want false", tc.err)
			}
		})
	}
}

func TestSentinels_ErrorsAsRoundTrip(t *testing.T) {
	for _, sentinel := range []error{ErrNotBootstrapped, ErrNoBaseline} {
		t.Run(sentinel.Error(), func(t *testing.T) {
			wrapped := fmt.Errorf("evaluate: %w", &customErr{Metric: "pg.cache.hit_ratio", Err: sentinel})

			var ce *customErr
			if !errors.As(wrapped, &ce) {
				t.Fatalf("errors.As(%v, *customErr) = false, want true", wrapped)
			}
			if ce.Metric != "pg.cache.hit_ratio" {
				t.Errorf("ce.Metric = %q, want %q", ce.Metric, "pg.cache.hit_ratio")
			}
			if ce.Err != sentinel { //nolint:errorlint // identity check is the point of this test
				t.Errorf("ce.Err = %v, want the sentinel itself", ce.Err)
			}
			// The unwrapped chain from the As target still reaches the sentinel.
			if !errors.Is(ce, sentinel) {
				t.Errorf("errors.Is(ce, sentinel) = false, want true")
			}
		})
	}
}

func TestSentinels_ErrorsAsNoFalsePositive(t *testing.T) {
	// A sentinel that was never wrapped in customErr must not satisfy errors.As.
	var ce *customErr
	if errors.As(fmt.Errorf("x: %w", ErrNoBaseline), &ce) {
		t.Fatalf("errors.As unexpectedly matched *customErr")
	}
}
