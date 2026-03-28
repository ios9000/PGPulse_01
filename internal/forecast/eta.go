package forecast

import (
	"log/slog"
	"time"

	"github.com/ios9000/PGPulse_01/internal/ml"
)

// ETACalculator computes real-time ETAs for in-progress maintenance operations.
// It is stateless — called per API request. The OperationTracker maintains the
// sample ring buffer; ETACalculator reads it.
type ETACalculator struct {
	tracker    *OperationTracker
	wmaCfg     ml.WMAConfig
	minSamples int
	logger     *slog.Logger
}

// NewETACalculator creates an ETA calculator wired to the given tracker.
func NewETACalculator(tracker *OperationTracker, cfg ETAConfig, logger *slog.Logger) *ETACalculator {
	minSamples := cfg.MinSamples
	if minSamples == 0 {
		minSamples = 4
	}
	return &ETACalculator{
		tracker: tracker,
		wmaCfg: ml.WMAConfig{
			WindowSize:  cfg.WindowSize,
			DecayFactor: cfg.DecayFactor,
		},
		minSamples: minSamples,
		logger:     logger,
	}
}

// ETAConfig holds the ETA-specific configuration values.
type ETAConfig struct {
	WindowSize  int
	DecayFactor float64
	MinSamples  int
}

// ComputeAll returns ETAs for all active operations on a given instance.
func (c *ETACalculator) ComputeAll(instanceID string) []OperationETA {
	ops := c.tracker.GetActiveOps(instanceID)
	now := time.Now()
	etas := make([]OperationETA, 0, len(ops))
	for i := range ops {
		etas = append(etas, c.computeOne(&ops[i], now))
	}
	return etas
}

// ComputeByPID returns the ETA for a specific operation identified by PID.
func (c *ETACalculator) ComputeByPID(instanceID string, pid int) *OperationETA {
	ops := c.tracker.GetActiveOps(instanceID)
	now := time.Now()
	for i := range ops {
		if ops[i].PID == pid {
			eta := c.computeOne(&ops[i], now)
			return &eta
		}
	}
	return nil
}

// computeOne builds an OperationETA from a TrackedOperation's sample buffer.
func (c *ETACalculator) computeOne(op *TrackedOperation, now time.Time) OperationETA {
	elapsed := now.Sub(op.StartedAt).Seconds()
	var pctDone float64
	if len(op.Samples) > 0 {
		pctDone = op.Samples[len(op.Samples)-1].PctDone
	}

	base := OperationETA{
		InstanceID:  op.InstanceID,
		PID:         op.PID,
		Operation:   op.Operation,
		Database:    op.Database,
		Table:       op.Table,
		StartedAt:   op.StartedAt,
		ElapsedSec:  elapsed,
		PercentDone: pctDone,
		SampleCount: len(op.Samples),
	}

	// Step 1: Minimum samples gate.
	if len(op.Samples) < c.minSamples {
		base.ETASec = -1
		base.Confidence = "estimating"
		return base
	}

	// Step 2: Extract timestamps and workDone values.
	timestamps := make([]time.Time, len(op.Samples))
	values := make([]float64, len(op.Samples))
	for i, s := range op.Samples {
		timestamps[i] = s.Timestamp
		values[i] = s.WorkDone
	}

	// Step 3: Compute WMA rate.
	wmaResult, err := ml.WeightedMovingAverage(c.wmaCfg, timestamps, values)
	if err != nil {
		base.ETASec = -1
		base.Confidence = "estimating"
		return base
	}

	// Step 4: Stall detection.
	if wmaResult.WeightedRate <= 0 {
		base.ETASec = -1
		base.Confidence = "stalled"
		base.RateCurrent = wmaResult.WeightedRate
		return base
	}

	// Step 5: Compute ETA from remaining work.
	lastSample := op.Samples[len(op.Samples)-1]
	remaining := lastSample.WorkTotal - lastSample.WorkDone
	if remaining <= 0 && pctDone < 100 {
		// WorkTotal not available — estimate from percentage.
		if pctDone > 0 {
			remaining = lastSample.WorkDone * (100.0 - pctDone) / pctDone
		}
	}

	etaSec := remaining / wmaResult.WeightedRate
	etaAt := now.Add(time.Duration(etaSec * float64(time.Second)))

	base.ETASec = etaSec
	base.ETAAt = etaAt
	base.RateCurrent = wmaResult.WeightedRate
	base.Confidence = classifyConfidence(wmaResult.SampleCount)
	return base
}

// classifyConfidence maps sample count to confidence level.
func classifyConfidence(sampleCount int) string {
	if sampleCount >= 8 {
		return "high"
	}
	return "medium"
}
