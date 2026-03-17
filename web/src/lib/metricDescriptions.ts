// Metric description registry for PGPulse alert detail panels and tooltips.
// Covers all metric keys used in internal/alert/rules.go plus common collector keys.

export interface MetricDescription {
  name: string
  description: string
  significance: string
  pageLink?: string // e.g. "/servers/{id}/query-insights"
}

export const METRIC_DESCRIPTIONS: Record<string, MetricDescription> = {
  // --- Cache ---
  'pg.cache.hit_ratio': {
    name: 'Buffer Cache Hit Ratio',
    description:
      'Ratio of data pages found in shared_buffers vs read from disk. Computed from pg_stat_database blks_hit / (blks_hit + blks_read).',
    significance:
      'A ratio below 90% means PostgreSQL is frequently reading from disk, which is orders of magnitude slower than memory. Increase shared_buffers or add RAM.',
  },

  // --- Connections ---
  'pg.connections.utilization_pct': {
    name: 'Connection Utilization',
    description:
      'Percentage of active connections relative to max_connections. Includes all backends except the monitoring connection.',
    significance:
      'Above 80% risks refusing new connections. Above 99% the server is effectively full. Consider connection pooling (PgBouncer) or raising max_connections.',
  },

  // --- Transactions ---
  'pg.transactions.commit_ratio_pct': {
    name: 'Transaction Commit Ratio',
    description:
      'Percentage of committed transactions vs total (committed + rolled back). Derived from pg_stat_database xact_commit / (xact_commit + xact_rollback).',
    significance:
      'A ratio below 95% indicates a high rollback rate, which may signal application bugs, deadlocks, or serialization failures.',
  },

  // --- Server / Wraparound ---
  'pg.server.wraparound_pct': {
    name: 'Transaction ID Wraparound',
    description:
      'Percentage of the transaction ID space consumed. Computed from age(datfrozenxid) relative to the 2-billion wraparound limit.',
    significance:
      'At 100% PostgreSQL will shut down to prevent data corruption. Run VACUUM FREEZE on affected databases when this rises above 20%.',
  },
  'pg.server.multixact_pct': {
    name: 'MultiXact Wraparound',
    description:
      'Percentage of the multixact ID space consumed. Similar to TXID wraparound but for multi-transaction locks.',
    significance:
      'Like TXID wraparound, reaching the limit causes a forced shutdown. Monitor and run VACUUM FREEZE proactively.',
  },

  // --- Replication ---
  'pg.replication.lag.total_bytes': {
    name: 'Replication Lag (bytes)',
    description:
      'Total replication lag in bytes between the primary WAL insert position and the replica replay position.',
    significance:
      'Large lag means the replica is behind, risking stale reads and longer failover recovery. Investigate network, disk I/O, or heavy write workloads.',
  },
  'pg.replication.slot.active': {
    name: 'Replication Slot Active',
    description:
      'Whether a replication slot is currently active (1) or inactive (0). Inactive slots retain WAL indefinitely.',
    significance:
      'Inactive slots prevent WAL cleanup, causing disk usage to grow without bound. Drop or reactivate inactive slots promptly.',
  },

  // --- Long Transactions ---
  'pg.long_transactions.oldest_seconds': {
    name: 'Oldest Transaction Age',
    description:
      'Duration in seconds of the longest-running open transaction, including idle-in-transaction sessions.',
    significance:
      'Long transactions hold locks, prevent VACUUM from reclaiming dead tuples, and cause table bloat. Investigate and terminate if safe.',
  },
  'pg.long_transactions.active_count': {
    name: 'Long Active Transactions',
    description:
      'Number of transactions that have been running longer than the configured threshold (default 30 minutes).',
    significance:
      'Multiple long transactions compound bloat and lock contention. Review application connection management and query performance.',
  },

  // --- Locks ---
  'pg.locks.blocked_count': {
    name: 'Blocked Queries',
    description:
      'Number of queries currently waiting to acquire a lock held by another session.',
    significance:
      'Blocked queries cause latency spikes and can cascade into connection exhaustion. Check the lock tree to identify root blockers.',
  },

  // --- Bloat ---
  'pg.db.bloat.table_ratio': {
    name: 'Table Bloat Ratio',
    description:
      'Estimated percentage of wasted space in a table due to dead tuples that VACUUM has not yet reclaimed.',
    significance:
      'High bloat degrades sequential scan performance and wastes disk. Run VACUUM FULL or pg_repack to reclaim space.',
  },

  // --- pg_stat_statements ---
  'pg.extensions.pgss_fill_pct': {
    name: 'PGSS Fill Percentage',
    description:
      'Percentage of pg_stat_statements slots in use. When full, the least-used entries are evicted (deallocated).',
    significance:
      'Above 95% means query stats are being lost. Increase pg_stat_statements.max to retain full workload visibility.',
    pageLink: '/servers/{id}/query-insights',
  },

  // --- Logical Replication ---
  'pg.db.logical_replication.pending_sync_tables': {
    name: 'Logical Replication Pending Sync',
    description:
      'Number of tables in a logical replication subscription that have not yet completed initial synchronization.',
    significance:
      'Pending tables mean the subscription is not fully caught up. Check network and subscription status.',
  },

  // --- WAL ---
  'pg.wal.spike_ratio': {
    name: 'WAL Generation Spike Ratio',
    description:
      'Ratio of current WAL generation rate to the ML-computed baseline. Requires ML baseline (M8).',
    significance:
      'A spike above 3x baseline may indicate bulk loads, index rebuilds, or unexpected write amplification.',
  },

  // --- Statements Regression ---
  'pg.statements.regression_ratio': {
    name: 'Query Performance Regression',
    description:
      'Ratio of current query execution time to the ML-computed historical baseline. Requires ML baseline (M8).',
    significance:
      'A ratio above 2x indicates a query has slowed significantly. Check for plan changes, bloat, or missing indexes.',
    pageLink: '/servers/{id}/query-insights',
  },

  // --- Disk Forecast ---
  'pg.disk.days_remaining': {
    name: 'Disk Space Days Remaining',
    description:
      'Projected number of days until disk space is exhausted, based on ML growth trend analysis.',
    significance:
      'Below 7 days is critical. Free space, add storage, or archive old data immediately.',
  },

  // --- OS Metrics ---
  'os.cpu.user_pct': {
    name: 'CPU User Time',
    description:
      'Percentage of CPU time spent in user-space processes, collected from /proc/stat on the monitored host.',
    significance:
      'Sustained high CPU indicates the server is compute-bound. Profile queries or scale up.',
  },
  'os.cpu.system_pct': {
    name: 'CPU System Time',
    description:
      'Percentage of CPU time spent in kernel-space, including I/O scheduling and context switches.',
    significance:
      'High system CPU often points to I/O bottlenecks or excessive context switching.',
  },
  'os.cpu.iowait_pct': {
    name: 'CPU I/O Wait',
    description:
      'Percentage of CPU time waiting for I/O operations to complete.',
    significance:
      'High iowait indicates disk or network I/O is the bottleneck. Check disk utilization and query I/O patterns.',
  },
  'os.memory.available_kb': {
    name: 'Available Memory',
    description:
      'Amount of memory in KB available for use without swapping, from /proc/meminfo MemAvailable.',
    significance:
      'Low available memory leads to swapping, which catastrophically degrades PostgreSQL performance.',
  },
  'os.memory.total_kb': {
    name: 'Total Memory',
    description: 'Total physical memory in KB on the monitored host.',
    significance:
      'Reference value for sizing shared_buffers (typically 25% of total) and effective_cache_size.',
  },
  'os.disk.util_pct': {
    name: 'Disk Utilization',
    description:
      'Percentage of time the disk device was busy servicing I/O requests, from /proc/diskstats.',
    significance:
      'Sustained 100% utilization means the disk is saturated. Queries waiting on I/O will be slow.',
  },
  'os.load.1m': {
    name: 'System Load (1 min)',
    description:
      'One-minute load average from /proc/loadavg. Represents the average number of processes in a runnable or waiting state.',
    significance:
      'Load above the CPU count indicates resource contention. Check for runaway queries or insufficient CPU.',
  },
  'os.load.5m': {
    name: 'System Load (5 min)',
    description: 'Five-minute load average from /proc/loadavg.',
    significance:
      'Smoothed load trend. Useful for detecting sustained load vs transient spikes.',
  },
  'os.load.15m': {
    name: 'System Load (15 min)',
    description: 'Fifteen-minute load average from /proc/loadavg.',
    significance:
      'Long-term load trend. Compare with 1m to see if load is rising or falling.',
  },

  // --- ML Anomaly ---
  'anomaly.': {
    name: 'ML Anomaly Detection',
    description:
      'Z-score from ML anomaly detection. Indicates how many standard deviations the current value is from the predicted baseline.',
    significance:
      'A Z-score above 3 is a warning; above 5 is critical. Investigate the underlying metric for unexpected changes.',
  },
}

/**
 * Look up a metric description by exact key.
 * Falls back to prefix matching for keys like "anomaly.xxx".
 */
export function getMetricDescription(metricKey: string): MetricDescription | null {
  const exact = METRIC_DESCRIPTIONS[metricKey]
  if (exact) return exact

  // Prefix match for anomaly.* keys
  for (const prefix of Object.keys(METRIC_DESCRIPTIONS)) {
    if (prefix.endsWith('.') && metricKey.startsWith(prefix)) {
      return METRIC_DESCRIPTIONS[prefix]
    }
  }
  return null
}

/**
 * Get a navigable page link for a metric, replacing {id} with the instance ID.
 */
export function getMetricPageLink(metricKey: string, instanceId: string): string | null {
  const desc = getMetricDescription(metricKey)
  if (!desc?.pageLink) return null
  return desc.pageLink.replace('{id}', instanceId)
}
