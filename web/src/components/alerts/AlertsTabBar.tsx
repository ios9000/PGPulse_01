import { Link } from 'react-router-dom'

type AlertTab = 'active' | 'history' | 'rules'

interface AlertsTabBarProps {
  activeTab: AlertTab
}

const tabs: { key: AlertTab; label: string; to: string }[] = [
  { key: 'active', label: 'Active', to: '/alerts' },
  { key: 'history', label: 'History', to: '/alerts?view=history' },
  { key: 'rules', label: 'Rules', to: '/alerts/rules' },
]

export function AlertsTabBar({ activeTab }: AlertsTabBarProps) {
  return (
    <div className="mb-4 flex gap-0 border-b border-pgp-border">
      {tabs.map((tab) => (
        <Link
          key={tab.key}
          to={tab.to}
          className={`px-4 py-2 text-sm font-medium transition-colors ${
            activeTab === tab.key
              ? 'border-b-2 border-blue-500 font-bold text-pgp-text-primary'
              : 'text-gray-500 hover:text-pgp-text-primary'
          }`}
        >
          {tab.label}
        </Link>
      ))}
    </div>
  )
}
