import { create } from 'zustand'

export type PresetKey = '15m' | '1h' | '6h' | '24h' | '7d'

export interface TimeRange {
  preset: PresetKey | 'custom'
  from?: Date
  to?: Date
}

const PRESET_DURATIONS: Record<PresetKey, number> = {
  '15m': 15 * 60 * 1000,
  '1h': 60 * 60 * 1000,
  '6h': 6 * 60 * 60 * 1000,
  '24h': 24 * 60 * 60 * 1000,
  '7d': 7 * 24 * 60 * 60 * 1000,
}

interface TimeRangeState {
  range: TimeRange
  setPreset: (preset: PresetKey) => void
  setCustomRange: (from: Date, to: Date) => void
  getEffectiveRange: () => { from: Date; to: Date }
}

export const useTimeRangeStore = create<TimeRangeState>()((set, get) => ({
  range: { preset: '1h' },

  setPreset: (preset: PresetKey) => {
    set({ range: { preset } })
  },

  setCustomRange: (from: Date, to: Date) => {
    set({ range: { preset: 'custom', from, to } })
  },

  getEffectiveRange: () => {
    const { range } = get()
    if (range.preset === 'custom' && range.from && range.to) {
      return { from: range.from, to: range.to }
    }
    const key = range.preset === 'custom' ? '1h' : range.preset
    const now = new Date()
    const from = new Date(now.getTime() - PRESET_DURATIONS[key])
    return { from, to: now }
  },
}))
