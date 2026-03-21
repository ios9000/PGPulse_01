package rca

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// Engine is the core RCA correlation engine. It traverses the causal graph,
// detects anomalies, prunes branches, ranks chains, and builds incidents.
type Engine struct {
	graph       *CausalGraph
	anomaly     AnomalySource
	store       IncidentStore
	metricStore collector.MetricStore
	cfg         RCAConfig
	mu          sync.Mutex
}

// EngineOptions holds constructor parameters for Engine.
type EngineOptions struct {
	Graph       *CausalGraph
	Anomaly     AnomalySource
	Store       IncidentStore
	MetricStore collector.MetricStore
	Config      RCAConfig
}

// NewEngine creates a new RCA correlation engine.
func NewEngine(opts EngineOptions) *Engine {
	return &Engine{
		graph:       opts.Graph,
		anomaly:     opts.Anomaly,
		store:       opts.Store,
		metricStore: opts.MetricStore,
		cfg:         opts.Config,
	}
}

// AnalyzeRequest holds the trigger information for an RCA analysis.
type AnalyzeRequest struct {
	InstanceID    string
	TriggerMetric string
	TriggerValue  float64
	TriggerTime   time.Time
	WindowMinutes int    // 0 = use default from config
	TriggerKind   string // "alert" or "manual"
}

// Analyze performs the 9-step RCA algorithm.
func (e *Engine) Analyze(ctx context.Context, req AnalyzeRequest) (*Incident, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Step 1: Define window.
	lookback := e.cfg.LookbackWindow
	if req.WindowMinutes > 0 {
		lookback = time.Duration(req.WindowMinutes) * time.Minute
	}
	from := req.TriggerTime.Add(-lookback)
	to := req.TriggerTime
	if req.TriggerKind == "alert" {
		to = req.TriggerTime.Add(e.cfg.DeferredForwardTail)
	}
	window := TimeWindow{From: from, To: to}

	anomalyMode := AnomalyMode(e.anomaly)
	builder := NewIncidentBuilder(req, window, anomalyMode)

	// Step 2: Scope — select candidate chains.
	chainIDs := e.graph.ChainsForTrigger(req.TriggerMetric)

	// Filter to Tier A only.
	var tierAChains []string
	for _, id := range chainIDs {
		if TierForChain[id] == TierA {
			tierAChains = append(tierAChains, id)
		}
	}
	chainIDs = tierAChains

	if len(chainIDs) > e.cfg.MaxCandidateChains {
		chainIDs = chainIDs[:e.cfg.MaxCandidateChains]
	}

	// Step 3: Query — determine needed metrics.
	metricKeys := e.graph.MetricKeysForChains(chainIDs)
	if len(metricKeys) > e.cfg.MaxMetricsPerRun {
		slog.Warn("RCA: metric keys capped",
			"needed", len(metricKeys),
			"max", e.cfg.MaxMetricsPerRun)
		metricKeys = metricKeys[:e.cfg.MaxMetricsPerRun]
	}

	// Step 4: Detect — get anomalies for all needed metrics.
	jitter := 90 * time.Second // default medium jitter
	anomalyMap, err := e.anomaly.GetAnomalies(ctx, req.InstanceID, metricKeys, from, to, jitter)
	if err != nil {
		return nil, fmt.Errorf("rca anomaly detection: %w", err)
	}

	// Track telemetry completeness.
	metricsWithData := 0
	for _, key := range metricKeys {
		if _, ok := anomalyMap[key]; ok {
			metricsWithData++
		} else {
			// Check if we have any data at all (not just anomalies).
			pts, qErr := e.metricStore.Query(ctx, collector.MetricQuery{
				InstanceID: req.InstanceID,
				Metric:     key,
				Start:      from,
				End:        to,
				Limit:      1,
			})
			if qErr == nil && len(pts) > 0 {
				metricsWithData++
			}
		}
	}
	builder.SetTelemetry(len(metricKeys), metricsWithData)

	// Step 5: Traverse + Prune.
	for _, chainID := range chainIDs {
		chainResult := e.traverseChain(ctx, chainID, req.TriggerMetric, req.TriggerTime, window, anomalyMap)
		if chainResult != nil && chainResult.Score >= e.cfg.MinChainScore {
			builder.AddChain(*chainResult)
		}
	}

	// Steps 6-7: Rank + Build (handled by builder).
	incident := builder.Build()

	// Step 8: Store.
	if e.store != nil {
		id, storeErr := e.store.Create(ctx, incident)
		if storeErr != nil {
			slog.Error("RCA: failed to store incident", "err", storeErr)
		} else {
			incident.ID = id
		}
	}

	// Step 9: Return.
	return incident, nil
}

// traverseChain walks backward through the chain from its terminal node,
// evaluating each edge for evidence and pruning branches without required evidence.
func (e *Engine) traverseChain(
	_ context.Context,
	chainID string,
	triggerMetric string,
	triggerTime time.Time,
	window TimeWindow,
	anomalyMap map[string][]AnomalyEvent,
) *CausalChainResult {
	edges := e.graph.EdgesForChain(chainID)
	if len(edges) == 0 {
		return nil
	}

	// Find terminal nodes for this chain.
	terminals := e.graph.TerminalNodes(chainID)
	if len(terminals) == 0 {
		return nil
	}

	// Pick the terminal node that matches the trigger metric.
	startNode := ""
	for _, t := range terminals {
		node := e.graph.Nodes[t]
		if node == nil {
			continue
		}
		for _, mk := range node.MetricKeys {
			if mk == triggerMetric {
				startNode = t
				break
			}
		}
		if startNode != "" {
			break
		}
	}
	if startNode == "" {
		// Use first terminal as fallback.
		startNode = terminals[0]
	}

	var events []TimelineEvent
	totalScore := 0.0
	edgeCount := 0
	rootCauseKey := ""

	// Walk backward from the terminal node.
	success := e.walkBackward(startNode, chainID, triggerTime, window, anomalyMap,
		0, &events, &totalScore, &edgeCount, &rootCauseKey)

	if !success || edgeCount == 0 {
		return nil
	}

	// Average score across edges.
	avgScore := totalScore / float64(edgeCount)

	// Sort events by timestamp (earliest first).
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	// Determine chain name from first and last events.
	chainName := chainID
	if len(events) > 0 {
		chainName = events[0].NodeName + " -> " + events[len(events)-1].NodeName
	}

	return &CausalChainResult{
		ChainID:      chainID,
		ChainName:    chainName,
		Score:        avgScore,
		RootCauseKey: rootCauseKey,
		Events:       events,
	}
}

// walkBackward recursively walks incoming edges, evaluating evidence.
// Returns false if a required edge has no evidence (branch killed).
func (e *Engine) walkBackward(
	nodeID, chainID string,
	triggerTime time.Time,
	window TimeWindow,
	anomalyMap map[string][]AnomalyEvent,
	depth int,
	events *[]TimelineEvent,
	totalScore *float64,
	edgeCount *int,
	rootCauseKey *string,
) bool {
	if depth >= e.cfg.MaxTraversalDepth {
		return true
	}

	incoming := e.graph.IncomingEdges(nodeID)

	// Filter to edges belonging to this chain.
	var chainEdges []CausalEdge
	for _, edge := range incoming {
		if edge.ChainID == chainID {
			chainEdges = append(chainEdges, edge)
		}
	}

	if len(chainEdges) == 0 {
		// This is a root node. Record it if we have evidence.
		node := e.graph.Nodes[nodeID]
		if node == nil {
			return true
		}
		// Try to find anomaly evidence at the root node.
		anomaly := findStrongestAnomaly(anomalyMap, node.MetricKeys, window.From, window.To)
		if anomaly != nil {
			*events = append(*events, buildTimelineEvent(node, anomaly, nil, "root_cause"))
			*rootCauseKey = nodeRootCauseKey(nodeID)
		}
		return true
	}

	for _, edge := range chainEdges {
		score, event, found := e.evaluateEdge(edge, triggerTime, window, anomalyMap)

		if !found && edge.Evidence == EvidenceRequired {
			return false // branch killed
		}

		if event != nil {
			*events = append(*events, *event)
		}
		*totalScore += score
		*edgeCount++

		// Recurse upstream.
		if !e.walkBackward(edge.FromNode, chainID, triggerTime, window, anomalyMap,
			depth+1, events, totalScore, edgeCount, rootCauseKey) {
			return false
		}
	}

	return true
}

// evaluateEdge checks evidence for a single edge based on its temporal semantics.
func (e *Engine) evaluateEdge(
	edge CausalEdge,
	triggerTime time.Time,
	window TimeWindow,
	anomalyMap map[string][]AnomalyEvent,
) (score float64, event *TimelineEvent, found bool) {
	node := e.graph.Nodes[edge.FromNode]
	if node == nil {
		return 0, nil, false
	}

	switch edge.Temporal {
	case BoundedLag:
		jitter := e.collectionJitter(node)
		searchFrom := triggerTime.Add(-edge.MaxLag - jitter)
		searchTo := triggerTime.Add(-edge.MinLag + jitter)

		// Clamp search window to analysis window.
		if searchFrom.Before(window.From) {
			searchFrom = window.From
		}
		if searchTo.After(window.To) {
			searchTo = window.To
		}

		anomaly := findStrongestAnomaly(anomalyMap, node.MetricKeys, searchFrom, searchTo)
		if anomaly == nil {
			if edge.Evidence == EvidenceRequired {
				return 0, nil, false
			}
			return edge.BaseConfidence * 0.3, nil, true
		}
		proximity := temporalProximity(anomaly.Timestamp, triggerTime, edge.MinLag, edge.MaxLag)
		score = edge.BaseConfidence * anomaly.Strength * proximity
		ev := buildTimelineEvent(node, anomaly, &edge, nodeRole(node))
		return score, &ev, true

	case PersistentState:
		anomaly := findStrongestAnomaly(anomalyMap, node.MetricKeys, window.From, window.To)
		if anomaly == nil {
			if edge.Evidence == EvidenceRequired {
				return 0, nil, false
			}
			return edge.BaseConfidence * 0.3, nil, true
		}
		score = edge.BaseConfidence * anomaly.Strength
		ev := buildTimelineEvent(node, anomaly, &edge, nodeRole(node))
		return score, &ev, true

	case WhileEffective:
		// Not implemented in M14_01 — Tier B chains only.
		return 0, nil, false
	}

	return 0, nil, false
}

// temporalProximity rewards anomalies at the expected time lag.
func temporalProximity(anomalyTime, triggerTime time.Time, minLag, maxLag time.Duration) float64 {
	actualLag := triggerTime.Sub(anomalyTime)
	expectedCenter := (minLag + maxLag) / 2
	deviation := math.Abs(float64(actualLag - expectedCenter))
	maxDeviation := float64(maxLag-minLag) / 2
	if maxDeviation == 0 {
		return 1.0
	}
	proximity := 1.0 - (deviation / (maxDeviation * 1.5))
	if proximity < 0.2 {
		proximity = 0.2
	}
	if proximity > 1.0 {
		proximity = 1.0
	}
	return proximity
}

// collectionJitter returns the fuzzy window extension based on the node's
// metric collection frequency.
func (e *Engine) collectionJitter(node *CausalNode) time.Duration {
	group := nodeCollectionGroup(node)
	switch group {
	case "high":
		return 15 * time.Second
	case "medium":
		return 90 * time.Second
	case "low":
		return 450 * time.Second
	default:
		return 90 * time.Second
	}
}

// nodeCollectionGroup determines the collection frequency group for a node
// based on its metric keys.
func nodeCollectionGroup(node *CausalNode) string {
	if node == nil || len(node.MetricKeys) == 0 {
		return "medium"
	}

	// High frequency (10s): connections, locks, wait events, long transactions
	highPrefixes := []string{
		"pg.connections.", "pg.locks.", "pg.wait_events.",
		"pg.long_transactions.",
	}
	// Low frequency (300s): server info, extensions
	lowPrefixes := []string{
		"pg.server.", "pg.extensions.",
	}

	for _, mk := range node.MetricKeys {
		for _, p := range highPrefixes {
			if len(mk) >= len(p) && mk[:len(p)] == p {
				return "high"
			}
		}
		for _, p := range lowPrefixes {
			if len(mk) >= len(p) && mk[:len(p)] == p {
				return "low"
			}
		}
	}
	return "medium"
}

// findStrongestAnomaly finds the anomaly with the highest strength
// for any of the given metric keys within the time window.
func findStrongestAnomaly(
	anomalyMap map[string][]AnomalyEvent,
	metricKeys []string,
	from, to time.Time,
) *AnomalyEvent {
	var best *AnomalyEvent
	for _, key := range metricKeys {
		events, ok := anomalyMap[key]
		if !ok {
			continue
		}
		for i := range events {
			ev := &events[i]
			if ev.Timestamp.Before(from) || ev.Timestamp.After(to) {
				continue
			}
			if best == nil || ev.Strength > best.Strength {
				best = ev
			}
		}
	}
	return best
}

// buildTimelineEvent constructs a TimelineEvent from a node and anomaly.
func buildTimelineEvent(node *CausalNode, anomaly *AnomalyEvent, edge *CausalEdge, role string) TimelineEvent {
	ev := TimelineEvent{
		Timestamp:   anomaly.Timestamp,
		NodeID:      node.ID,
		NodeName:    node.Name,
		MetricKey:   anomaly.MetricKey,
		Value:       anomaly.Value,
		BaselineVal: anomaly.BaselineVal,
		ZScore:      anomaly.ZScore,
		Strength:    anomaly.Strength,
		Layer:       node.Layer,
		Role:        role,
		Evidence:    "required",
		Description: fmt.Sprintf("%s anomaly detected (value=%.2f, baseline=%.2f)", node.Name, anomaly.Value, anomaly.BaselineVal),
	}
	if edge != nil {
		ev.EdgeDesc = edge.Description
		if edge.Evidence == EvidenceSupporting {
			ev.Evidence = "supporting"
		}
	}
	return ev
}

// nodeRole determines the role label for a node in the timeline.
func nodeRole(node *CausalNode) string {
	if node.SymptomKey != "" {
		return "symptom"
	}
	if node.MechanismKey != "" {
		return "intermediate"
	}
	return "root_cause"
}

// nodeRootCauseKey maps node IDs to ontology root cause keys.
func nodeRootCauseKey(nodeID string) string {
	mapping := map[string]string{
		"bulk_workload":      RCBulkWorkload,
		"inactive_slot":      RCInactiveSlot,
		"long_tx_primary":    RCLongTransaction,
		"long_tx":            RCLongTransaction,
		"long_tx_blocking":   RCLongTxBlocking,
		"missing_index":      RCMissingIndex,
		"wraparound_risk":    RCWraparoundRisk,
		"connection_spike":   RCConnectionSpike,
		"os_memory_pressure": RCOSMemoryPressure,
		"buffer_eviction":    RCBufferEviction,
		"lock_contention":    RCLongTransaction,
		"autovacuum_storm":   RCDeadTuples,
		"temp_file_spike":    RCBulkWorkload,
		"dead_tuples":        RCDeadTuples,
		"pgss_fill":          RCPGSSFill,
		"wal_generation":     RCBulkWorkload,
		"network_issue":      RCNetworkIssue,
		"query_regression":   RCQueryRegression,
		"new_query":          RCNewQuery,
		"settings_change":    RCConfigChange,
	}
	if key, ok := mapping[nodeID]; ok {
		return key
	}
	return ""
}

// Graph returns the engine's causal graph (for API serialization).
func (e *Engine) Graph() *CausalGraph {
	return e.graph
}
