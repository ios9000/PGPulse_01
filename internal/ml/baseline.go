package ml

import (
	"sort"
	"time"

	"gonum.org/v1/gonum/stat"
)

// STLBaseline implements a simplified STL decomposition for online anomaly detection.
// Trend: EWMA. Seasonal: period-folded mean. Residual: actual - trend - seasonal.
type STLBaseline struct {
	MetricKey  string
	Period     int
	windowSize int
	ring       []float64
	rHead      int
	rCount     int
	residuals  []float64
	resHead    int
	resCount   int
	ewma       float64
	ewmaAlpha  float64
	seasonal   []float64
	seasonN    []int
	totalSeen  int
	sumAll     float64
}

// NewSTLBaseline creates a baseline tracker for the given metric and seasonal period.
func NewSTLBaseline(key string, period int) *STLBaseline {
	size := max(3*period, 1000)
	alpha := 2.0 / float64(size+1)
	return &STLBaseline{
		MetricKey:  key,
		Period:     period,
		windowSize: size,
		ring:       make([]float64, size),
		residuals:  make([]float64, size),
		ewmaAlpha:  alpha,
		seasonal:   make([]float64, period),
		seasonN:    make([]int, period),
	}
}

// Update adds a new observation to the baseline.
func (b *STLBaseline) Update(value float64) {
	if b.totalSeen == 0 {
		b.ewma = value
	} else {
		b.ewma = b.ewmaAlpha*value + (1-b.ewmaAlpha)*b.ewma
	}

	bucket := b.totalSeen % b.Period
	n := float64(b.seasonN[bucket] + 1)
	b.seasonal[bucket] = (b.seasonal[bucket]*(n-1) + value) / n
	b.seasonN[bucket]++

	b.sumAll += value
	b.totalSeen++

	overallMean := b.sumAll / float64(b.totalSeen)
	seasonal := b.seasonal[bucket] - overallMean
	residual := value - b.ewma - seasonal

	b.residuals[b.resHead] = residual
	b.resHead = (b.resHead + 1) % b.windowSize
	if b.resCount < b.windowSize {
		b.resCount++
	}

	b.ring[b.rHead] = value
	b.rHead = (b.rHead + 1) % b.windowSize
	if b.rCount < b.windowSize {
		b.rCount++
	}
}

// Score returns the Z-score and IQR outlier status for a value against the baseline.
// Returns (0, false) if the baseline is not yet ready (fewer than Period*2 observations).
func (b *STLBaseline) Score(value float64) (zScore float64, isIQR bool) {
	if b.resCount < b.Period*2 {
		return 0, false
	}

	bucket := b.totalSeen % b.Period
	overallMean := b.sumAll / float64(b.totalSeen)
	seasonal := b.seasonal[bucket] - overallMean
	residual := value - b.ewma - seasonal

	resSlice := b.residualSlice()
	mean := stat.Mean(resSlice, nil)
	stddev := stat.StdDev(resSlice, nil)
	if stddev > 1e-10 {
		zScore = (residual - mean) / stddev
	}

	sorted := make([]float64, len(resSlice))
	copy(sorted, resSlice)
	sort.Float64s(sorted)
	q1 := stat.Quantile(0.25, stat.Empirical, sorted, nil)
	q3 := stat.Quantile(0.75, stat.Empirical, sorted, nil)
	iqr := q3 - q1
	isIQR = residual < q1-1.5*iqr || residual > q3+1.5*iqr

	return zScore, isIQR
}

// ResidualStddev returns the standard deviation of stored residuals.
func (b *STLBaseline) ResidualStddev() float64 {
	if b.resCount < 2 {
		return 0
	}
	return stat.StdDev(b.residualSlice(), nil)
}

// Ready returns true if the baseline has enough observations to produce scores.
func (b *STLBaseline) Ready() bool {
	return b.resCount >= b.Period*2
}

// residualSlice returns the current residuals as a contiguous slice.
func (b *STLBaseline) residualSlice() []float64 {
	if b.resCount < b.windowSize {
		return b.residuals[:b.resCount]
	}
	// Full ring buffer: return copy in order
	result := make([]float64, b.windowSize)
	copy(result, b.residuals[b.resHead:])
	copy(result[b.windowSize-b.resHead:], b.residuals[:b.resHead])
	return result
}

// BaselineSnapshot holds the serializable state of an STLBaseline.
type BaselineSnapshot struct {
	InstanceID string    `json:"instance_id"`
	MetricKey  string    `json:"metric_key"`
	Period     int       `json:"period"`
	WindowSize int       `json:"window_size"`
	EWMA       float64   `json:"ewma"`
	EWMAAlpha  float64   `json:"ewma_alpha"`
	Seasonal   []float64 `json:"seasonal"`
	SeasonN    []int     `json:"season_n"`
	Residuals  []float64 `json:"residuals"`
	ResCount   int       `json:"res_count"`
	TotalSeen  int       `json:"total_seen"`
	SumAll     float64   `json:"sum_all"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Snapshot exports the baseline state for persistence.
func (b *STLBaseline) Snapshot(instanceID string) BaselineSnapshot {
	// Export only live residuals from ring buffer in chronological order
	liveResiduals := make([]float64, b.resCount)
	for i := 0; i < b.resCount; i++ {
		idx := (b.resHead - b.resCount + i + b.windowSize) % b.windowSize
		liveResiduals[i] = b.residuals[idx]
	}
	return BaselineSnapshot{
		InstanceID: instanceID,
		MetricKey:  b.MetricKey,
		Period:     b.Period,
		WindowSize: b.windowSize,
		EWMA:       b.ewma,
		EWMAAlpha:  b.ewmaAlpha,
		Seasonal:   append([]float64{}, b.seasonal...),
		SeasonN:    append([]int{}, b.seasonN...),
		Residuals:  liveResiduals,
		ResCount:   b.resCount,
		TotalSeen:  b.totalSeen,
		SumAll:     b.sumAll,
		UpdatedAt:  time.Now(),
	}
}

// LoadFromSnapshot reconstructs an STLBaseline from a persisted snapshot.
func LoadFromSnapshot(s BaselineSnapshot) *STLBaseline {
	b := &STLBaseline{
		MetricKey:  s.MetricKey,
		Period:     s.Period,
		windowSize: s.WindowSize,
		ring:       make([]float64, s.WindowSize),
		residuals:  make([]float64, s.WindowSize),
		ewma:       s.EWMA,
		ewmaAlpha:  s.EWMAAlpha,
		seasonal:   append([]float64{}, s.Seasonal...),
		seasonN:    append([]int{}, s.SeasonN...),
		resCount:   s.ResCount,
		totalSeen:  s.TotalSeen,
		sumAll:     s.SumAll,
	}
	copy(b.residuals, s.Residuals)
	b.resHead = s.ResCount % s.WindowSize
	return b
}
