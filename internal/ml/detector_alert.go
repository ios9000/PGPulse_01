package ml

import (
	"context"

	"github.com/ios9000/PGPulse_01/internal/alert"
)

// ForecastForAlert satisfies alert.ForecastProvider.
// It converts ml.ForecastResult.Points to []alert.ForecastPoint.
// ErrNotBootstrapped and ErrNoBaseline are passed through unchanged.
func (d *Detector) ForecastForAlert(
	ctx context.Context,
	instanceID, metricKey string,
	horizon int,
) ([]alert.ForecastPoint, error) {
	result, err := d.Forecast(ctx, instanceID, metricKey, horizon)
	if err != nil {
		return nil, err
	}
	out := make([]alert.ForecastPoint, len(result.Points))
	for i, p := range result.Points {
		out[i] = alert.ForecastPoint{
			Offset: p.Offset,
			Value:  p.Value,
			Lower:  p.Lower,
			Upper:  p.Upper,
		}
	}
	return out, nil
}
