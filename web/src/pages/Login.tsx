import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Database } from 'lucide-react'
import { useAuthStore } from '@/stores/authStore'

export function Login() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [retryCountdown, setRetryCountdown] = useState(0)
  const navigate = useNavigate()
  const login = useAuthStore((s) => s.login)
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)

  useEffect(() => {
    if (isAuthenticated) {
      navigate('/fleet', { replace: true })
    }
  }, [isAuthenticated, navigate])

  useEffect(() => {
    if (retryCountdown <= 0) return
    const timer = setInterval(() => {
      setRetryCountdown((prev) => {
        if (prev <= 1) {
          clearInterval(timer)
          return 0
        }
        return prev - 1
      })
    }, 1000)
    return () => clearInterval(timer)
  }, [retryCountdown])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setIsSubmitting(true)

    try {
      await login(username, password)
      navigate('/fleet', { replace: true })
    } catch (err) {
      const loginErr = err as Error & { status?: number; retryAfter?: number }
      if (loginErr.status === 429 && loginErr.retryAfter) {
        setRetryCountdown(loginErr.retryAfter)
        setError(`Too many login attempts. Try again in ${loginErr.retryAfter} seconds.`)
      } else {
        setError(loginErr.message || 'Login failed')
      }
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-pgp-bg-primary">
      <div className="w-full max-w-sm space-y-6 rounded-lg border border-pgp-border bg-pgp-bg-card p-8">
        <div className="flex flex-col items-center gap-2">
          <Database className="h-10 w-10 text-pgp-accent" />
          <h1 className="text-2xl font-semibold text-pgp-text-primary">PGPulse</h1>
          <p className="text-sm text-pgp-text-muted">PostgreSQL Health &amp; Activity Monitor</p>
        </div>

        <form className="space-y-4" onSubmit={handleSubmit}>
          {error && (
            <div className="rounded-md bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
          )}

          <div>
            <label htmlFor="username" className="block text-sm font-medium text-pgp-text-secondary">
              Username
            </label>
            <input
              id="username"
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="mt-1 block w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-2 text-sm text-pgp-text-primary placeholder:text-pgp-text-muted focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
              placeholder="admin"
              autoComplete="username"
              required
            />
          </div>
          <div>
            <label htmlFor="password" className="block text-sm font-medium text-pgp-text-secondary">
              Password
            </label>
            <input
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="mt-1 block w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-2 text-sm text-pgp-text-primary placeholder:text-pgp-text-muted focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
              placeholder="••••••••"
              autoComplete="current-password"
              required
            />
          </div>
          <button
            type="submit"
            disabled={isSubmitting || retryCountdown > 0}
            className="w-full rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:bg-pgp-accent-hover focus:outline-none focus:ring-2 focus:ring-pgp-accent focus:ring-offset-2 focus:ring-offset-pgp-bg-primary disabled:opacity-50"
          >
            {isSubmitting ? 'Signing in...' : retryCountdown > 0 ? `Retry in ${retryCountdown}s` : 'Sign in'}
          </button>
        </form>
      </div>
    </div>
  )
}
