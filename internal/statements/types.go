package statements

import "time"

// Snapshot holds metadata for one pg_stat_statements capture.
type Snapshot struct {
	ID              int64      `json:"id"`
	InstanceID      string     `json:"instance_id"`
	CapturedAt      time.Time  `json:"captured_at"`
	PGVersion       int        `json:"pg_version"`
	StatsReset      *time.Time `json:"stats_reset,omitempty"`
	TotalStatements int        `json:"total_statements"`
	TotalCalls      int64      `json:"total_calls"`
	TotalExecTime   float64    `json:"total_exec_time_ms"`
}

// SnapshotEntry holds one query's counters at a point in time.
type SnapshotEntry struct {
	SnapshotID        int64    `json:"snapshot_id"`
	QueryID           int64    `json:"queryid"`
	UserID            uint32   `json:"userid"`
	DbID              uint32   `json:"dbid"`
	DatabaseName      string   `json:"database_name,omitempty"`
	UserName          string   `json:"user_name,omitempty"`
	Query             string   `json:"query"`
	Calls             int64    `json:"calls"`
	TotalExecTime     float64  `json:"total_exec_time_ms"`
	TotalPlanTime     *float64 `json:"total_plan_time_ms,omitempty"`
	Rows              int64    `json:"rows"`
	SharedBlksHit     int64    `json:"shared_blks_hit"`
	SharedBlksRead    int64    `json:"shared_blks_read"`
	SharedBlksDirtied int64    `json:"shared_blks_dirtied"`
	SharedBlksWritten int64    `json:"shared_blks_written"`
	LocalBlksHit      int64    `json:"local_blks_hit"`
	LocalBlksRead     int64    `json:"local_blks_read"`
	TempBlksRead      int64    `json:"temp_blks_read"`
	TempBlksWritten   int64    `json:"temp_blks_written"`
	BlkReadTime       float64  `json:"blk_read_time_ms"`
	BlkWriteTime      float64  `json:"blk_write_time_ms"`
	WALRecords        *int64   `json:"wal_records,omitempty"`
	WALFpi            *int64   `json:"wal_fpi,omitempty"`
	WALBytes          *float64 `json:"wal_bytes,omitempty"`
	MeanExecTime      *float64 `json:"mean_exec_time_ms,omitempty"`
	MinExecTime       *float64 `json:"min_exec_time_ms,omitempty"`
	MaxExecTime       *float64 `json:"max_exec_time_ms,omitempty"`
	StddevExecTime    *float64 `json:"stddev_exec_time_ms,omitempty"`
}

// DiffResult is the output of comparing two snapshots.
type DiffResult struct {
	FromSnapshot       Snapshot      `json:"from_snapshot"`
	ToSnapshot         Snapshot      `json:"to_snapshot"`
	StatsResetWarning  bool          `json:"stats_reset_warning"`
	Duration           time.Duration `json:"-"`
	DurationStr        string        `json:"duration"`
	TotalCallsDelta    int64         `json:"total_calls_delta"`
	TotalExecTimeDelta float64       `json:"total_exec_time_delta_ms"`
	Entries            []DiffEntry   `json:"entries"`
	NewQueries         []DiffEntry   `json:"new_queries"`
	EvictedQueries     []DiffEntry   `json:"evicted_queries"`
	TotalEntries       int           `json:"total_entries"`
}

// DiffEntry holds per-query delta between two snapshots.
type DiffEntry struct {
	QueryID              int64    `json:"queryid"`
	UserID               uint32   `json:"userid"`
	DbID                 uint32   `json:"dbid"`
	Query                string   `json:"query"`
	DatabaseName         string   `json:"database_name,omitempty"`
	UserName             string   `json:"user_name,omitempty"`
	CallsDelta           int64    `json:"calls_delta"`
	ExecTimeDelta        float64  `json:"exec_time_delta_ms"`
	PlanTimeDelta        *float64 `json:"plan_time_delta_ms,omitempty"`
	RowsDelta            int64    `json:"rows_delta"`
	SharedBlksReadDelta  int64    `json:"shared_blks_read_delta"`
	SharedBlksHitDelta   int64    `json:"shared_blks_hit_delta"`
	TempBlksReadDelta    int64    `json:"temp_blks_read_delta"`
	TempBlksWrittenDelta int64    `json:"temp_blks_written_delta"`
	BlkReadTimeDelta     float64  `json:"blk_read_time_delta_ms"`
	BlkWriteTimeDelta    float64  `json:"blk_write_time_delta_ms"`
	WALBytesDelta        *float64 `json:"wal_bytes_delta,omitempty"`
	AvgExecTimePerCall   float64  `json:"avg_exec_time_per_call_ms"`
	IOTimePct            float64  `json:"io_time_pct"`
	CPUTimeDelta         float64  `json:"cpu_time_delta_ms"`
	SharedHitRatio       float64  `json:"shared_hit_ratio_pct"`
}

// QueryInsight is a time-series view for a single queryid across snapshots.
type QueryInsight struct {
	QueryID      int64               `json:"queryid"`
	Query        string              `json:"query"`
	DatabaseName string              `json:"database_name"`
	UserName     string              `json:"user_name"`
	FirstSeen    time.Time           `json:"first_seen"`
	Points       []QueryInsightPoint `json:"points"`
}

// QueryInsightPoint holds one interval's delta for a query.
type QueryInsightPoint struct {
	CapturedAt     time.Time `json:"captured_at"`
	CallsDelta     int64     `json:"calls_delta"`
	ExecTimeDelta  float64   `json:"exec_time_delta_ms"`
	RowsDelta      int64     `json:"rows_delta"`
	AvgExecTime    float64   `json:"avg_exec_time_ms"`
	SharedHitRatio float64   `json:"shared_hit_ratio_pct"`
}

// WorkloadReport is a structured report with top-N sections.
type WorkloadReport struct {
	InstanceID        string        `json:"instance_id"`
	FromTime          time.Time     `json:"from_time"`
	ToTime            time.Time     `json:"to_time"`
	Duration          string        `json:"duration"`
	StatsResetWarning bool          `json:"stats_reset_warning"`
	Summary           ReportSummary `json:"summary"`
	TopByExecTime     []DiffEntry   `json:"top_by_exec_time"`
	TopByCalls        []DiffEntry   `json:"top_by_calls"`
	TopByRows         []DiffEntry   `json:"top_by_rows"`
	TopByIOReads      []DiffEntry   `json:"top_by_io_reads"`
	TopByAvgTime      []DiffEntry   `json:"top_by_avg_time"`
	NewQueries        []DiffEntry   `json:"new_queries"`
	EvictedQueries    []DiffEntry   `json:"evicted_queries"`
}

// ReportSummary holds aggregate statistics for a workload report.
type ReportSummary struct {
	TotalQueries       int     `json:"total_queries"`
	TotalCallsDelta    int64   `json:"total_calls_delta"`
	TotalExecTimeDelta float64 `json:"total_exec_time_delta_ms"`
	TotalRowsDelta     int64   `json:"total_rows_delta"`
	UniqueQueries      int     `json:"unique_queries"`
	NewQueries         int     `json:"new_queries"`
	EvictedQueries     int     `json:"evicted_queries"`
}

// SnapshotListOptions holds pagination and filter options for listing snapshots.
type SnapshotListOptions struct {
	Limit  int
	Offset int
	From   *time.Time
	To     *time.Time
}

// DiffOptions holds sorting and pagination for diff results.
type DiffOptions struct {
	SortBy string // total_exec_time, calls, rows, shared_blks_read, avg_exec_time
	Limit  int
	Offset int
}
