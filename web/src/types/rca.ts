export interface RCAIncident {
  id: number
  instance_id: string
  trigger_metric: string
  trigger_value: number
  trigger_time: string
  trigger_kind: string
  analysis_window: { from: string; to: string }
  primary_chain?: RCACausalChainResult
  alternative_chain?: RCACausalChainResult
  timeline?: RCATimelineEvent[]
  summary: string
  confidence: number
  confidence_bucket: string
  quality: RCAQualityStatus
  remediation_hooks?: string[]
  auto_triggered: boolean
  chain_version: string
  anomaly_mode: string
  created_at: string
  review_status?: string
  review_comment?: string
}

export interface RCACausalChainResult {
  chain_id: string
  chain_name: string
  score: number
  root_cause_key: string
  events: RCATimelineEvent[]
}

export interface RCATimelineEvent {
  timestamp: string
  node_id: string
  node_name: string
  metric_key: string
  value: number
  baseline_val: number
  z_score: number
  strength: number
  layer: string
  role: string
  evidence: string
  description: string
  edge_desc: string
}

export interface RCAQualityStatus {
  telemetry_completeness: number
  anomaly_source_mode: string
  scope_limitations?: string[]
  unavailable_deps?: string[]
}

export interface RCACausalNode {
  id: string
  name: string
  metric_keys: string[]
  layer: string
  symptom_key: string
  mechanism_key: string
}

export interface RCACausalEdge {
  from_node: string
  to_node: string
  min_lag_seconds: number
  max_lag_seconds: number
  temporal: string
  evidence: string
  description: string
  base_confidence: number
  chain_id: string
  remediation_hook?: string
}

export interface RCACausalGraph {
  nodes: Record<string, RCACausalNode>
  edges: RCACausalEdge[]
  chain_ids: string[]
}
