package rca

// Symptom keys — what the DBA observes.
const (
	SymReplicationLagHigh   = "symptom.replication_lag_high"
	SymDiskIOSaturated      = "symptom.disk_io_saturated"
	SymConnectionExhaustion = "symptom.connection_exhaustion"
	SymQueryLatencyHigh     = "symptom.query_latency_high"
	SymDiskSpaceLow         = "symptom.disk_space_low"
	SymOOMRisk              = "symptom.oom_risk"
	SymPGSSEviction         = "symptom.pgss_eviction"
	SymScanDegradation      = "symptom.scan_degradation"
	SymDeadTupleGrowth      = "symptom.dead_tuple_growth"
	SymPGCrash              = "symptom.pg_crash"
	SymCPUSpike             = "symptom.cpu_spike"
	SymResourceShift        = "symptom.resource_shift"
	SymBehavioralShift      = "symptom.behavioral_shift"
)

// Mechanism keys — intermediate causal steps.
const (
	MechCheckpointStorm     = "mechanism.checkpoint_storm"
	MechWALSpike            = "mechanism.wal_spike"
	MechWALRetention        = "mechanism.wal_retention"
	MechBufferBackendWrites = "mechanism.buffer_backend_writes"
	MechLockContention      = "mechanism.lock_contention"
	MechBlockedQueries      = "mechanism.blocked_queries"
	MechAutovacuumStorm     = "mechanism.autovacuum_storm"
	MechTempFileSpike       = "mechanism.temp_file_spike"
	MechMVCCBloat           = "mechanism.mvcc_bloat"
	MechVacuumBlocked       = "mechanism.vacuum_blocked"
	MechMemoryPressure      = "mechanism.memory_pressure"
	MechSeqScans            = "mechanism.seq_scans"
	MechAggressiveVacuum    = "mechanism.aggressive_vacuum"
	MechWALRecvDisconnect   = "mechanism.wal_receiver_disconnect"
	MechOOMKiller           = "mechanism.oom_killer"
	MechTableBloat          = "mechanism.table_bloat"
)

// Root cause keys — the initiating factor.
const (
	RCBulkWorkload     = "root_cause.bulk_workload"
	RCInactiveSlot     = "root_cause.inactive_replication_slot"
	RCLongTransaction  = "root_cause.long_transaction"
	RCMissingIndex     = "root_cause.missing_index"
	RCWraparoundRisk   = "root_cause.wraparound_approaching"
	RCMemoryPressure   = "root_cause.memory_pressure"
	RCConfigChange     = "root_cause.config_change"
	RCNetworkIssue     = "root_cause.network_issue"
	RCQueryRegression  = "root_cause.query_regression"
	RCNewQuery         = "root_cause.new_query"
	RCBufferEviction   = "root_cause.buffer_eviction"
	RCConnectionSpike  = "root_cause.connection_spike"
	RCPGSSFill         = "root_cause.pgss_fill"
	RCDeadTuples       = "root_cause.dead_tuple_accumulation"
	RCLongTxBlocking   = "root_cause.long_tx_blocking_vacuum"
	RCOSMemoryPressure = "root_cause.os_memory_pressure"
)

// Chain IDs — stable identifiers for each causal chain.
const (
	ChainBulkWALCheckpointIOReplLag = "chain.bulk_wal_checkpoint_io_repllag"       // #1
	ChainInactiveSlotWALDisk        = "chain.inactive_slot_wal_disk"               // #2
	ChainNetworkWALRecvReplLag      = "chain.network_walrecv_repllag"              // #3
	ChainLongTxReplDelay            = "chain.longtx_repl_delay"                    // #4
	ChainWALSpikeCheckpointIO       = "chain.walspike_checkpoint_io"               // #5
	ChainAutovacuumDiskLatency      = "chain.autovacuum_disk_latency"              // #6
	ChainTempFileDiskQuery          = "chain.tempfile_disk_query"                  // #7
	ChainBufferEvictionBackendIO    = "chain.buffer_eviction_backend_io"           // #8
	ChainLockBlockedConnPileup      = "chain.lock_blocked_conn_pileup"            // #9
	ChainConnSpikeMemoryOOM        = "chain.conn_spike_memory_oom"                // #10
	ChainLongTxBloatConn            = "chain.longtx_bloat_conn"                   // #11
	ChainQueryRegressionCPULatency  = "chain.query_regression_cpu_latency"        // #12
	ChainNewQueryResourceShift      = "chain.new_query_resource_shift"            // #13
	ChainMissingIndexSeqScanIO      = "chain.missing_index_seqscan_io"            // #14
	ChainPGSSFillEviction           = "chain.pgss_fill_eviction"                  // #15
	ChainDeadTupleBloatScan         = "chain.dead_tuple_bloat_scan"               // #16
	ChainWraparoundVacuumIO         = "chain.wraparound_vacuum_io"                // #17
	ChainLongTxBlockVacuumGrowth    = "chain.longtx_block_vacuum_growth"          // #18
	ChainSettingsChangeBehavior     = "chain.settings_change_behavior"            // #19
	ChainOSMemoryOOMCrash           = "chain.os_memory_oom_crash"                 // #20
)

// AllChainIDs lists every chain ID in order.
var AllChainIDs = []string{
	ChainBulkWALCheckpointIOReplLag,
	ChainInactiveSlotWALDisk,
	ChainNetworkWALRecvReplLag,
	ChainLongTxReplDelay,
	ChainWALSpikeCheckpointIO,
	ChainAutovacuumDiskLatency,
	ChainTempFileDiskQuery,
	ChainBufferEvictionBackendIO,
	ChainLockBlockedConnPileup,
	ChainConnSpikeMemoryOOM,
	ChainLongTxBloatConn,
	ChainQueryRegressionCPULatency,
	ChainNewQueryResourceShift,
	ChainMissingIndexSeqScanIO,
	ChainPGSSFillEviction,
	ChainDeadTupleBloatScan,
	ChainWraparoundVacuumIO,
	ChainLongTxBlockVacuumGrowth,
	ChainSettingsChangeBehavior,
	ChainOSMemoryOOMCrash,
}

// Remediation hook IDs — link to adviser rules.
const (
	HookCheckpointTuning   = "remediation.checkpoint_completion_target"
	HookVacuumTuning       = "remediation.vacuum_cost_settings"
	HookConnectionPooling  = "remediation.connection_pooling"
	HookIndexCreation      = "remediation.create_missing_index"
	HookSlotCleanup        = "remediation.drop_inactive_slot"
	HookKillLongTx         = "remediation.kill_long_transaction"
	HookSharedBuffers      = "remediation.shared_buffers_tuning"
	HookTempFileWork       = "remediation.work_mem_tuning"
	HookPGSSReset          = "remediation.pgss_reset"
	HookVacuumAggressive   = "remediation.vacuum_aggressive"
	HookWraparoundVacuum   = "remediation.wraparound_vacuum"
	HookMemoryTuning       = "remediation.memory_tuning"
	HookLockInvestigation  = "remediation.lock_investigation"
	HookQueryOptimization  = "remediation.query_optimization"
	HookNetworkDiagnostics = "remediation.network_diagnostics"

	// M14_04: New hooks for guided remediation playbooks.
	HookWALArchive     = "remediation.wal_archive"
	HookReplicationLag = "remediation.replication_lag"
	HookDiskCapacity   = "remediation.disk_capacity"
)

// Tier classification.
const (
	TierA = "stable"       // fully supported in M14_01
	TierB = "experimental" // dependent on later integrations
)

// KnowledgeVersion tracks the chain definition version.
const KnowledgeVersion = "1.0.0"

// TierForChain classifies each chain as Tier A or Tier B.
var TierForChain = map[string]string{
	ChainBulkWALCheckpointIOReplLag: TierA,
	ChainInactiveSlotWALDisk:        TierA,
	ChainNetworkWALRecvReplLag:      TierB,
	ChainLongTxReplDelay:            TierA,
	ChainWALSpikeCheckpointIO:       TierA,
	ChainAutovacuumDiskLatency:      TierA,
	ChainTempFileDiskQuery:          TierA,
	ChainBufferEvictionBackendIO:    TierA,
	ChainLockBlockedConnPileup:      TierA,
	ChainConnSpikeMemoryOOM:         TierA,
	ChainLongTxBloatConn:            TierA,
	ChainQueryRegressionCPULatency:  TierB,
	ChainNewQueryResourceShift:      TierB,
	ChainMissingIndexSeqScanIO:      TierA,
	ChainPGSSFillEviction:           TierA,
	ChainDeadTupleBloatScan:         TierA,
	ChainWraparoundVacuumIO:         TierA,
	ChainLongTxBlockVacuumGrowth:    TierA,
	ChainSettingsChangeBehavior:     TierB,
	ChainOSMemoryOOMCrash:           TierA,
}
