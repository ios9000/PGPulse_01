package rca

import "time"

// TemporalSemantics defines how time relates cause to effect.
type TemporalSemantics int

const (
	// BoundedLag means the effect occurs within MinLag..MaxLag after the cause.
	BoundedLag TemporalSemantics = iota
	// PersistentState means the cause is an ongoing condition present before the trigger.
	PersistentState
	// WhileEffective means a configuration state active at incident time (Tier B, not implemented in M14_01).
	WhileEffective
)

// EvidenceRequirement defines whether evidence is mandatory for a branch to survive.
type EvidenceRequirement int

const (
	// EvidenceRequired kills the branch if no anomaly is found.
	EvidenceRequired EvidenceRequirement = iota
	// EvidenceSupporting reduces confidence if absent but does not kill the branch.
	EvidenceSupporting
)

// CausalNode represents a vertex in the causal graph.
type CausalNode struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	MetricKeys   []string `json:"metric_keys"`
	Layer        string   `json:"layer"`
	SymptomKey   string   `json:"symptom_key,omitempty"`
	MechanismKey string   `json:"mechanism_key,omitempty"`
}

// CausalEdge represents a directed edge in the causal graph (cause -> effect).
type CausalEdge struct {
	FromNode        string              `json:"from_node"`
	ToNode          string              `json:"to_node"`
	MinLag          time.Duration       `json:"-"`
	MaxLag          time.Duration       `json:"-"`
	MinLagSeconds   float64             `json:"min_lag_seconds"`
	MaxLagSeconds   float64             `json:"max_lag_seconds"`
	Temporal        TemporalSemantics   `json:"temporal"`
	Evidence        EvidenceRequirement `json:"evidence"`
	Description     string              `json:"description"`
	BaseConfidence  float64             `json:"base_confidence"`
	ChainID         string              `json:"chain_id"`
	RemediationHook string              `json:"remediation_hook,omitempty"`
}

// CausalGraph holds the full set of nodes and edges.
type CausalGraph struct {
	Nodes    map[string]*CausalNode
	Edges    []CausalEdge
	ChainIDs []string // all registered chain IDs
}

// IncomingEdges returns all edges pointing to the given nodeID.
func (g *CausalGraph) IncomingEdges(nodeID string) []CausalEdge {
	var result []CausalEdge
	for _, e := range g.Edges {
		if e.ToNode == nodeID {
			result = append(result, e)
		}
	}
	return result
}

// ChainsForTrigger returns chain IDs whose terminal (downstream) nodes
// contain a metric key matching the trigger metric.
func (g *CausalGraph) ChainsForTrigger(metricKey string) []string {
	// Find all nodes that reference this metric key.
	matchingNodes := make(map[string]bool)
	for id, node := range g.Nodes {
		for _, mk := range node.MetricKeys {
			if mk == metricKey {
				matchingNodes[id] = true
			}
		}
	}

	// Find edges whose ToNode is a matching node and collect their chain IDs.
	seen := make(map[string]bool)
	var chains []string
	for _, e := range g.Edges {
		if matchingNodes[e.ToNode] && !seen[e.ChainID] {
			seen[e.ChainID] = true
			chains = append(chains, e.ChainID)
		}
	}

	// Also check if any matching node is a terminal node in a chain
	// (i.e., only appears as ToNode, never as FromNode for that chain).
	for _, chainID := range g.ChainIDs {
		if seen[chainID] {
			continue
		}
		// Collect all edge nodes for this chain.
		for _, e := range g.Edges {
			if e.ChainID == chainID && matchingNodes[e.ToNode] {
				seen[chainID] = true
				chains = append(chains, chainID)
				break
			}
		}
	}

	return chains
}

// ReachableNodes returns all upstream nodes reachable from nodeID
// by traversing incoming edges, up to maxDepth levels.
func (g *CausalGraph) ReachableNodes(nodeID string, maxDepth int) []*CausalNode {
	visited := make(map[string]bool)
	var result []*CausalNode
	g.reachableWalk(nodeID, maxDepth, visited, &result)
	return result
}

func (g *CausalGraph) reachableWalk(nodeID string, depth int, visited map[string]bool, result *[]*CausalNode) {
	if depth <= 0 {
		return
	}
	for _, e := range g.Edges {
		if e.ToNode == nodeID && !visited[e.FromNode] {
			visited[e.FromNode] = true
			if node, ok := g.Nodes[e.FromNode]; ok {
				*result = append(*result, node)
			}
			g.reachableWalk(e.FromNode, depth-1, visited, result)
		}
	}
}

// MetricKeysForChains returns the deduplicated set of metric keys
// needed to evaluate the given chain IDs.
func (g *CausalGraph) MetricKeysForChains(chainIDs []string) []string {
	chainSet := make(map[string]bool, len(chainIDs))
	for _, id := range chainIDs {
		chainSet[id] = true
	}

	// Collect all node IDs referenced by edges in the target chains.
	nodeIDs := make(map[string]bool)
	for _, e := range g.Edges {
		if chainSet[e.ChainID] {
			nodeIDs[e.FromNode] = true
			nodeIDs[e.ToNode] = true
		}
	}

	// Collect metric keys from those nodes.
	seen := make(map[string]bool)
	var keys []string
	for id := range nodeIDs {
		node, ok := g.Nodes[id]
		if !ok {
			continue
		}
		for _, mk := range node.MetricKeys {
			if !seen[mk] {
				seen[mk] = true
				keys = append(keys, mk)
			}
		}
	}
	return keys
}

// EdgesForChain returns all edges belonging to the given chain ID.
func (g *CausalGraph) EdgesForChain(chainID string) []CausalEdge {
	var result []CausalEdge
	for _, e := range g.Edges {
		if e.ChainID == chainID {
			result = append(result, e)
		}
	}
	return result
}

// TerminalNodes returns the node IDs that appear as ToNode but never as
// FromNode within the edges of the given chain ID.
func (g *CausalGraph) TerminalNodes(chainID string) []string {
	toNodes := make(map[string]bool)
	fromNodes := make(map[string]bool)
	for _, e := range g.Edges {
		if e.ChainID == chainID {
			toNodes[e.ToNode] = true
			fromNodes[e.FromNode] = true
		}
	}
	var terminals []string
	for id := range toNodes {
		if !fromNodes[id] {
			terminals = append(terminals, id)
		}
	}
	return terminals
}
