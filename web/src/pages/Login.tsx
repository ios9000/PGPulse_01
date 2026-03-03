import { Database } from 'lucide-react'

export function Login() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-pgp-bg-primary">
      <div className="w-full max-w-sm space-y-6 rounded-lg border border-pgp-border bg-pgp-bg-card p-8">
        <div className="flex flex-col items-center gap-2">
          <Database className="h-10 w-10 text-pgp-accent" />
          <h1 className="text-2xl font-semibold text-pgp-text-primary">PGPulse</h1>
          <p className="text-sm text-pgp-text-muted">PostgreSQL Health &amp; Activity Monitor</p>
        </div>

        <form className="space-y-4" onSubmit={(e) => e.preventDefault()}>
          <div>
            <label htmlFor="username" className="block text-sm font-medium text-pgp-text-secondary">
              Username
            </label>
            <input
              id="username"
              type="text"
              className="mt-1 block w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-2 text-sm text-pgp-text-primary placeholder:text-pgp-text-muted focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
              placeholder="admin"
            />
          </div>
          <div>
            <label htmlFor="password" className="block text-sm font-medium text-pgp-text-secondary">
              Password
            </label>
            <input
              id="password"
              type="password"
              className="mt-1 block w-full rounded-md border border-pgp-border bg-pgp-bg-primary px-3 py-2 text-sm text-pgp-text-primary placeholder:text-pgp-text-muted focus:border-pgp-accent focus:outline-none focus:ring-1 focus:ring-pgp-accent"
              placeholder="••••••••"
            />
          </div>
          <button
            type="submit"
            className="w-full rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:bg-pgp-accent-hover focus:outline-none focus:ring-2 focus:ring-pgp-accent focus:ring-offset-2 focus:ring-offset-pgp-bg-primary"
          >
            Sign in
          </button>
        </form>
      </div>
    </div>
  )
}
