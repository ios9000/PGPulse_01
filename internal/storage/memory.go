package storage

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// MemoryStore implements collector.MetricStore with an in-memory ring buffer.
// Used in live mode when no persistent storage DSN is configured.
type MemoryStore struct {
	mu        sync.RWMutex
	retention time.Duration
	data      map[string][]collector.MetricPoint
	done      chan struct{}
}

// NewMemoryStore creates a MemoryStore that retains data for the given duration.
// It starts a background goroutine that expires old data every 30 seconds.
func NewMemoryStore(retention time.Duration) *MemoryStore {
	m := &MemoryStore{
		retention: retention,
		data:      make(map[string][]collector.MetricPoint),
		done:      make(chan struct{}),
	}
	go m.expireLoop()
	return m
}

// Write appends metric points to their respective key slices.
func (m *MemoryStore) Write(_ context.Context, points []collector.MetricPoint) error {
	if len(points) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range points {
		key := storageKey(p.InstanceID, p.Metric, p.Labels)
		m.data[key] = append(m.data[key], p)
	}
	return nil
}

// Query returns metric points matching the query parameters, sorted by timestamp ascending.
func (m *MemoryStore) Query(_ context.Context, query collector.MetricQuery) ([]collector.MetricPoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []collector.MetricPoint
	for key, points := range m.data {
		if query.InstanceID != "" && !strings.HasPrefix(key, query.InstanceID+"\x00") {
			continue
		}
		if query.Metric != "" {
			parts := strings.SplitN(key, "\x00", 3)
			if len(parts) < 2 || parts[1] != query.Metric {
				continue
			}
		}
		for _, p := range points {
			if !query.Start.IsZero() && p.Timestamp.Before(query.Start) {
				continue
			}
			if !query.End.IsZero() && p.Timestamp.After(query.End) {
				continue
			}
			if len(query.Labels) > 0 && !matchLabels(p.Labels, query.Labels) {
				continue
			}
			results = append(results, p)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.Before(results[j].Timestamp)
	})

	if query.Limit > 0 && len(results) > query.Limit {
		results = results[len(results)-query.Limit:]
	}

	return results, nil
}

// Close stops the expiry goroutine.
func (m *MemoryStore) Close() error {
	select {
	case <-m.done:
		// already closed
	default:
		close(m.done)
	}
	return nil
}

// GetMetricStats computes statistics for the given metrics from in-memory data.
func (m *MemoryStore) GetMetricStats(_ context.Context, instanceID string, keys []string, from, to time.Time) (map[string]MetricStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keySet := make(map[string]bool, len(keys))
	for _, k := range keys {
		keySet[k] = true
	}

	// Collect values per metric.
	buckets := make(map[string][]float64)
	for storageKey, points := range m.data {
		parts := strings.SplitN(storageKey, "\x00", 3)
		if len(parts) < 2 {
			continue
		}
		if instanceID != "" && parts[0] != instanceID {
			continue
		}
		if !keySet[parts[1]] {
			continue
		}
		for _, p := range points {
			if !from.IsZero() && p.Timestamp.Before(from) {
				continue
			}
			if !to.IsZero() && p.Timestamp.After(to) {
				continue
			}
			buckets[parts[1]] = append(buckets[parts[1]], p.Value)
		}
	}

	result := make(map[string]MetricStats)
	for metric, values := range buckets {
		if len(values) == 0 {
			continue
		}
		var sum float64
		mn := values[0]
		mx := values[0]
		for _, v := range values {
			sum += v
			if v < mn {
				mn = v
			}
			if v > mx {
				mx = v
			}
		}
		mean := sum / float64(len(values))
		var variance float64
		if len(values) > 1 {
			for _, v := range values {
				d := v - mean
				variance += d * d
			}
			variance /= float64(len(values) - 1) // sample variance
		}
		result[metric] = MetricStats{
			Mean:   mean,
			StdDev: math.Sqrt(variance),
			Min:    mn,
			Max:    mx,
			Count:  len(values),
		}
	}
	return result, nil
}

// storageKey builds a deterministic map key from instanceID, metric, and sorted labels.
func storageKey(instanceID, metric string, labels map[string]string) string {
	if len(labels) == 0 {
		return instanceID + "\x00" + metric + "\x00"
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(instanceID)
	b.WriteByte(0)
	b.WriteString(metric)
	b.WriteByte(0)
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
	}
	return b.String()
}

// matchLabels returns true if all queryLabels are present in pointLabels.
func matchLabels(pointLabels, queryLabels map[string]string) bool {
	for k, v := range queryLabels {
		if pointLabels[k] != v {
			return false
		}
	}
	return true
}

// expireLoop runs every 30 seconds and trims expired data.
func (m *MemoryStore) expireLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.expire()
		}
	}
}

// expire removes data points older than the retention window.
func (m *MemoryStore) expire() {
	cutoff := time.Now().Add(-m.retention)
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, points := range m.data {
		idx := sort.Search(len(points), func(i int) bool {
			return !points[i].Timestamp.Before(cutoff)
		})
		if idx >= len(points) {
			delete(m.data, key)
		} else if idx > 0 {
			m.data[key] = points[idx:]
		}
	}
}
