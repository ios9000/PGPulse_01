package collector

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/cluster/etcd"
	"github.com/ios9000/PGPulse_01/internal/cluster/patroni"
)

// ClusterCollector collects HA cluster metrics from Patroni and ETCD providers.
type ClusterCollector struct {
	instanceID string
	patroni    patroni.PatroniProvider
	etcd       etcd.ETCDProvider
	logger     *slog.Logger
}

// NewClusterCollector creates a ClusterCollector with the given providers.
func NewClusterCollector(instanceID string, p patroni.PatroniProvider, e etcd.ETCDProvider, logger *slog.Logger) *ClusterCollector {
	return &ClusterCollector{
		instanceID: instanceID,
		patroni:    p,
		etcd:       e,
		logger:     logger,
	}
}

func (c *ClusterCollector) Name() string            { return "cluster" }
func (c *ClusterCollector) Interval() time.Duration { return 30 * time.Second }

// Collect gathers cluster metrics from Patroni and ETCD.
// Provider errors are logged at WARN level; partial data is returned (not errors).
func (c *ClusterCollector) Collect(ctx context.Context, _ *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	now := time.Now()
	var points []MetricPoint

	point := func(metric string, value float64, labels map[string]string) MetricPoint {
		return MetricPoint{
			InstanceID: c.instanceID,
			Metric:     metric,
			Value:      value,
			Labels:     labels,
			Timestamp:  now,
		}
	}

	// Patroni
	if c.patroni != nil {
		state, err := c.patroni.GetClusterState(ctx)
		if err != nil {
			if !errors.Is(err, patroni.ErrPatroniNotConfigured) {
				c.logger.Warn("patroni cluster state collection failed", "instance", c.instanceID, "error", err)
			}
		} else if state != nil {
			var leaderCount, replicaCount float64
			for _, m := range state.Members {
				switch m.Role {
				case "leader", "Leader":
					leaderCount++
				default:
					replicaCount++
				}
				stateVal := 0.0
				if m.State == "running" {
					stateVal = 1.0
				}
				labels := map[string]string{"member": m.Name, "role": m.Role}
				points = append(points,
					point("cluster.patroni.member_state", stateVal, labels),
					point("cluster.patroni.member_lag_bytes", float64(m.Lag), labels),
				)
			}
			points = append(points,
				point("cluster.patroni.member_count", float64(len(state.Members)), nil),
				point("cluster.patroni.leader_count", leaderCount, nil),
				point("cluster.patroni.replica_count", replicaCount, nil),
			)
		}
	}

	// ETCD
	if c.etcd != nil {
		members, err := c.etcd.GetMembers(ctx)
		if err != nil {
			if !errors.Is(err, etcd.ErrETCDNotConfigured) {
				c.logger.Warn("etcd member list collection failed", "instance", c.instanceID, "error", err)
			}
		} else {
			var leaderCount float64
			for _, m := range members {
				if m.IsLeader {
					leaderCount++
				}
			}
			points = append(points,
				point("cluster.etcd.member_count", float64(len(members)), nil),
				point("cluster.etcd.leader_count", leaderCount, nil),
			)
		}

		health, err := c.etcd.GetEndpointHealth(ctx)
		if err != nil {
			if !errors.Is(err, etcd.ErrETCDNotConfigured) {
				c.logger.Warn("etcd health collection failed", "instance", c.instanceID, "error", err)
			}
		} else {
			for endpoint, healthy := range health {
				val := 0.0
				if healthy {
					val = 1.0
				}
				points = append(points,
					point("cluster.etcd.member_healthy", val, map[string]string{"member": endpoint}),
				)
			}
		}
	}

	return points, nil
}
