export type SafetyTier = 'diagnostic' | 'remediate' | 'dangerous' | 'external'
export type PlaybookStatus = 'draft' | 'stable' | 'deprecated'
export type RunStatus = 'in_progress' | 'completed' | 'abandoned' | 'escalated'
export type StepStatus =
  | 'pending'
  | 'running'
  | 'completed'
  | 'skipped'
  | 'failed'
  | 'awaiting_confirmation'
  | 'awaiting_approval'
  | 'pending_approval'
export type Verdict = 'green' | 'yellow' | 'red'

export interface InterpretationRule {
  column: string
  operator: string // '>', '<', '>=', '<=', '==', '!=', 'contains'
  value: string | number | boolean
  verdict: Verdict
  message: string
  scope: 'first' | 'any' | 'all'
}

export interface RowCountRule {
  operator: string
  value: number
  verdict: Verdict
  message: string
}

export interface InterpretationSpec {
  rules: InterpretationRule[]
  row_count_rules?: RowCountRule[]
  default_verdict: Verdict
  default_message: string
}

export interface BranchRule {
  condition_column?: string
  condition_operator?: string
  condition_value?: string | number | boolean
  on_verdict?: Verdict
  goto_step: number
  label: string
}

export interface PlaybookStep {
  id: number
  playbook_id: number
  step_order: number
  name: string
  description: string
  safety_tier: SafetyTier
  sql_template: string | null
  timeout_seconds: number
  result_interpretation: InterpretationSpec | null
  branch_rules: BranchRule[] | null
  manual_instructions: string | null
  escalation_contact: string | null
  requires_permission: string | null
}

export interface TriggerBindings {
  hooks?: string[]
  root_causes?: string[]
  metrics?: string[]
  adviser_rules?: string[]
}

export interface Playbook {
  id: number
  slug: string
  name: string
  description: string
  category: string
  version: number
  status: PlaybookStatus
  is_builtin: boolean
  trigger_bindings: TriggerBindings
  estimated_duration: string
  requires_permission: string | null
  steps: PlaybookStep[]
  created_at: string
  updated_at: string
}

export interface StepResult {
  columns: string[]
  rows: (string | number | boolean | null)[][]
  row_count: number
  truncated: boolean
}

export interface PlaybookRunStep {
  id: number
  run_id: number
  step_order: number
  status: StepStatus
  sql_executed: string | null
  result_json: StepResult | null
  verdict: Verdict | null
  verdict_message: string | null
  error: string | null
  confirmed_by: string | null
  executed_at: string | null
  completed_at: string | null
}

export interface PlaybookRun {
  id: number
  playbook_id: number
  playbook_name: string
  playbook_version: number
  instance_id: string
  started_by: string
  status: RunStatus
  current_step_order: number
  trigger_source: string | null
  trigger_id: string | null
  feedback_helpful: boolean | null
  feedback_resolved: boolean | null
  feedback_notes: string | null
  steps: PlaybookRunStep[]
  step_definitions: PlaybookStep[]
  started_at: string
  completed_at: string | null
}

export interface ResolverResult {
  playbook: Playbook | null
  match_reason: string
  match_value: string
}

export interface ExecuteStepResponse {
  step_result: PlaybookRunStep
  next_step: number | null
  run_status: RunStatus
  can_retry?: boolean
}
