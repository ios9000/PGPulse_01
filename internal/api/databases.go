package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// --- response types ---

type databaseSummary struct {
	Name            string  `json:"name"`
	LargeObjCount   int64   `json:"large_object_count"`
	DeadTuples      int64   `json:"dead_tuples"`
	UnusedIndexes   int64   `json:"unused_indexes"`
	MaxBloatRatio   float64 `json:"max_bloat_ratio"`
	LastCollected   string  `json:"last_collected,omitempty"`
}

type tableMetric struct {
	Schema     string  `json:"schema"`
	Table      string  `json:"table"`
	TotalBytes float64 `json:"total_bytes"`
	TableBytes float64 `json:"table_bytes"`
	BloatRatio float64 `json:"bloat_ratio,omitempty"`
	WastedBytes float64 `json:"wasted_bytes,omitempty"`
}

type indexMetric struct {
	Schema      string  `json:"schema"`
	Table       string  `json:"table"`
	Index       string  `json:"index"`
	ScanCount   float64 `json:"scan_count"`
	TupRead     float64 `json:"tup_read,omitempty"`
	CacheHitPct float64 `json:"cache_hit_pct,omitempty"`
	Unused      bool    `json:"unused,omitempty"`
	UnusedBytes float64 `json:"unused_bytes,omitempty"`
	BloatRatio  float64 `json:"bloat_ratio,omitempty"`
	WastedBytes float64 `json:"wasted_bytes,omitempty"`
}

type vacuumMetric struct {
	Schema             string  `json:"schema"`
	Table              string  `json:"table"`
	DeadTuples         float64 `json:"dead_tuples"`
	DeadPct            float64 `json:"dead_pct"`
	AutovacuumAgeSec   float64 `json:"autovacuum_age_sec,omitempty"`
	AutoanalyzeAgeSec  float64 `json:"autoanalyze_age_sec,omitempty"`
}

type schemaMetric struct {
	Schema    string  `json:"schema"`
	SizeBytes float64 `json:"size_bytes"`
}

type sequenceMetric struct {
	Schema   string  `json:"schema"`
	Sequence string  `json:"sequence"`
	LastValue float64 `json:"last_value"`
}

type functionMetric struct {
	Schema      string  `json:"schema"`
	Function    string  `json:"function"`
	Calls       float64 `json:"calls"`
	TotalTimeMs float64 `json:"total_time_ms"`
	SelfTimeMs  float64 `json:"self_time_ms"`
}

type catalogMetric struct {
	Table     string  `json:"table"`
	SizeBytes float64 `json:"size_bytes"`
}

type databaseMetrics struct {
	DatabaseName        string           `json:"database_name"`
	CollectedAt         string           `json:"collected_at"`
	Tables              []tableMetric    `json:"tables"`
	Indexes             []indexMetric    `json:"indexes"`
	Vacuum              []vacuumMetric   `json:"vacuum"`
	Schemas             []schemaMetric   `json:"schemas"`
	Sequences           []sequenceMetric `json:"sequences"`
	Functions           []functionMetric `json:"functions"`
	Catalogs            []catalogMetric  `json:"catalogs"`
	LargeObjectCount    float64          `json:"large_object_count"`
	LargeObjectSizeBytes float64         `json:"large_object_size_bytes"`
	UnusedIndexCount    int              `json:"unused_index_count"`
	UnloggedCount       float64          `json:"unlogged_count"`
	PartitionCount      float64          `json:"partition_count"`
}

// handleListDatabases returns a summary of databases discovered for an instance.
func (s *APIServer) handleListDatabases(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")

	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	// Query all db.* metrics for this instance from the last 10 minutes.
	now := time.Now()
	points, err := s.store.Query(r.Context(), collector.MetricQuery{
		InstanceID: instanceID,
		Metric:     "pgpulse.db.",
		Start:      now.Add(-10 * time.Minute),
		End:        now,
		Limit:      10000,
	})
	if err != nil {
		s.logger.ErrorContext(r.Context(), "database list query failed",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to query database metrics")
		return
	}

	// Aggregate per database.
	type dbAgg struct {
		largeObjCount int64
		deadTuples    int64
		unusedIndexes int64
		maxBloat      float64
		lastSeen      time.Time
	}
	dbs := make(map[string]*dbAgg)

	for _, p := range points {
		dbName := p.Labels["database"]
		if dbName == "" {
			continue
		}
		agg, ok := dbs[dbName]
		if !ok {
			agg = &dbAgg{}
			dbs[dbName] = agg
		}
		if p.Timestamp.After(agg.lastSeen) {
			agg.lastSeen = p.Timestamp
		}
		switch p.Metric {
		case "pgpulse.db.large_objects.count":
			agg.largeObjCount += int64(p.Value)
		case "pgpulse.db.vacuum.dead_tuples":
			agg.deadTuples += int64(p.Value)
		case "pgpulse.db.index.unused":
			agg.unusedIndexes += int64(p.Value)
		case "pgpulse.db.table.bloat_ratio":
			if p.Value > agg.maxBloat {
				agg.maxBloat = p.Value
			}
		}
	}

	result := make([]databaseSummary, 0, len(dbs))
	for name, agg := range dbs {
		ds := databaseSummary{
			Name:          name,
			LargeObjCount: agg.largeObjCount,
			DeadTuples:    agg.deadTuples,
			UnusedIndexes: agg.unusedIndexes,
			MaxBloatRatio: agg.maxBloat,
		}
		if !agg.lastSeen.IsZero() {
			ds.LastCollected = agg.lastSeen.Format(time.RFC3339)
		}
		result = append(result, ds)
	}

	writeJSON(w, http.StatusOK, result)
}

// handleGetDatabaseMetrics returns full per-DB analysis metrics for a specific database.
func (s *APIServer) handleGetDatabaseMetrics(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	dbName := chi.URLParam(r, "dbname")

	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	// Query all db.* metrics for this instance + database from the last 10 minutes.
	now := time.Now()
	points, err := s.store.Query(r.Context(), collector.MetricQuery{
		InstanceID: instanceID,
		Metric:     "pgpulse.db.",
		Labels:     map[string]string{"database": dbName},
		Start:      now.Add(-10 * time.Minute),
		End:        now,
		Limit:      10000,
	})
	if err != nil {
		s.logger.ErrorContext(r.Context(), "database metrics query failed",
			"instance_id", instanceID, "database", dbName, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to query database metrics")
		return
	}

	resp := databaseMetrics{
		DatabaseName: dbName,
		Tables:       []tableMetric{},
		Indexes:      []indexMetric{},
		Vacuum:       []vacuumMetric{},
		Schemas:      []schemaMetric{},
		Sequences:    []sequenceMetric{},
		Functions:    []functionMetric{},
		Catalogs:     []catalogMetric{},
	}

	// Track latest timestamp for collected_at.
	var latestTime time.Time

	// Index maps for building composite objects.
	tableMap := make(map[string]*tableMetric)  // "schema.table"
	indexMap := make(map[string]*indexMetric)   // "schema.table.index"
	vacMap := make(map[string]*vacuumMetric)    // "schema.table"

	for _, p := range points {
		if p.Timestamp.After(latestTime) {
			latestTime = p.Timestamp
		}

		schema := p.Labels["schema"]
		table := p.Labels["table"]
		index := p.Labels["index"]

		switch p.Metric {
		// Table sizes
		case "pgpulse.db.table.total_bytes":
			key := schema + "." + table
			tm := getOrCreateTable(tableMap, key, schema, table)
			tm.TotalBytes = p.Value
		case"pgpulse.db.table.table_bytes":
			key := schema + "." + table
			tm := getOrCreateTable(tableMap, key, schema, table)
			tm.TableBytes = p.Value
		case"pgpulse.db.table.bloat_ratio":
			key := schema + "." + table
			tm := getOrCreateTable(tableMap, key, schema, table)
			tm.BloatRatio = p.Value
		case"pgpulse.db.table.wasted_bytes":
			key := schema + "." + table
			tm := getOrCreateTable(tableMap, key, schema, table)
			tm.WastedBytes = p.Value

		// Index metrics
		case"pgpulse.db.index.scan_count":
			key := schema + "." + table + "." + index
			im := getOrCreateIndex(indexMap, key, schema, table, index)
			im.ScanCount = p.Value
		case"pgpulse.db.index.tup_read":
			key := schema + "." + table + "." + index
			im := getOrCreateIndex(indexMap, key, schema, table, index)
			im.TupRead = p.Value
		case"pgpulse.db.index.cache_hit_pct":
			key := schema + "." + table + "." + index
			im := getOrCreateIndex(indexMap, key, schema, table, index)
			im.CacheHitPct = p.Value
		case"pgpulse.db.index.unused":
			key := schema + "." + table + "." + index
			im := getOrCreateIndex(indexMap, key, schema, table, index)
			im.Unused = true
		case"pgpulse.db.index.unused_bytes":
			key := schema + "." + table + "." + index
			im := getOrCreateIndex(indexMap, key, schema, table, index)
			im.UnusedBytes = p.Value
		case"pgpulse.db.index.bloat_ratio":
			key := schema + "." + table + "." + index
			im := getOrCreateIndex(indexMap, key, schema, table, index)
			im.BloatRatio = p.Value
		case"pgpulse.db.index.wasted_bytes":
			key := schema + "." + table + "." + index
			im := getOrCreateIndex(indexMap, key, schema, table, index)
			im.WastedBytes = p.Value

		// Vacuum
		case"pgpulse.db.vacuum.dead_tuples":
			key := schema + "." + table
			vm := getOrCreateVacuum(vacMap, key, schema, table)
			vm.DeadTuples = p.Value
		case"pgpulse.db.vacuum.dead_pct":
			key := schema + "." + table
			vm := getOrCreateVacuum(vacMap, key, schema, table)
			vm.DeadPct = p.Value
		case"pgpulse.db.vacuum.autovacuum_age_sec":
			key := schema + "." + table
			vm := getOrCreateVacuum(vacMap, key, schema, table)
			vm.AutovacuumAgeSec = p.Value
		case"pgpulse.db.vacuum.autoanalyze_age_sec":
			key := schema + "." + table
			vm := getOrCreateVacuum(vacMap, key, schema, table)
			vm.AutoanalyzeAgeSec = p.Value

		// Schema sizes
		case"pgpulse.db.schema.size_bytes":
			resp.Schemas = append(resp.Schemas, schemaMetric{
				Schema:    schema,
				SizeBytes: p.Value,
			})

		// Sequences
		case"pgpulse.db.sequence.last_value":
			seq := p.Labels["sequence"]
			resp.Sequences = append(resp.Sequences, sequenceMetric{
				Schema:   schema,
				Sequence: seq,
				LastValue: p.Value,
			})

		// Functions
		case"pgpulse.db.function.calls":
			fn := p.Labels["function"]
			resp.Functions = append(resp.Functions, functionMetric{
				Schema:   schema,
				Function: fn,
				Calls:    p.Value,
			})
		case"pgpulse.db.function.total_time_ms":
			// Handled via calls entry — skip to avoid duplicates.

		// Catalogs
		case"pgpulse.db.catalog.size_bytes":
			catTable := p.Labels["table"]
			resp.Catalogs = append(resp.Catalogs, catalogMetric{
				Table:     catTable,
				SizeBytes: p.Value,
			})

		// Scalar counts
		case"pgpulse.db.large_objects.count":
			resp.LargeObjectCount = p.Value
		case"pgpulse.db.large_objects.size_bytes":
			resp.LargeObjectSizeBytes = p.Value
		case"pgpulse.db.unlogged.count":
			resp.UnloggedCount = p.Value
		case"pgpulse.db.partition.count":
			resp.PartitionCount += p.Value
		}
	}

	// Flatten maps into slices.
	for _, tm := range tableMap {
		resp.Tables = append(resp.Tables, *tm)
	}
	for _, im := range indexMap {
		resp.Indexes = append(resp.Indexes, *im)
	}
	unusedCount := 0
	for _, im := range indexMap {
		if im.Unused {
			unusedCount++
		}
	}
	resp.UnusedIndexCount = unusedCount
	for _, vm := range vacMap {
		resp.Vacuum = append(resp.Vacuum, *vm)
	}

	if !latestTime.IsZero() {
		resp.CollectedAt = latestTime.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, resp)
}

func getOrCreateTable(m map[string]*tableMetric, key, schema, table string) *tableMetric {
	if tm, ok := m[key]; ok {
		return tm
	}
	tm := &tableMetric{Schema: schema, Table: table}
	m[key] = tm
	return tm
}

func getOrCreateIndex(m map[string]*indexMetric, key, schema, table, index string) *indexMetric {
	if im, ok := m[key]; ok {
		return im
	}
	im := &indexMetric{Schema: schema, Table: table, Index: index}
	m[key] = im
	return im
}

func getOrCreateVacuum(m map[string]*vacuumMetric, key, schema, table string) *vacuumMetric {
	if vm, ok := m[key]; ok {
		return vm
	}
	vm := &vacuumMetric{Schema: schema, Table: table}
	m[key] = vm
	return vm
}
