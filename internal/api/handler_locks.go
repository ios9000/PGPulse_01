package api

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"
)

// LockTreeSummary contains aggregate statistics about the blocking tree.
type LockTreeSummary struct {
	RootBlockers int `json:"root_blockers"`
	TotalBlocked int `json:"total_blocked"`
	MaxDepth     int `json:"max_depth"`
}

// LockEntry describes a single process involved in a blocking chain.
type LockEntry struct {
	PID             int     `json:"pid"`
	Depth           int     `json:"depth"`
	Usename         string  `json:"usename"`
	Datname         string  `json:"datname"`
	State           string  `json:"state"`
	WaitEventType   *string `json:"wait_event_type"`
	WaitEvent       *string `json:"wait_event"`
	DurationSeconds float64 `json:"duration_seconds"`
	Query           string  `json:"query"`
	BlockedByCount  int     `json:"blocked_by_count"`
	BlockingCount   int     `json:"blocking_count"`
	IsRoot          bool    `json:"is_root"`
	ParentPID       *int    `json:"parent_pid"`
}

// LockTreeResponse wraps the blocking tree summary and entries.
type LockTreeResponse struct {
	Summary LockTreeSummary `json:"summary"`
	Locks   []LockEntry     `json:"locks"`
}

// RawLockEntry holds a row from pg_stat_activity before tree building.
// Exported so the tree-building logic can be unit-tested without a database.
type RawLockEntry struct {
	PID             int
	Usename         string
	Datname         string
	State           string
	WaitEventType   *string
	WaitEvent       *string
	DurationSeconds float64
	Query           string
	BlockingPIDs    []int
}

// handleLockTree returns the blocking lock tree for a monitored instance.
func (s *APIServer) handleLockTree(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")

	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.connProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "not_available",
			"instance connection provider not configured")
		return
	}

	conn, err := s.connProvider.ConnFor(r.Context(), instanceID)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get connection for lock tree",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusBadGateway, "connection_error",
			"failed to connect to instance")
		return
	}
	defer func() { _ = conn.Close(r.Context()) }()

	if _, err := conn.Exec(r.Context(), "SET LOCAL statement_timeout = '5s'"); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to set statement_timeout",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to configure connection")
		return
	}

	rows, err := conn.Query(r.Context(), `SELECT
    sa.pid,
    sa.usename,
    sa.datname,
    sa.state,
    sa.wait_event_type,
    sa.wait_event,
    EXTRACT(EPOCH FROM (now() - sa.xact_start))::float8 AS duration_seconds,
    LEFT(sa.query, 200) AS query,
    pg_blocking_pids(sa.pid) AS blocking_pids
FROM pg_stat_activity sa
WHERE sa.pid != pg_backend_pid()
  AND sa.state IS NOT NULL`)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to query lock tree",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to query lock tree")
		return
	}
	defer rows.Close()

	var raw []RawLockEntry
	for rows.Next() {
		var entry RawLockEntry
		var duration *float64
		var blockingPIDs []int32
		if err := rows.Scan(
			&entry.PID, &entry.Usename, &entry.Datname, &entry.State,
			&entry.WaitEventType, &entry.WaitEvent,
			&duration, &entry.Query, &blockingPIDs,
		); err != nil {
			s.logger.ErrorContext(r.Context(), "failed to scan lock tree row",
				"instance_id", instanceID, "error", err)
			continue
		}
		if duration != nil {
			entry.DurationSeconds = *duration
		}
		for _, bp := range blockingPIDs {
			entry.BlockingPIDs = append(entry.BlockingPIDs, int(bp))
		}
		raw = append(raw, entry)
	}
	if err := rows.Err(); err != nil {
		s.logger.ErrorContext(r.Context(), "lock tree rows error",
			"instance_id", instanceID, "error", err)
	}

	result := BuildLockTree(raw)

	writeJSON(w, http.StatusOK, Envelope{Data: result})
}

// BuildLockTree constructs the blocking tree from raw pg_stat_activity rows
// using BFS. This is a pure function with no database dependency, making it
// directly unit-testable.
func BuildLockTree(raw []RawLockEntry) LockTreeResponse {
	byPID := map[int]*RawLockEntry{}
	for i := range raw {
		byPID[raw[i].PID] = &raw[i]
	}

	// Build blockedBy map (pid -> list of pids that block it) and
	// reverse map (pid -> list of pids it blocks).
	blockedBy := map[int][]int{}
	blocks := map[int][]int{}

	for _, entry := range raw {
		for _, bp := range entry.BlockingPIDs {
			blockedBy[entry.PID] = append(blockedBy[entry.PID], bp)
			blocks[bp] = append(blocks[bp], entry.PID)
		}
	}

	// Determine which PIDs are involved in any blocking chain.
	involved := map[int]bool{}
	for pid := range blockedBy {
		involved[pid] = true
		for _, bp := range blockedBy[pid] {
			involved[bp] = true
		}
	}
	for pid := range blocks {
		involved[pid] = true
	}

	// Find root blockers: involved PIDs that are not blocked by anyone.
	var roots []int
	for pid := range involved {
		if len(blockedBy[pid]) == 0 {
			roots = append(roots, pid)
		}
	}
	sort.Ints(roots)

	// BFS from roots to assign depth and parent.
	type bfsItem struct {
		pid    int
		depth  int
		parent *int
	}

	entries := []LockEntry{}
	visited := map[int]bool{}
	queue := []bfsItem{}

	for _, rp := range roots {
		queue = append(queue, bfsItem{pid: rp, depth: 0, parent: nil})
	}

	maxDepth := 0
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if visited[item.pid] {
			continue
		}
		visited[item.pid] = true

		if item.depth > maxDepth {
			maxDepth = item.depth
		}

		row, ok := byPID[item.pid]
		if !ok {
			continue
		}

		entry := LockEntry{
			PID:             row.PID,
			Depth:           item.depth,
			Usename:         row.Usename,
			Datname:         row.Datname,
			State:           row.State,
			WaitEventType:   row.WaitEventType,
			WaitEvent:       row.WaitEvent,
			DurationSeconds: row.DurationSeconds,
			Query:           row.Query,
			BlockedByCount:  len(blockedBy[row.PID]),
			BlockingCount:   len(blocks[row.PID]),
			IsRoot:          item.depth == 0,
			ParentPID:       item.parent,
		}
		entries = append(entries, entry)

		// Enqueue children (PIDs blocked by this one), sorted for deterministic output.
		children := blocks[item.pid]
		sort.Ints(children)
		parentPID := item.pid
		for _, child := range children {
			if !visited[child] {
				pp := parentPID // capture for pointer
				queue = append(queue, bfsItem{pid: child, depth: item.depth + 1, parent: &pp})
			}
		}
	}

	// Sort result by depth, then PID.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Depth != entries[j].Depth {
			return entries[i].Depth < entries[j].Depth
		}
		return entries[i].PID < entries[j].PID
	})

	totalBlocked := 0
	for _, e := range entries {
		if !e.IsRoot {
			totalBlocked++
		}
	}

	return LockTreeResponse{
		Summary: LockTreeSummary{
			RootBlockers: len(roots),
			TotalBlocked: totalBlocked,
			MaxDepth:     maxDepth,
		},
		Locks: entries,
	}
}
