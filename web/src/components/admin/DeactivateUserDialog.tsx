interface DeactivateUserDialogProps {
  username: string
  onConfirm: () => void
  onCancel: () => void
}

export function DeactivateUserDialog({ username, onConfirm, onCancel }: DeactivateUserDialogProps) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-sm rounded-lg border border-pgp-border bg-pgp-bg-card p-6">
        <h2 className="mb-2 text-lg font-semibold text-pgp-text-primary">Deactivate User</h2>
        <p className="mb-6 text-sm text-pgp-text-secondary">
          Are you sure you want to deactivate <span className="font-medium text-pgp-text-primary">{username}</span>? They will no longer be able to sign in.
        </p>
        <div className="flex justify-end gap-3">
          <button onClick={onCancel} className="rounded-md px-4 py-2 text-sm text-pgp-text-secondary hover:bg-pgp-bg-hover">
            Cancel
          </button>
          <button
            onClick={onConfirm}
            className="rounded-md bg-red-500 px-4 py-2 text-sm font-medium text-white hover:bg-red-600"
          >
            Deactivate
          </button>
        </div>
      </div>
    </div>
  )
}
