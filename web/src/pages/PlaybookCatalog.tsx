import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Plus } from 'lucide-react'
import { PageHeader } from '@/components/ui/PageHeader'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { PlaybookCard } from '@/components/playbook/PlaybookCard'
import { PlaybookFilters } from '@/components/playbook/PlaybookFilters'
import { usePlaybooks } from '@/hooks/usePlaybooks'
import { useAuth } from '@/hooks/useAuth'

export function PlaybookCatalog() {
  const [status, setStatus] = useState('all')
  const [category, setCategory] = useState('all')
  const [search, setSearch] = useState('')
  const { can } = useAuth()

  const filters = {
    status: status === 'all' ? undefined : status,
    category: category === 'all' ? undefined : category,
    search: search || undefined,
  }

  const { data: playbooks, isLoading } = usePlaybooks(filters)

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <PageHeader title="Playbooks" subtitle="Guided remediation workflows" />
        {can('alert_management') && (
          <Link
            to="/playbooks/new/edit"
            className="inline-flex items-center gap-2 rounded-md bg-pgp-accent px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-pgp-accent/80"
          >
            <Plus className="h-4 w-4" />
            Create Playbook
          </Link>
        )}
      </div>

      <PlaybookFilters
        status={status}
        category={category}
        search={search}
        onStatusChange={setStatus}
        onCategoryChange={setCategory}
        onSearchChange={setSearch}
      />

      {isLoading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : !playbooks || playbooks.length === 0 ? (
        <EmptyState
          title="No playbooks found"
          description="No playbooks match the current filters."
        />
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {playbooks.map((pb) => (
            <PlaybookCard key={pb.id} playbook={pb} />
          ))}
        </div>
      )}
    </div>
  )
}
