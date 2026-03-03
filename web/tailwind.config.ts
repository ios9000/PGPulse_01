import type { Config } from 'tailwindcss'

export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        pgp: {
          'bg-primary': 'var(--pgp-bg-primary)',
          'bg-secondary': 'var(--pgp-bg-secondary)',
          'bg-card': 'var(--pgp-bg-card)',
          'bg-hover': 'var(--pgp-bg-hover)',
          border: 'var(--pgp-border)',
          accent: 'var(--pgp-accent)',
          'accent-hover': 'var(--pgp-accent-hover)',
          ok: 'var(--pgp-ok)',
          warning: 'var(--pgp-warning)',
          critical: 'var(--pgp-critical)',
          info: 'var(--pgp-info)',
          'text-primary': 'var(--pgp-text-primary)',
          'text-secondary': 'var(--pgp-text-secondary)',
          'text-muted': 'var(--pgp-text-muted)',
        },
      },
      width: {
        sidebar: '240px',
        'sidebar-collapsed': '64px',
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
        mono: ['JetBrains Mono', 'Fira Code', 'monospace'],
      },
    },
  },
  plugins: [],
} satisfies Config
