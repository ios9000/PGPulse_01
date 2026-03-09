import { useState } from 'react'
import { Navigate } from 'react-router-dom'
import { PageHeader } from '@/components/ui/PageHeader'
import { InstancesTab } from '@/components/admin/InstancesTab'
import { UsersTab } from '@/components/admin/UsersTab'
import { useAuth } from '@/hooks/useAuth'

type Tab = 'instances' | 'users'

export function Administration() {
  const { can } = useAuth()
  const canManageInstances = can('instance_management')
  const canManageUsers = can('user_management')

  const defaultTab: Tab = canManageInstances ? 'instances' : 'users'
  const [activeTab, setActiveTab] = useState<Tab>(defaultTab)

  if (!canManageInstances && !canManageUsers) {
    return <Navigate to="/fleet" replace />
  }

  const tabs: { id: Tab; label: string; visible: boolean }[] = [
    { id: 'instances', label: 'Instances', visible: canManageInstances },
    { id: 'users', label: 'Users', visible: canManageUsers },
  ]

  return (
    <div>
      <PageHeader title="Administration" subtitle="Manage instances and users" />

      <div className="mb-6 border-b border-pgp-border">
        <nav className="-mb-px flex gap-6">
          {tabs
            .filter((t) => t.visible)
            .map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`border-b-2 pb-3 text-sm font-medium transition-colors ${
                  activeTab === tab.id
                    ? 'border-blue-500 text-pgp-text-primary'
                    : 'border-transparent text-pgp-text-muted hover:border-pgp-border hover:text-pgp-text-secondary'
                }`}
              >
                {tab.label}
              </button>
            ))}
        </nav>
      </div>

      {activeTab === 'instances' && canManageInstances && <InstancesTab />}

      {activeTab === 'users' && canManageUsers && <UsersTab />}
    </div>
  )
}
