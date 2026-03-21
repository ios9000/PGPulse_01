package rca

import (
	"strings"
	"testing"
)

func TestAllChainsExist(t *testing.T) {
	t.Parallel()
	g := NewDefaultGraph()

	for _, chainID := range AllChainIDs {
		edges := g.EdgesForChain(chainID)
		if len(edges) == 0 {
			t.Errorf("chain %s has no edges in graph", chainID)
		}
	}
}

func TestChainEdgeIntegrity(t *testing.T) {
	t.Parallel()
	g := NewDefaultGraph()

	for _, edge := range g.Edges {
		if _, ok := g.Nodes[edge.FromNode]; !ok {
			t.Errorf("edge %s -> %s (chain %s): FromNode %q not in Nodes map",
				edge.FromNode, edge.ToNode, edge.ChainID, edge.FromNode)
		}
		if _, ok := g.Nodes[edge.ToNode]; !ok {
			t.Errorf("edge %s -> %s (chain %s): ToNode %q not in Nodes map",
				edge.FromNode, edge.ToNode, edge.ChainID, edge.ToNode)
		}
	}
}

func TestChainEdgeChainIDValid(t *testing.T) {
	t.Parallel()
	g := NewDefaultGraph()

	validChains := make(map[string]bool)
	for _, id := range AllChainIDs {
		validChains[id] = true
	}

	for _, edge := range g.Edges {
		if !validChains[edge.ChainID] {
			t.Errorf("edge %s -> %s has invalid ChainID %q", edge.FromNode, edge.ToNode, edge.ChainID)
		}
	}
}

func TestChainBaseConfidence(t *testing.T) {
	t.Parallel()
	g := NewDefaultGraph()

	for _, edge := range g.Edges {
		if edge.BaseConfidence <= 0.0 || edge.BaseConfidence > 1.0 {
			t.Errorf("edge %s -> %s (chain %s): BaseConfidence %.2f out of range (0,1]",
				edge.FromNode, edge.ToNode, edge.ChainID, edge.BaseConfidence)
		}
	}
}

func TestChainMetricKeys(t *testing.T) {
	t.Parallel()
	g := NewDefaultGraph()

	// Count nodes with empty MetricKeys — some Tier B nodes (network_issue,
	// settings_change, behavioral_shift) legitimately have empty keys.
	emptyKeyNodes := 0
	for id, node := range g.Nodes {
		if len(node.MetricKeys) == 0 {
			emptyKeyNodes++
			// These are expected to be empty.
			allowed := map[string]bool{
				"network_issue":    true,
				"settings_change":  true,
				"behavioral_shift": true,
			}
			if !allowed[id] {
				t.Errorf("node %s has empty MetricKeys but is not in the allowed list", id)
			}
			continue
		}
		for _, mk := range node.MetricKeys {
			if !strings.Contains(mk, ".") {
				t.Errorf("node %s has metric key %q that does not look like a real metric key (missing dot)", id, mk)
			}
		}
	}
}

func TestTierAChainCount(t *testing.T) {
	t.Parallel()

	tierACount := 0
	for _, id := range AllChainIDs {
		if TierForChain[id] == TierA {
			tierACount++
		}
	}
	if tierACount != 16 {
		t.Errorf("expected 16 Tier A chains, got %d", tierACount)
	}
}

func TestTierBChainCount(t *testing.T) {
	t.Parallel()

	tierBChains := []string{
		ChainNetworkWALRecvReplLag,
		ChainQueryRegressionCPULatency,
		ChainNewQueryResourceShift,
		ChainSettingsChangeBehavior,
	}

	tierBCount := 0
	for _, id := range AllChainIDs {
		if TierForChain[id] == TierB {
			tierBCount++
		}
	}
	if tierBCount != 4 {
		t.Errorf("expected 4 Tier B chains, got %d", tierBCount)
	}

	for _, id := range tierBChains {
		if TierForChain[id] != TierB {
			t.Errorf("expected chain %s to be Tier B, got %s", id, TierForChain[id])
		}
	}
}

func TestChainTierAHasRequiredEvidence(t *testing.T) {
	t.Parallel()
	g := NewDefaultGraph()

	for _, chainID := range AllChainIDs {
		if TierForChain[chainID] != TierA {
			continue
		}
		edges := g.EdgesForChain(chainID)
		hasRequired := false
		for _, edge := range edges {
			if edge.Evidence == EvidenceRequired {
				hasRequired = true
				break
			}
		}
		if !hasRequired {
			t.Errorf("Tier A chain %s has no EvidenceRequired edges", chainID)
		}
	}
}

func TestChainTemporalSemantics(t *testing.T) {
	t.Parallel()
	g := NewDefaultGraph()

	validTemporals := map[TemporalSemantics]bool{
		BoundedLag:      true,
		PersistentState: true,
		WhileEffective:  true,
	}

	for _, edge := range g.Edges {
		if !validTemporals[edge.Temporal] {
			t.Errorf("edge %s -> %s (chain %s): invalid Temporal value %d",
				edge.FromNode, edge.ToNode, edge.ChainID, edge.Temporal)
		}
	}
}

func TestChainEvidenceRequirements(t *testing.T) {
	t.Parallel()
	g := NewDefaultGraph()

	validEvidence := map[EvidenceRequirement]bool{
		EvidenceRequired:   true,
		EvidenceSupporting: true,
	}

	for _, edge := range g.Edges {
		if !validEvidence[edge.Evidence] {
			t.Errorf("edge %s -> %s (chain %s): invalid Evidence value %d",
				edge.FromNode, edge.ToNode, edge.ChainID, edge.Evidence)
		}
	}
}
