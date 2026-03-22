package rca

import (
	"fmt"
	"sort"
	"time"
)

// Incident represents a completed RCA analysis result.
type Incident struct {
	ID               int64              `json:"id"`
	InstanceID       string             `json:"instance_id"`
	TriggerMetric    string             `json:"trigger_metric"`
	TriggerValue     float64            `json:"trigger_value"`
	TriggerTime      time.Time          `json:"trigger_time"`
	TriggerKind      string             `json:"trigger_kind"` // "alert" or "manual"
	AnalysisWindow   TimeWindow         `json:"analysis_window"`
	PrimaryChain     *CausalChainResult `json:"primary_chain,omitempty"`
	AlternativeChain *CausalChainResult `json:"alternative_chain,omitempty"`
	Timeline         []TimelineEvent    `json:"timeline"`
	Summary          string             `json:"summary"`
	Confidence       float64            `json:"confidence"`
	ConfidenceBucket string             `json:"confidence_bucket"` // "high", "medium", "low"
	Quality          QualityStatus      `json:"quality"`
	RemediationHooks []string           `json:"remediation_hooks,omitempty"`
	AutoTriggered    bool               `json:"auto_triggered"`
	ChainVersion     string             `json:"chain_version"`
	AnomalyMode      string             `json:"anomaly_mode"` // "ml" or "threshold"
	CreatedAt        time.Time          `json:"created_at"`
	// Future-ready (nullable, not populated in M14_01).
	ReviewStatus  *string    `json:"review_status,omitempty"`
	ReviewedBy    *string    `json:"reviewed_by,omitempty"`
	ReviewedAt    *time.Time `json:"reviewed_at,omitempty"`
	ReviewComment *string    `json:"review_comment,omitempty"`
}

// CausalChainResult holds the traversal result for a single causal chain.
type CausalChainResult struct {
	ChainID      string          `json:"chain_id"`
	ChainName    string          `json:"chain_name"`
	Score        float64         `json:"score"`
	RootCauseKey string          `json:"root_cause_key"`
	Events       []TimelineEvent `json:"events"`
}

// TimelineEvent is a single data point in the RCA timeline.
type TimelineEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	NodeID      string    `json:"node_id"`
	NodeName    string    `json:"node_name"`
	MetricKey   string    `json:"metric_key"`
	Value       float64   `json:"value"`
	BaselineVal float64   `json:"baseline_val"`
	ZScore      float64   `json:"z_score"`
	Strength    float64   `json:"strength"`
	Layer       string    `json:"layer"`
	Role        string    `json:"role"` // "root_cause", "intermediate", "symptom"
	Evidence    string    `json:"evidence"`
	Description string    `json:"description"`
	EdgeDesc    string    `json:"edge_desc"`
}

// QualityStatus captures data quality and completeness of the analysis.
type QualityStatus struct {
	TelemetryCompleteness float64  `json:"telemetry_completeness"`
	AnomalySourceMode     string   `json:"anomaly_source_mode"`
	ScopeLimitations      []string `json:"scope_limitations,omitempty"`
	UnavailableDeps       []string `json:"unavailable_deps,omitempty"`
}

// TimeWindow defines a time range for analysis.
type TimeWindow struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// IncidentBuilder constructs an Incident from traversal results.
type IncidentBuilder struct {
	req            AnalyzeRequest
	window         TimeWindow
	chains         []CausalChainResult
	anomalyMode    string
	metricsNeeded  int
	metricsWithData int
}

// NewIncidentBuilder creates a builder for an incident.
func NewIncidentBuilder(req AnalyzeRequest, window TimeWindow, anomalyMode string) *IncidentBuilder {
	return &IncidentBuilder{
		req:         req,
		window:      window,
		anomalyMode: anomalyMode,
	}
}

// SetTelemetry records how many of the needed metrics had data.
func (b *IncidentBuilder) SetTelemetry(needed, withData int) {
	b.metricsNeeded = needed
	b.metricsWithData = withData
}

// AddChain adds a scored chain result to the builder.
func (b *IncidentBuilder) AddChain(chain CausalChainResult) {
	b.chains = append(b.chains, chain)
}

// Build constructs the final Incident.
func (b *IncidentBuilder) Build() *Incident {
	// Sort chains by score descending.
	sort.Slice(b.chains, func(i, j int) bool {
		return b.chains[i].Score > b.chains[j].Score
	})

	var primary *CausalChainResult
	var alt *CausalChainResult
	if len(b.chains) > 0 {
		c := b.chains[0]
		primary = &c
	}
	if len(b.chains) > 1 {
		// Include alternative if within 20% of primary score.
		if b.chains[1].Score >= b.chains[0].Score*0.8 {
			c := b.chains[1]
			alt = &c
		}
	}

	confidence := 0.0
	if primary != nil {
		confidence = primary.Score
	}

	timeline := b.buildTimeline(primary, alt)
	summary := generateSummary(primary, confidence)
	hooks := collectHooks(primary)

	completeness := 0.0
	if b.metricsNeeded > 0 {
		completeness = float64(b.metricsWithData) / float64(b.metricsNeeded)
	}

	return &Incident{
		InstanceID:    b.req.InstanceID,
		TriggerMetric: b.req.TriggerMetric,
		TriggerValue:  b.req.TriggerValue,
		TriggerTime:   b.req.TriggerTime,
		TriggerKind:   b.req.TriggerKind,
		AnalysisWindow: b.window,
		PrimaryChain:     primary,
		AlternativeChain: alt,
		Timeline:         timeline,
		Summary:          summary,
		Confidence:       confidence,
		ConfidenceBucket: bucketizeConfidence(confidence),
		Quality: QualityStatus{
			TelemetryCompleteness: completeness,
			AnomalySourceMode:     b.anomalyMode,
			ScopeLimitations:      []string{"single-instance only"},
		},
		RemediationHooks: hooks,
		AutoTriggered:    b.req.TriggerKind == "alert",
		ChainVersion:     KnowledgeVersion,
		AnomalyMode:      b.anomalyMode,
		CreatedAt:        time.Now(),
	}
}

// buildTimeline merges events from primary and alternative chains, sorted by timestamp.
func (b *IncidentBuilder) buildTimeline(primary, alt *CausalChainResult) []TimelineEvent {
	seen := make(map[string]bool)
	var events []TimelineEvent

	addEvents := func(chain *CausalChainResult) {
		if chain == nil {
			return
		}
		for _, e := range chain.Events {
			key := e.NodeID + ":" + e.MetricKey + ":" + e.Timestamp.Format(time.RFC3339Nano)
			if !seen[key] {
				seen[key] = true
				events = append(events, e)
			}
		}
	}

	addEvents(primary)
	addEvents(alt)

	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	return events
}

// generateSummary produces a qualified-language summary based on confidence.
func generateSummary(chain *CausalChainResult, confidence float64) string {
	if chain == nil || len(chain.Events) == 0 {
		return "No probable causal chain identified. Manual investigation recommended."
	}

	qualifier := "Likely caused by"
	if confidence < 0.4 {
		qualifier = "Possibly related to"
	} else if confidence > 0.8 {
		qualifier = "Strongly consistent with"
	}

	rootEvent := chain.Events[0]
	symptomEvent := chain.Events[len(chain.Events)-1]

	// Build direction verb based on whether the value rose or fell relative to baseline.
	rootVerb := "spiked to"
	if rootEvent.BaselineVal > 0 && rootEvent.Value < rootEvent.BaselineVal {
		rootVerb = "dropped to"
	}

	return fmt.Sprintf("%s %s. %s %s %.2f (baseline: %.2f) at %s, leading to %s (value: %.2f) at %s.",
		qualifier,
		rootEvent.Description,
		rootEvent.NodeName,
		rootVerb,
		rootEvent.Value,
		rootEvent.BaselineVal,
		rootEvent.Timestamp.Format("15:04:05"),
		symptomEvent.NodeName,
		symptomEvent.Value,
		symptomEvent.Timestamp.Format("15:04:05"),
	)
}

// bucketizeConfidence maps a confidence score to a bucket string.
func bucketizeConfidence(confidence float64) string {
	switch {
	case confidence > 0.7:
		return "high"
	case confidence >= 0.4:
		return "medium"
	default:
		return "low"
	}
}

// collectHooks gathers unique remediation hook IDs from the chain's events.
// It maps timeline events back to the causal graph via edge descriptions.
// This is supplemented by the engine's fireRemediationHooks for actual upserts.
func collectHooks(_ *CausalChainResult) []string {
	// Hook collection is handled by the engine's fireRemediationHooks
	// which processes edges directly. Return nil here.
	return nil
}
