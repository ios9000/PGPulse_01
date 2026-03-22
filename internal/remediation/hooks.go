package remediation

import "github.com/ios9000/PGPulse_01/internal/rca"

// HookToRuleID maps RCA ontology hook constants to the semantically matching
// remediation rule ID. Empty string means no matching rule.
var HookToRuleID = map[string]string{
	rca.HookCheckpointTuning:   "rem_wraparound_warn",    // checkpoint tuning relates to WAL/wraparound issues
	rca.HookVacuumTuning:       "rem_bloat_high",         // vacuum cost settings relate to bloat management
	rca.HookConnectionPooling:  "rem_conn_high",          // connection pooling relates to connection utilization
	rca.HookIndexCreation:      "",                       // no direct rule for missing indexes yet
	rca.HookSlotCleanup:        "rem_repl_slot_inactive", // slot cleanup matches inactive slot rule
	rca.HookKillLongTx:         "rem_long_txn_warn",      // kill long tx matches long transaction warning
	rca.HookSharedBuffers:      "rem_cache_low",          // shared buffers tuning relates to cache hit ratio
	rca.HookTempFileWork:       "",                       // no direct rule for work_mem/temp files yet
	rca.HookPGSSReset:          "rem_pgss_fill",          // pgss reset matches pgss fill rule
	rca.HookVacuumAggressive:   "rem_bloat_extreme",      // aggressive vacuum for extreme bloat
	rca.HookWraparoundVacuum:   "rem_wraparound_crit",    // wraparound vacuum matches critical wraparound
	rca.HookMemoryTuning:       "rem_mem_pressure",       // memory tuning matches OS memory pressure
	rca.HookLockInvestigation:  "rem_locks_blocking",     // lock investigation matches blocking locks
	rca.HookQueryOptimization:  "",                       // no direct rule for query optimization yet
	rca.HookNetworkDiagnostics: "",                       // no direct rule for network diagnostics
}
