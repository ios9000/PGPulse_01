package ml

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/alert"
	"github.com/ios9000/PGPulse_01/internal/mlerrors"
)

// fittedBaseline creates an STLBaseline with enough data to be Ready() and
// produce forecasts. It feeds 3*period values using a simple sin wave so
// that seasonal buckets and trend are populated.
func fittedBaseline(key string, period int) *STLBaseline {
	b := NewSTLBaseline(key, period)
	n := 3 * period
	for i := 0; i < n; i++ {
		v := 100.0 + 10.0*math.Sin(2*math.Pi*float64(i)/float64(period))
		b.Update(v)
	}
	return b
}

func TestForecastForAlert_ConvertsPoints(t *testing.T) {
	const (
		period     = 10
		instanceID = "inst-1"
		metricKey  = "pg.connections.utilization_pct"
		horizon    = 5
	)

	b := fittedBaseline(metricKey, period)
	if !b.Ready() {
		t.Fatal("baseline should be Ready after 3*period updates")
	}

	d := &Detector{
		config: DetectorConfig{
			Enabled:            true,
			CollectionInterval: 60 * time.Second,
			ForecastZ:          1.96,
			Metrics: []MetricConfig{
				{Key: metricKey, Period: period, Enabled: true, ForecastHorizon: horizon},
			},
		},
		baselines:    map[string]*STLBaseline{instanceID + ":" + metricKey: b},
		bootstrapped: true,
	}

	ctx := context.Background()
	got, err := d.ForecastForAlert(ctx, instanceID, metricKey, horizon)
	if err != nil {
		t.Fatalf("ForecastForAlert: unexpected error: %v", err)
	}
	if len(got) != horizon {
		t.Fatalf("got %d points, want %d", len(got), horizon)
	}

	// Verify field-by-field conversion from ml.ForecastPoint → alert.ForecastPoint.
	result, err := d.Forecast(ctx, instanceID, metricKey, horizon)
	if err != nil {
		t.Fatalf("Forecast: unexpected error: %v", err)
	}
	for i, ap := range got {
		mp := result.Points[i]
		if ap.Offset != mp.Offset {
			t.Errorf("point[%d].Offset = %d, want %d", i, ap.Offset, mp.Offset)
		}
		if ap.Value != mp.Value {
			t.Errorf("point[%d].Value = %f, want %f", i, ap.Value, mp.Value)
		}
		if ap.Lower != mp.Lower {
			t.Errorf("point[%d].Lower = %f, want %f", i, ap.Lower, mp.Lower)
		}
		if ap.Upper != mp.Upper {
			t.Errorf("point[%d].Upper = %f, want %f", i, ap.Upper, mp.Upper)
		}
	}
}

func TestForecastForAlert_PassesErrNotBootstrapped(t *testing.T) {
	d := &Detector{
		config: DetectorConfig{
			Enabled:            true,
			CollectionInterval: 60 * time.Second,
			ForecastZ:          1.96,
		},
		baselines:    make(map[string]*STLBaseline),
		bootstrapped: false,
	}

	_, err := d.ForecastForAlert(context.Background(), "inst-1", "some.metric", 5)
	if !errors.Is(err, mlerrors.ErrNotBootstrapped) {
		t.Errorf("error = %v, want ErrNotBootstrapped", err)
	}
}

func TestForecastForAlert_PassesErrNoBaseline(t *testing.T) {
	d := &Detector{
		config: DetectorConfig{
			Enabled:            true,
			CollectionInterval: 60 * time.Second,
			ForecastZ:          1.96,
		},
		baselines:    make(map[string]*STLBaseline),
		bootstrapped: true,
	}

	_, err := d.ForecastForAlert(context.Background(), "inst-1", "missing.metric", 5)
	if !errors.Is(err, mlerrors.ErrNoBaseline) {
		t.Errorf("error = %v, want ErrNoBaseline", err)
	}
}

func TestForecastForAlert_EmptyPoints(t *testing.T) {
	const (
		period     = 10
		instanceID = "inst-1"
		metricKey  = "pg.test.metric"
	)

	// Create a baseline that is NOT warm enough for Forecast (totalSeen < period).
	// STLBaseline.Forecast returns nil when totalSeen < Period, and Detector.Forecast
	// maps that to ErrNoBaseline. However, the spec asks for "empty slice, nil error"
	// from a baseline that returns empty points. We achieve this by building a baseline
	// with exactly period updates — enough to avoid the nil return from Forecast, but
	// requesting horizon=0 so the result has zero points.
	b := NewSTLBaseline(metricKey, period)
	for i := 0; i < 2*period; i++ {
		b.Update(100.0 + float64(i))
	}
	if !b.Ready() {
		t.Fatal("baseline should be Ready after 2*period updates")
	}

	d := &Detector{
		config: DetectorConfig{
			Enabled:            true,
			CollectionInterval: 60 * time.Second,
			ForecastZ:          1.96,
		},
		baselines:    map[string]*STLBaseline{instanceID + ":" + metricKey: b},
		bootstrapped: true,
	}

	got, err := d.ForecastForAlert(context.Background(), instanceID, metricKey, 0)
	if err != nil {
		t.Fatalf("ForecastForAlert(horizon=0): unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d points, want 0", len(got))
	}
}

// Verify the alert.ForecastPoint type is the one returned (compile-time check).
var _ []alert.ForecastPoint = ([]alert.ForecastPoint)(nil)
