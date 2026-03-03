import { useState } from 'react'
import { useTimeRangeStore, type PresetKey } from '@/stores/timeRangeStore'

const PRESETS: PresetKey[] = ['15m', '1h', '6h', '24h', '7d']

export function TimeRangeSelector() {
  const range = useTimeRangeStore((s) => s.range)
  const setPreset = useTimeRangeStore((s) => s.setPreset)
  const setCustomRange = useTimeRangeStore((s) => s.setCustomRange)
  const [showCustom, setShowCustom] = useState(range.preset === 'custom')
  const [customFrom, setCustomFrom] = useState('')
  const [customTo, setCustomTo] = useState('')

  const handlePresetClick = (preset: PresetKey) => {
    setShowCustom(false)
    setPreset(preset)
  }

  const handleCustomClick = () => {
    setShowCustom(true)
  }

  const handleApplyCustom = () => {
    if (customFrom && customTo) {
      setCustomRange(new Date(customFrom), new Date(customTo))
    }
  }

  return (
    <div className="rounded-lg border border-pgp-border bg-pgp-bg-card p-3">
      <div className="flex flex-wrap items-center gap-2">
        {PRESETS.map((preset) => {
          const isActive = range.preset === preset
          return (
            <button
              key={preset}
              onClick={() => handlePresetClick(preset)}
              className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
                isActive
                  ? 'border border-blue-500 bg-blue-500/20 text-blue-400'
                  : 'border border-pgp-border bg-pgp-bg-secondary text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary'
              }`}
            >
              {preset}
            </button>
          )
        })}
        <button
          onClick={handleCustomClick}
          className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
            range.preset === 'custom'
              ? 'border border-blue-500 bg-blue-500/20 text-blue-400'
              : 'border border-pgp-border bg-pgp-bg-secondary text-pgp-text-secondary hover:bg-pgp-bg-hover hover:text-pgp-text-primary'
          }`}
        >
          Custom
        </button>
      </div>
      {showCustom && (
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <input
            type="datetime-local"
            value={customFrom}
            onChange={(e) => setCustomFrom(e.target.value)}
            className="rounded-md border border-pgp-border bg-pgp-bg-secondary px-2 py-1 text-sm text-pgp-text-primary"
          />
          <span className="text-sm text-pgp-text-muted">to</span>
          <input
            type="datetime-local"
            value={customTo}
            onChange={(e) => setCustomTo(e.target.value)}
            className="rounded-md border border-pgp-border bg-pgp-bg-secondary px-2 py-1 text-sm text-pgp-text-primary"
          />
          <button
            onClick={handleApplyCustom}
            disabled={!customFrom || !customTo}
            className="rounded-md bg-pgp-accent px-3 py-1 text-sm font-medium text-white hover:bg-pgp-accent-hover disabled:opacity-50"
          >
            Apply
          </button>
        </div>
      )}
    </div>
  )
}
