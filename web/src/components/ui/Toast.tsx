import { X } from 'lucide-react'
import { useToastStore, type ToastItem } from '@/stores/toastStore'

const TYPE_STYLES: Record<ToastItem['type'], string> = {
  success: 'border-green-500 bg-green-500/10 text-green-400',
  error: 'border-red-500 bg-red-500/10 text-red-400',
  warning: 'border-amber-500 bg-amber-500/10 text-amber-400',
}

function ToastEntry({ item }: { item: ToastItem }) {
  const removeToast = useToastStore((s) => s.removeToast)

  return (
    <div
      className={`flex items-start gap-2 rounded-lg border px-4 py-3 shadow-lg backdrop-blur-sm ${TYPE_STYLES[item.type]}`}
    >
      <span className="flex-1 text-sm">{item.message}</span>
      <button
        onClick={() => removeToast(item.id)}
        className="shrink-0 opacity-70 hover:opacity-100"
      >
        <X className="h-4 w-4" />
      </button>
    </div>
  )
}

export function ToastContainer() {
  const toasts = useToastStore((s) => s.toasts)

  if (toasts.length === 0) return null

  return (
    <div className="fixed right-4 top-4 z-50 flex w-80 flex-col gap-2">
      {toasts.map((item) => (
        <ToastEntry key={item.id} item={item} />
      ))}
    </div>
  )
}
