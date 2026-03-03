import { Link } from 'react-router-dom'

export function NotFound() {
  return (
    <div className="flex flex-col items-center justify-center py-24 text-center">
      <h1 className="text-6xl font-bold text-pgp-text-muted">404</h1>
      <p className="mt-4 text-xl text-pgp-text-secondary">Page Not Found</p>
      <Link
        to="/fleet"
        className="mt-6 rounded-md bg-pgp-accent px-4 py-2 text-sm font-medium text-white hover:bg-pgp-accent-hover"
      >
        Return to Fleet Overview
      </Link>
    </div>
  )
}
