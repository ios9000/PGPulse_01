package rca

import "time"

// NewDefaultGraph builds the 20-chain causal graph.
// 16 chains are Tier A (active), 4 are Tier B (experimental).
func NewDefaultGraph() *CausalGraph {
	g := &CausalGraph{
		Nodes:    make(map[string]*CausalNode),
		ChainIDs: AllChainIDs,
	}

	// Shared nodes — referenced by multiple chains.
	addNode(g, "bulk_workload", "Bulk Workload", []string{
		"pg.statements.top.total_time_ms", "pg.statements.top.calls",
	}, "workload", "", "")
	addNode(g, "wal_generation", "WAL Generation Rate", []string{
		"pg.checkpoint.buffers_written_per_second",
	}, "db", "", MechWALSpike)
	addNode(g, "checkpoint_storm", "Checkpoint Storm", []string{
		"pg.checkpoint.requested_per_second", "pg.checkpoint.sync_time_ms",
	}, "db", "", MechCheckpointStorm)
	addNode(g, "disk_io", "Disk I/O Saturation", []string{
		"os.disk.util_pct", "os.disk.write_bytes_per_sec", "os.cpu.iowait_pct",
	}, "os", SymDiskIOSaturated, "")
	addNode(g, "replication_lag", "Replication Lag", []string{
		"pg.replication.lag.replay_bytes", "pg.replication.lag.replay_seconds",
	}, "db", SymReplicationLagHigh, "")
	addNode(g, "inactive_slot", "Inactive Replication Slot", []string{
		"pg.replication.slot.active", "pg.replication.slot.retained_bytes",
	}, "db", "", "")
	addNode(g, "wal_retention", "WAL Retention", []string{
		"pg.replication.slot.retained_bytes",
	}, "db", "", MechWALRetention)
	addNode(g, "disk_fill", "Disk Space Low", []string{
		"os.disk.used_bytes", "os.disk.free_bytes",
	}, "os", SymDiskSpaceLow, "")
	addNode(g, "long_tx_primary", "Long Transaction on Primary", []string{
		"pg.long_transactions.oldest_seconds", "pg.long_transactions.count",
	}, "db", "", "")
	addNode(g, "replication_apply_delay", "Replication Apply Delay", []string{
		"pg.replication.lag.replay_bytes", "pg.replication.lag.replay_seconds",
	}, "db", SymReplicationLagHigh, "")
	addNode(g, "autovacuum_storm", "Autovacuum Storm", []string{
		"pg.progress.vacuum.completion_pct", "pg.db.vacuum.dead_tuples",
	}, "db", "", MechAutovacuumStorm)
	addNode(g, "query_latency", "Query Latency", []string{
		"pg.statements.top.avg_time_ms", "pg.wait_events.count",
	}, "db", SymQueryLatencyHigh, "")
	addNode(g, "temp_file_spike", "Temp File Spike", []string{
		"pg.io.writes", "pg.io.write_time",
	}, "db", "", MechTempFileSpike)
	addNode(g, "query_slowdown", "Query Slowdown", []string{
		"pg.statements.top.avg_time_ms",
	}, "db", SymQueryLatencyHigh, "")
	addNode(g, "buffer_eviction", "Shared Buffers Eviction", []string{
		"pg.bgwriter.buffers_backend_per_second", "pg.bgwriter.buffers_alloc_per_second",
	}, "db", "", "")
	addNode(g, "backend_writes", "Backend Direct Writes", []string{
		"pg.bgwriter.buffers_backend_per_second",
	}, "db", "", MechBufferBackendWrites)
	addNode(g, "lock_contention", "Lock Contention", []string{
		"pg.locks.blocker_count", "pg.locks.blocked_count", "pg.locks.max_chain_depth",
	}, "db", "", MechLockContention)
	addNode(g, "blocked_queries", "Blocked Queries", []string{
		"pg.wait_events.count",
	}, "db", "", MechBlockedQueries)
	addNode(g, "connection_pileup", "Connection Pileup", []string{
		"pg.connections.total", "pg.connections.utilization_pct",
	}, "db", SymConnectionExhaustion, "")
	addNode(g, "connection_spike", "Connection Spike", []string{
		"pg.connections.total", "pg.connections.utilization_pct",
	}, "db", "", "")
	addNode(g, "memory_pressure", "Memory Pressure", []string{
		"os.memory.available_kb", "os.memory.used_kb",
	}, "os", "", MechMemoryPressure)
	addNode(g, "oom_risk", "OOM Risk", []string{
		"os.memory.available_kb",
	}, "os", SymOOMRisk, "")
	addNode(g, "long_tx", "Long Transaction", []string{
		"pg.long_transactions.oldest_seconds", "pg.long_transactions.count",
	}, "db", "", "")
	addNode(g, "mvcc_bloat", "MVCC Bloat", []string{
		"pg.db.vacuum.dead_tuples", "pg.db.vacuum.dead_pct",
	}, "db", "", MechMVCCBloat)
	addNode(g, "conn_holding", "Connection Holding", []string{
		"pg.connections.total", "pg.connections.utilization_pct",
	}, "db", SymConnectionExhaustion, "")
	addNode(g, "missing_index", "Missing Index", []string{
		"pg.db.index.unused_scans", "pg.db.table.cache_hit_pct",
	}, "db", "", "")
	addNode(g, "seq_scans", "Sequential Scans", []string{
		"pg.io.reads", "pg.io.read_time",
	}, "db", "", MechSeqScans)
	addNode(g, "pgss_fill", "pg_stat_statements Fill", []string{
		"pg.statements.fill_pct", "pg.extensions.pgss_fill_pct",
	}, "db", "", "")
	addNode(g, "pgss_eviction", "pg_stat_statements Eviction", []string{
		"pg.statements.fill_pct",
	}, "db", SymPGSSEviction, "")
	addNode(g, "dead_tuples", "Dead Tuple Accumulation", []string{
		"pg.db.vacuum.dead_tuples", "pg.db.vacuum.dead_pct",
	}, "db", "", "")
	addNode(g, "table_bloat", "Table Bloat", []string{
		"pg.db.bloat.table_ratio",
	}, "db", "", MechTableBloat)
	addNode(g, "scan_degradation", "Scan Degradation", []string{
		"pg.statements.top.avg_time_ms", "pg.io.reads",
	}, "db", SymScanDegradation, "")
	addNode(g, "wraparound_risk", "Wraparound Risk", []string{
		"pg.server.wraparound_pct",
	}, "db", "", "")
	addNode(g, "aggressive_vacuum", "Aggressive Anti-Wraparound Vacuum", []string{
		"pg.progress.vacuum.completion_pct", "pg.db.vacuum.dead_tuples",
	}, "db", "", MechAggressiveVacuum)
	addNode(g, "long_tx_blocking", "Long TX Blocking Vacuum", []string{
		"pg.long_transactions.oldest_seconds",
	}, "db", "", "")
	addNode(g, "vacuum_blocked", "Vacuum Blocked", []string{
		"pg.db.vacuum.dead_tuples", "pg.db.vacuum.dead_pct",
	}, "db", "", MechVacuumBlocked)
	addNode(g, "dead_tuple_growth", "Dead Tuple Growth", []string{
		"pg.db.vacuum.dead_tuples", "pg.db.bloat.table_ratio",
	}, "db", SymDeadTupleGrowth, "")
	addNode(g, "os_memory_pressure", "OS Memory Pressure", []string{
		"os.memory.available_kb", "os.memory.used_kb", "os.memory.total_kb",
	}, "os", "", "")
	addNode(g, "oom_killer", "OOM Killer", []string{
		"os.memory.available_kb",
	}, "os", "", MechOOMKiller)
	addNode(g, "pg_crash", "PostgreSQL Crash", []string{
		"pg.server.uptime_seconds",
	}, "db", SymPGCrash, "")

	// Tier B only nodes.
	addNode(g, "network_issue", "Network Issue", []string{}, "os", "", "")
	addNode(g, "wal_receiver_disconnect", "WAL Receiver Disconnect", []string{
		"pg.replication.wal_receiver.connected",
	}, "db", "", MechWALRecvDisconnect)
	addNode(g, "query_regression", "Query Regression", []string{
		"pg.statements.top.avg_time_ms",
	}, "workload", "", "")
	addNode(g, "cpu_spike", "CPU Spike", []string{
		"os.cpu.user_pct", "os.cpu.system_pct",
	}, "os", SymCPUSpike, "")
	addNode(g, "latency_spike", "Latency Spike", []string{
		"pg.statements.top.avg_time_ms",
	}, "db", SymQueryLatencyHigh, "")
	addNode(g, "new_query", "New Query (Deployment)", []string{
		"pg.statements.count",
	}, "workload", "", "")
	addNode(g, "resource_shift", "Resource Shift", []string{
		"os.cpu.user_pct", "pg.statements.top.total_time_ms",
	}, "workload", SymResourceShift, "")
	addNode(g, "settings_change", "Settings Change", []string{}, "config", "", "")
	addNode(g, "behavioral_shift", "Behavioral Shift", []string{}, "workload", SymBehavioralShift, "")

	// --- Chain 1 (Tier A): Bulk workload -> WAL -> checkpoint -> disk I/O -> replication lag ---
	addEdge(g, CausalEdge{
		FromNode: "bulk_workload", ToNode: "wal_generation",
		MinLag: 0, MaxLag: 30 * time.Second,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "Bulk workload drives increased WAL generation",
		BaseConfidence: 0.8, ChainID: ChainBulkWALCheckpointIOReplLag,
		RemediationHook: HookQueryOptimization,
	})
	addEdge(g, CausalEdge{
		FromNode: "wal_generation", ToNode: "checkpoint_storm",
		MinLag: 30 * time.Second, MaxLag: 3 * time.Minute,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "High WAL rate triggers frequent checkpoints",
		BaseConfidence: 0.85, ChainID: ChainBulkWALCheckpointIOReplLag,
		RemediationHook: HookCheckpointTuning,
	})
	addEdge(g, CausalEdge{
		FromNode: "checkpoint_storm", ToNode: "disk_io",
		MinLag: 0, MaxLag: 30 * time.Second,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "Checkpoint fsync saturates disk I/O",
		BaseConfidence: 0.9, ChainID: ChainBulkWALCheckpointIOReplLag,
	})
	addEdge(g, CausalEdge{
		FromNode: "disk_io", ToNode: "replication_lag",
		MinLag: 10 * time.Second, MaxLag: 2 * time.Minute,
		Temporal: BoundedLag, Evidence: EvidenceSupporting,
		Description: "Disk I/O saturation delays WAL replay on replica",
		BaseConfidence: 0.75, ChainID: ChainBulkWALCheckpointIOReplLag,
	})

	// --- Chain 2 (Tier A): Inactive slot -> WAL retention -> disk fill ---
	addEdge(g, CausalEdge{
		FromNode: "inactive_slot", ToNode: "wal_retention",
		Temporal: PersistentState, Evidence: EvidenceRequired,
		Description: "Inactive replication slot prevents WAL cleanup",
		BaseConfidence: 0.9, ChainID: ChainInactiveSlotWALDisk,
		RemediationHook: HookSlotCleanup,
	})
	addEdge(g, CausalEdge{
		FromNode: "wal_retention", ToNode: "disk_fill",
		MinLag: 5 * time.Minute, MaxLag: 2 * time.Hour,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "Accumulated WAL files consume disk space",
		BaseConfidence: 0.85, ChainID: ChainInactiveSlotWALDisk,
	})

	// --- Chain 3 (Tier B): Network issue -> WAL receiver disconnect -> replication lag ---
	addEdge(g, CausalEdge{
		FromNode: "network_issue", ToNode: "wal_receiver_disconnect",
		MinLag: 0, MaxLag: 30 * time.Second,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "Network failure disconnects WAL receiver",
		BaseConfidence: 0.7, ChainID: ChainNetworkWALRecvReplLag,
		RemediationHook: HookNetworkDiagnostics,
	})
	addEdge(g, CausalEdge{
		FromNode: "wal_receiver_disconnect", ToNode: "replication_lag",
		MinLag: 0, MaxLag: 2 * time.Minute,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "Disconnected WAL receiver causes growing replication lag",
		BaseConfidence: 0.8, ChainID: ChainNetworkWALRecvReplLag,
	})

	// --- Chain 4 (Tier A): Long transaction on primary -> replication apply delay ---
	addEdge(g, CausalEdge{
		FromNode: "long_tx_primary", ToNode: "replication_apply_delay",
		Temporal: PersistentState, Evidence: EvidenceRequired,
		Description: "Long transaction on primary delays replication apply",
		BaseConfidence: 0.75, ChainID: ChainLongTxReplDelay,
		RemediationHook: HookKillLongTx,
	})

	// --- Chain 5 (Tier A): WAL spike -> checkpoint storm -> disk I/O ---
	addEdge(g, CausalEdge{
		FromNode: "wal_generation", ToNode: "checkpoint_storm",
		MinLag: 30 * time.Second, MaxLag: 3 * time.Minute,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "WAL spike forces frequent checkpoints",
		BaseConfidence: 0.85, ChainID: ChainWALSpikeCheckpointIO,
		RemediationHook: HookCheckpointTuning,
	})
	addEdge(g, CausalEdge{
		FromNode: "checkpoint_storm", ToNode: "disk_io",
		MinLag: 0, MaxLag: 30 * time.Second,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "Checkpoint storm saturates disk I/O",
		BaseConfidence: 0.9, ChainID: ChainWALSpikeCheckpointIO,
	})

	// --- Chain 6 (Tier A): Autovacuum storm -> disk I/O -> query latency ---
	addEdge(g, CausalEdge{
		FromNode: "autovacuum_storm", ToNode: "disk_io",
		MinLag: 0, MaxLag: 2 * time.Minute,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "Autovacuum storm generates heavy disk I/O",
		BaseConfidence: 0.8, ChainID: ChainAutovacuumDiskLatency,
		RemediationHook: HookVacuumTuning,
	})
	addEdge(g, CausalEdge{
		FromNode: "disk_io", ToNode: "query_latency",
		MinLag: 0, MaxLag: 30 * time.Second,
		Temporal: BoundedLag, Evidence: EvidenceSupporting,
		Description: "Disk I/O contention increases query latency",
		BaseConfidence: 0.7, ChainID: ChainAutovacuumDiskLatency,
	})

	// --- Chain 7 (Tier A): Temp file spike -> disk I/O -> query slowdown ---
	addEdge(g, CausalEdge{
		FromNode: "temp_file_spike", ToNode: "disk_io",
		MinLag: 0, MaxLag: 30 * time.Second,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "Temporary file writes saturate disk I/O",
		BaseConfidence: 0.8, ChainID: ChainTempFileDiskQuery,
		RemediationHook: HookTempFileWork,
	})
	addEdge(g, CausalEdge{
		FromNode: "disk_io", ToNode: "query_slowdown",
		MinLag: 0, MaxLag: 30 * time.Second,
		Temporal: BoundedLag, Evidence: EvidenceSupporting,
		Description: "Disk I/O contention slows queries",
		BaseConfidence: 0.7, ChainID: ChainTempFileDiskQuery,
	})

	// --- Chain 8 (Tier A): Buffer eviction -> backend writes -> disk I/O ---
	addEdge(g, CausalEdge{
		FromNode: "buffer_eviction", ToNode: "backend_writes",
		MinLag: 0, MaxLag: 10 * time.Second,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "Shared buffer eviction forces backend direct writes",
		BaseConfidence: 0.85, ChainID: ChainBufferEvictionBackendIO,
		RemediationHook: HookSharedBuffers,
	})
	addEdge(g, CausalEdge{
		FromNode: "backend_writes", ToNode: "disk_io",
		MinLag: 0, MaxLag: 30 * time.Second,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "Backend direct writes saturate disk I/O",
		BaseConfidence: 0.8, ChainID: ChainBufferEvictionBackendIO,
	})

	// --- Chain 9 (Tier A): Lock contention -> blocked queries -> connection pileup ---
	addEdge(g, CausalEdge{
		FromNode: "lock_contention", ToNode: "blocked_queries",
		MinLag: 0, MaxLag: 10 * time.Second,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "Lock contention blocks waiting queries",
		BaseConfidence: 0.9, ChainID: ChainLockBlockedConnPileup,
		RemediationHook: HookLockInvestigation,
	})
	addEdge(g, CausalEdge{
		FromNode: "blocked_queries", ToNode: "connection_pileup",
		MinLag: 10 * time.Second, MaxLag: 2 * time.Minute,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "Blocked queries accumulate open connections",
		BaseConfidence: 0.8, ChainID: ChainLockBlockedConnPileup,
	})

	// --- Chain 10 (Tier A): Connection spike -> memory pressure -> OOM risk ---
	addEdge(g, CausalEdge{
		FromNode: "connection_spike", ToNode: "memory_pressure",
		MinLag: 0, MaxLag: 2 * time.Minute,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "Excessive connections consume work_mem allocations",
		BaseConfidence: 0.75, ChainID: ChainConnSpikeMemoryOOM,
		RemediationHook: HookConnectionPooling,
	})
	addEdge(g, CausalEdge{
		FromNode: "memory_pressure", ToNode: "oom_risk",
		Temporal: PersistentState, Evidence: EvidenceSupporting,
		Description: "Sustained memory pressure increases OOM risk",
		BaseConfidence: 0.7, ChainID: ChainConnSpikeMemoryOOM,
	})

	// --- Chain 11 (Tier A): Long tx -> MVCC bloat -> connection holding ---
	addEdge(g, CausalEdge{
		FromNode: "long_tx", ToNode: "mvcc_bloat",
		Temporal: PersistentState, Evidence: EvidenceRequired,
		Description: "Long transaction prevents dead tuple cleanup",
		BaseConfidence: 0.8, ChainID: ChainLongTxBloatConn,
		RemediationHook: HookKillLongTx,
	})
	addEdge(g, CausalEdge{
		FromNode: "mvcc_bloat", ToNode: "conn_holding",
		MinLag: 1 * time.Minute, MaxLag: 10 * time.Minute,
		Temporal: BoundedLag, Evidence: EvidenceSupporting,
		Description: "Table bloat causes longer scans, holding connections",
		BaseConfidence: 0.6, ChainID: ChainLongTxBloatConn,
	})

	// --- Chain 12 (Tier B): Query regression -> CPU spike -> latency spike ---
	addEdge(g, CausalEdge{
		FromNode: "query_regression", ToNode: "cpu_spike",
		MinLag: 0, MaxLag: 2 * time.Minute,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "Query regression increases CPU consumption",
		BaseConfidence: 0.75, ChainID: ChainQueryRegressionCPULatency,
		RemediationHook: HookQueryOptimization,
	})
	addEdge(g, CausalEdge{
		FromNode: "cpu_spike", ToNode: "latency_spike",
		MinLag: 0, MaxLag: 30 * time.Second,
		Temporal: BoundedLag, Evidence: EvidenceSupporting,
		Description: "CPU saturation increases query latency",
		BaseConfidence: 0.7, ChainID: ChainQueryRegressionCPULatency,
	})

	// --- Chain 13 (Tier B): New query -> resource shift ---
	addEdge(g, CausalEdge{
		FromNode: "new_query", ToNode: "resource_shift",
		MinLag: 0, MaxLag: 5 * time.Minute,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "New query after deployment shifts resource usage",
		BaseConfidence: 0.65, ChainID: ChainNewQueryResourceShift,
	})

	// --- Chain 14 (Tier A): Missing index -> sequential scans -> disk I/O ---
	addEdge(g, CausalEdge{
		FromNode: "missing_index", ToNode: "seq_scans",
		Temporal: PersistentState, Evidence: EvidenceRequired,
		Description: "Missing index forces sequential scans",
		BaseConfidence: 0.8, ChainID: ChainMissingIndexSeqScanIO,
		RemediationHook: HookIndexCreation,
	})
	addEdge(g, CausalEdge{
		FromNode: "seq_scans", ToNode: "disk_io",
		MinLag: 0, MaxLag: 30 * time.Second,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "Sequential scans generate heavy read I/O",
		BaseConfidence: 0.85, ChainID: ChainMissingIndexSeqScanIO,
	})

	// --- Chain 15 (Tier A): pg_stat_statements fill -> eviction ---
	addEdge(g, CausalEdge{
		FromNode: "pgss_fill", ToNode: "pgss_eviction",
		Temporal: PersistentState, Evidence: EvidenceRequired,
		Description: "pg_stat_statements at capacity triggers eviction",
		BaseConfidence: 0.9, ChainID: ChainPGSSFillEviction,
		RemediationHook: HookPGSSReset,
	})

	// --- Chain 16 (Tier A): Dead tuple accumulation -> table bloat -> scan degradation ---
	addEdge(g, CausalEdge{
		FromNode: "dead_tuples", ToNode: "table_bloat",
		Temporal: PersistentState, Evidence: EvidenceRequired,
		Description: "Dead tuple accumulation causes table bloat",
		BaseConfidence: 0.85, ChainID: ChainDeadTupleBloatScan,
		RemediationHook: HookVacuumAggressive,
	})
	addEdge(g, CausalEdge{
		FromNode: "table_bloat", ToNode: "scan_degradation",
		Temporal: PersistentState, Evidence: EvidenceSupporting,
		Description: "Bloated tables degrade scan performance",
		BaseConfidence: 0.7, ChainID: ChainDeadTupleBloatScan,
	})

	// --- Chain 17 (Tier A): Wraparound risk -> aggressive vacuum -> disk I/O ---
	addEdge(g, CausalEdge{
		FromNode: "wraparound_risk", ToNode: "aggressive_vacuum",
		Temporal: PersistentState, Evidence: EvidenceRequired,
		Description: "Approaching wraparound triggers aggressive vacuum",
		BaseConfidence: 0.9, ChainID: ChainWraparoundVacuumIO,
		RemediationHook: HookWraparoundVacuum,
	})
	addEdge(g, CausalEdge{
		FromNode: "aggressive_vacuum", ToNode: "disk_io",
		MinLag: 0, MaxLag: 5 * time.Minute,
		Temporal: BoundedLag, Evidence: EvidenceRequired,
		Description: "Anti-wraparound vacuum generates heavy disk I/O",
		BaseConfidence: 0.8, ChainID: ChainWraparoundVacuumIO,
	})

	// --- Chain 18 (Tier A): Long tx blocking vacuum -> dead tuple growth ---
	addEdge(g, CausalEdge{
		FromNode: "long_tx_blocking", ToNode: "vacuum_blocked",
		Temporal: PersistentState, Evidence: EvidenceRequired,
		Description: "Long transaction prevents vacuum from cleaning dead tuples",
		BaseConfidence: 0.85, ChainID: ChainLongTxBlockVacuumGrowth,
		RemediationHook: HookKillLongTx,
	})
	addEdge(g, CausalEdge{
		FromNode: "vacuum_blocked", ToNode: "dead_tuple_growth",
		Temporal: PersistentState, Evidence: EvidenceRequired,
		Description: "Blocked vacuum causes dead tuple accumulation",
		BaseConfidence: 0.8, ChainID: ChainLongTxBlockVacuumGrowth,
	})

	// --- Chain 19 (Tier B): Settings change -> behavioral shift ---
	addEdge(g, CausalEdge{
		FromNode: "settings_change", ToNode: "behavioral_shift",
		Temporal: WhileEffective, Evidence: EvidenceRequired,
		Description: "Configuration change causes behavioral shift",
		BaseConfidence: 0.6, ChainID: ChainSettingsChangeBehavior,
	})

	// --- Chain 20 (Tier A): OS memory pressure -> OOM killer -> PG crash ---
	addEdge(g, CausalEdge{
		FromNode: "os_memory_pressure", ToNode: "oom_killer",
		Temporal: PersistentState, Evidence: EvidenceRequired,
		Description: "OS memory exhaustion activates OOM killer",
		BaseConfidence: 0.85, ChainID: ChainOSMemoryOOMCrash,
		RemediationHook: HookMemoryTuning,
	})
	addEdge(g, CausalEdge{
		FromNode: "oom_killer", ToNode: "pg_crash",
		MinLag: 0, MaxLag: 5 * time.Minute,
		Temporal: BoundedLag, Evidence: EvidenceSupporting,
		Description: "OOM killer terminates PostgreSQL process",
		BaseConfidence: 0.7, ChainID: ChainOSMemoryOOMCrash,
	})

	return g
}

func addNode(g *CausalGraph, id, name string, metricKeys []string, layer, symptom, mechanism string) {
	g.Nodes[id] = &CausalNode{
		ID:           id,
		Name:         name,
		MetricKeys:   metricKeys,
		Layer:        layer,
		SymptomKey:   symptom,
		MechanismKey: mechanism,
	}
}

func addEdge(g *CausalGraph, edge CausalEdge) {
	g.Edges = append(g.Edges, edge)
}
