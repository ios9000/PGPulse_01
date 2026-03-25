import { useState, useEffect } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { ArrowLeft, Save } from 'lucide-react'
import { Spinner } from '@/components/ui/Spinner'
import { StepBuilder } from '@/components/playbook/StepBuilder'
import { usePlaybook, useCreatePlaybook, useUpdatePlaybook } from '@/hooks/usePlaybooks'
import { toast } from '@/stores/toastStore'
import type { PlaybookStep, TriggerBindings } from '@/types/playbook'

type DraftStep = Omit<PlaybookStep, 'id' | 'playbook_id'>

const CATEGORY_OPTIONS = [
  'replication',
  'storage',
  'connections',
  'locks',
  'vacuum',
  'performance',
  'configuration',
  'general',
]

const PERMISSION_OPTIONS = [
  { value: 'view_all', label: 'View All (any role)' },
  { value: 'alert_management', label: 'Alert Management' },
  { value: 'instance_management', label: 'Instance Management' },
  { value: 'user_management', label: 'User Management' },
]

function slugify(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
}

export function PlaybookEditor() {
  const { playbookId } = useParams<{ playbookId: string }>()
  const isNew = playbookId === 'new'
  const numericId = isNew ? undefined : parseInt(playbookId ?? '', 10)
  const navigate = useNavigate()
  const { data: existing, isLoading } = usePlaybook(numericId)
  const createPb = useCreatePlaybook()
  const updatePb = useUpdatePlaybook()

  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [description, setDescription] = useState('')
  const [category, setCategory] = useState('general')
  const [estimatedDuration, setEstimatedDuration] = useState('')
  const [requiresPermission, setRequiresPermission] = useState('view_all')
  const [steps, setSteps] = useState<DraftStep[]>([])
  const [triggerBindings, setTriggerBindings] = useState<TriggerBindings>({
    hooks: [],
    root_causes: [],
    metrics: [],
    adviser_rules: [],
  })

  // Populate from existing
  useEffect(() => {
    if (existing) {
      setName(existing.name)
      setSlug(existing.slug)
      setDescription(existing.description)
      setCategory(existing.category)
      setEstimatedDuration(existing.estimated_duration ?? '')
      setRequiresPermission(existing.requires_permission ?? 'view_all')
      setSteps(
        (existing.steps ?? []).map((s) => ({
          step_order: s.step_order,
          name: s.name,
          description: s.description,
          safety_tier: s.safety_tier,
          sql_template: s.sql_template,
          timeout_seconds: s.timeout_seconds,
          result_interpretation: s.result_interpretation,
          branch_rules: s.branch_rules,
          manual_instructions: s.manual_instructions,
          escalation_contact: s.escalation_contact,
          requires_permission: s.requires_permission,
        })),
      )
      setTriggerBindings(existing.trigger_bindings ?? {})
    }
  }, [existing])

  // Auto-slug from name for new playbooks
  useEffect(() => {
    if (isNew) {
      setSlug(slugify(name))
    }
  }, [name, isNew])

  if (!isNew && isLoading) {
    return (
      <div className="flex justify-center py-12">
        <Spinner size="lg" />
      </div>
    )
  }

  const handleSave = async () => {
    if (!name.trim() || !slug.trim()) {
      toast.error('Name and slug are required')
      return
    }

    const payload = {
      name,
      slug,
      description,
      category,
      estimated_duration: estimatedDuration || undefined,
      requires_permission: requiresPermission,
      trigger_bindings: triggerBindings,
      steps: steps.map((s) => ({
        ...s,
        sql_template: s.sql_template || null,
      })),
    }

    try {
      if (isNew) {
        const created = await createPb.mutateAsync(payload as never)
        toast.success('Playbook created')
        navigate(`/playbooks/${created.id}`)
      } else if (numericId) {
        await updatePb.mutateAsync({ id: numericId, playbook: payload as never })
        toast.success('Playbook updated (reset to draft)')
        navigate(`/playbooks/${numericId}`)
      }
    } catch {
      toast.error('Failed to save playbook')
    }
  }

  const updateBinding = (key: keyof TriggerBindings, value: string) => {
    setTriggerBindings((prev) => ({
      ...prev,
      [key]: value
        .split(',')
        .map((v) => v.trim())
        .filter(Boolean),
    }))
  }

  return (
    <div className="space-y-6">
      <Link
        to={isNew ? '/playbooks' : `/playbooks/${numericId}`}
        className="inline-flex items-center gap-1 text-sm text-pgp-text-secondary hover:text-pgp-text-primary"
      >
        <ArrowLeft className="h-4 w-4" /> Back
      </Link>

      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-pgp-text-primary">
          {isNew ? 'Create Playbook' : 'Edit Playbook'}
        </h1>
        <button
          onClick={handleSave}
          disabled={createPb.isPending || updatePb.isPending}
          className="inline-flex items-center gap-2 rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-pgp-accent/80 disabled:opacity-50"
        >
          <Save className="h-4 w-4" />
          {createPb.isPending || updatePb.isPending ? 'Saving...' : 'Save as Draft'}
        </button>
      </div>

      {/* Basic fields */}
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <h2 className="mb-3 text-sm font-semibold text-pgp-text-primary">Basic Information</h2>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
              Name
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="w-full rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-1.5 text-sm text-pgp-text-primary"
              placeholder="WAL Archive Failure"
            />
          </div>
          <div>
            <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
              Slug
            </label>
            <input
              type="text"
              value={slug}
              onChange={(e) => setSlug(e.target.value)}
              className="w-full rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-1.5 text-sm text-pgp-text-primary"
              placeholder="wal-archive-failure"
            />
          </div>
          <div className="col-span-2">
            <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
              Description
            </label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
              className="w-full rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-1.5 text-sm text-pgp-text-primary"
            />
          </div>
          <div>
            <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
              Category
            </label>
            <select
              value={category}
              onChange={(e) => setCategory(e.target.value)}
              className="w-full rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-1.5 text-sm text-pgp-text-primary"
            >
              {CATEGORY_OPTIONS.map((opt) => (
                <option key={opt} value={opt}>
                  {opt.charAt(0).toUpperCase() + opt.slice(1)}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
              Estimated Duration
            </label>
            <input
              type="text"
              value={estimatedDuration}
              onChange={(e) => setEstimatedDuration(e.target.value)}
              className="w-full rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-1.5 text-sm text-pgp-text-primary"
              placeholder="5 min"
            />
          </div>
          <div>
            <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
              Requires Permission
            </label>
            <select
              value={requiresPermission}
              onChange={(e) => setRequiresPermission(e.target.value)}
              className="w-full rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-1.5 text-sm text-pgp-text-primary"
            >
              {PERMISSION_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {opt.label}
                </option>
              ))}
            </select>
          </div>
        </div>
      </div>

      {/* Trigger Bindings */}
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <h2 className="mb-3 text-sm font-semibold text-pgp-text-primary">Trigger Bindings</h2>
        <p className="mb-3 text-xs text-pgp-text-muted">
          Comma-separated values. These determine when the playbook is auto-suggested.
        </p>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
              Hooks
            </label>
            <input
              type="text"
              value={(triggerBindings.hooks ?? []).join(', ')}
              onChange={(e) => updateBinding('hooks', e.target.value)}
              className="w-full rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-1.5 text-sm text-pgp-text-primary"
              placeholder="remediation.wal_archive"
            />
          </div>
          <div>
            <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
              Root Causes
            </label>
            <input
              type="text"
              value={(triggerBindings.root_causes ?? []).join(', ')}
              onChange={(e) => updateBinding('root_causes', e.target.value)}
              className="w-full rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-1.5 text-sm text-pgp-text-primary"
              placeholder="root_cause.wal_archive_failure"
            />
          </div>
          <div>
            <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
              Metrics
            </label>
            <input
              type="text"
              value={(triggerBindings.metrics ?? []).join(', ')}
              onChange={(e) => updateBinding('metrics', e.target.value)}
              className="w-full rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-1.5 text-sm text-pgp-text-primary"
              placeholder="pg.archiver.failed_count"
            />
          </div>
          <div>
            <label className="mb-1 block text-[10px] font-medium uppercase tracking-wider text-pgp-text-muted">
              Adviser Rules
            </label>
            <input
              type="text"
              value={(triggerBindings.adviser_rules ?? []).join(', ')}
              onChange={(e) => updateBinding('adviser_rules', e.target.value)}
              className="w-full rounded-md border border-pgp-border bg-pgp-bg-secondary px-3 py-1.5 text-sm text-pgp-text-primary"
              placeholder="rem_wal_archive_warn"
            />
          </div>
        </div>
      </div>

      {/* Steps */}
      <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-4">
        <h2 className="mb-3 text-sm font-semibold text-pgp-text-primary">
          Steps ({steps.length})
        </h2>
        <StepBuilder steps={steps} onChange={setSteps} />
      </div>
    </div>
  )
}
