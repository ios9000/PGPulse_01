import { Moon, Sun, Monitor } from 'lucide-react'
import { useThemeStore } from '@/stores/themeStore'

const themeIcons = {
  dark: Moon,
  light: Sun,
  system: Monitor,
} as const

const themeLabels = {
  dark: 'Dark mode',
  light: 'Light mode',
  system: 'System theme',
} as const

const nextTheme = {
  dark: 'light',
  light: 'system',
  system: 'dark',
} as const

export function ThemeToggle() {
  const { theme, setTheme } = useThemeStore()
  const Icon = themeIcons[theme]

  return (
    <button
      onClick={() => setTheme(nextTheme[theme])}
      className="rounded-md p-2 text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary"
      aria-label={themeLabels[theme]}
      title={themeLabels[theme]}
    >
      <Icon className="h-5 w-5" />
    </button>
  )
}
