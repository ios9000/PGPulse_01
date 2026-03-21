package rca

import (
	"strings"
	"testing"
)

func TestNewDefaultGraph(t *testing.T) {
	t.Parallel()
	g := NewDefaultGraph()

	// Must contain all 20 chain IDs.
	if len(g.ChainIDs) != 20 {
		t.Fatalf("expected 20 chain IDs, got %d", len(g.ChainIDs))
	}
	for _, id := range AllChainIDs {
		found := false
		for _, cid := range g.ChainIDs {
			if cid == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("chain ID %s missing from graph", id)
		}
	}

	// Reasonable node and edge counts.
	if len(g.Nodes) < 30 {
		t.Errorf("expected at least 30 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) < 20 {
		t.Errorf("expected at least 20 edges, got %d", len(g.Edges))
	}
}

func TestIncomingEdges(t *testing.T) {
	t.Parallel()
	g := NewDefaultGraph()

	// replication_lag should have incoming edges (from chain 1 and chain 3 at least).
	edges := g.IncomingEdges("replication_lag")
	if len(edges) == 0 {
		t.Fatal("expected incoming edges for replication_lag, got 0")
	}
	// At least one edge from disk_io (chain 1).
	foundDiskIO := false
	for _, e := range edges {
		if e.FromNode == "disk_io" {
			foundDiskIO = true
		}
	}
	if !foundDiskIO {
		t.Error("expected incoming edge from disk_io to replication_lag")
	}
}

func TestChainsForTrigger(t *testing.T) {
	t.Parallel()
	g := NewDefaultGraph()

	// pg.replication.lag.replay_bytes is in the replication_lag node and
	// replication_apply_delay node. Should match chains 1, 3, 4 at minimum.
	chains := g.ChainsForTrigger("pg.replication.lag.replay_bytes")
	if len(chains) == 0 {
		t.Fatal("expected at least one chain for pg.replication.lag.replay_bytes")
	}

	chainSet := make(map[string]bool)
	for _, c := range chains {
		chainSet[c] = true
	}

	// Chain 1 has replication_lag as terminal node.
	if !chainSet[ChainBulkWALCheckpointIOReplLag] {
		t.Errorf("expected chain %s for replication lag trigger", ChainBulkWALCheckpointIOReplLag)
	}
}

func TestChainsForTrigger_NoMatch(t *testing.T) {
	t.Parallel()
	g := NewDefaultGraph()

	chains := g.ChainsForTrigger("nonexistent.metric.key")
	if len(chains) != 0 {
		t.Errorf("expected no chains for nonexistent metric, got %d", len(chains))
	}
}

func TestReachableNodes(t *testing.T) {
	t.Parallel()
	g := NewDefaultGraph()

	// From replication_lag, going upstream, we should reach disk_io at depth 1.
	nodes := g.ReachableNodes("replication_lag", 1)
	if len(nodes) == 0 {
		t.Fatal("expected reachable nodes from replication_lag at depth 1")
	}
	foundDiskIO := false
	for _, n := range nodes {
		if n.ID == "disk_io" {
			foundDiskIO = true
		}
	}
	if !foundDiskIO {
		t.Error("expected disk_io to be reachable from replication_lag at depth 1")
	}

	// At depth 0 nothing should be returned.
	nodesD0 := g.ReachableNodes("replication_lag", 0)
	if len(nodesD0) != 0 {
		t.Errorf("expected 0 reachable nodes at depth 0, got %d", len(nodesD0))
	}

	// At depth 2 we should get more nodes than depth 1.
	nodesD2 := g.ReachableNodes("replication_lag", 2)
	if len(nodesD2) < len(nodes) {
		t.Errorf("expected depth 2 to yield >= depth 1 nodes; got %d vs %d", len(nodesD2), len(nodes))
	}
}

func TestMetricKeysForChains(t *testing.T) {
	t.Parallel()
	g := NewDefaultGraph()

	keys := g.MetricKeysForChains([]string{ChainBulkWALCheckpointIOReplLag})
	if len(keys) == 0 {
		t.Fatal("expected metric keys for chain 1")
	}

	// Verify deduplication: no duplicates.
	seen := make(map[string]bool)
	for _, k := range keys {
		if seen[k] {
			t.Errorf("duplicate metric key: %s", k)
		}
		seen[k] = true
	}

	// Should contain a replication lag metric.
	foundReplLag := false
	for _, k := range keys {
		if strings.Contains(k, "replication.lag") {
			foundReplLag = true
		}
	}
	if !foundReplLag {
		t.Error("expected replication lag metric key in chain 1 keys")
	}
}

func TestEdgesForChain(t *testing.T) {
	t.Parallel()
	g := NewDefaultGraph()

	// Chain 1 has 4 edges.
	edges := g.EdgesForChain(ChainBulkWALCheckpointIOReplLag)
	if len(edges) != 4 {
		t.Fatalf("expected 4 edges for chain 1, got %d", len(edges))
	}
}

func TestTerminalNodes(t *testing.T) {
	t.Parallel()
	g := NewDefaultGraph()

	terminals := g.TerminalNodes(ChainBulkWALCheckpointIOReplLag)
	if len(terminals) == 0 {
		t.Fatal("expected terminal nodes for chain 1")
	}
	// replication_lag should be a terminal for chain 1.
	found := false
	for _, t := range terminals {
		if t == "replication_lag" {
			found = true
		}
	}
	if !found {
		t.Error("expected replication_lag as terminal node for chain 1")
	}
}
