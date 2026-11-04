import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ETABadge } from './ETABadge'
import { makeETA } from '@/test/fixtures'

function dot() {
  return document.querySelector('[title^="Confidence:"]')
}

describe('ETABadge', () => {
  it('renders an em dash and no indicator when there is no ETA', () => {
    const { container } = render(<ETABadge eta={undefined} />)
    expect(container.textContent).toBe('—')
    expect(dot()).toBeNull()
  })

  it.each([
    [30, '< 1 min'],
    [300, '5m'],
    [3600, '1h'],
    [5400, '1h 30m'],
    [86400, '1d'],
    [93600, '1d 2h'],
  ])('formats a high-confidence ETA of %is as "%s" in green', (sec, text) => {
    const { container } = render(<ETABadge eta={makeETA({ eta_sec: sec, confidence: 'high' })} />)
    const badge = container.firstElementChild!
    expect(badge.textContent).toBe(text)
    expect(badge.classList).toContain('text-green-400')
    expect(dot()?.classList).toContain('bg-green-500')
  })

  it('prefixes medium-confidence ETAs with "~" and colours them yellow', () => {
    const { container } = render(<ETABadge eta={makeETA({ eta_sec: 600, confidence: 'medium' })} />)
    expect(container.textContent).toBe('~10m')
    expect(container.firstElementChild!.classList).toContain('text-yellow-400')
    expect(dot()?.getAttribute('title')).toBe('Confidence: medium')
  })

  it('shows "Stalled" in red when eta_sec is -1 and confidence is stalled', () => {
    render(<ETABadge eta={makeETA({ eta_sec: -1, confidence: 'stalled' })} />)
    const badge = screen.getByText('Stalled')
    expect(badge.classList).toContain('text-red-400')
    expect(dot()?.classList).toContain('bg-red-500')
  })

  it('shows a pulsing "estimating..." when eta_sec is -1 and confidence is estimating', () => {
    render(<ETABadge eta={makeETA({ eta_sec: -1, confidence: 'estimating' })} />)
    const badge = screen.getByText('estimating...')
    expect(badge.classList).toContain('animate-pulse')
    expect(badge.classList).toContain('text-slate-400')
  })

  it('falls back to an em dash when eta_sec is -1 with a confident estimate', () => {
    const { container } = render(<ETABadge eta={makeETA({ eta_sec: -1, confidence: 'high' })} />)
    expect(container.textContent).toBe('—')
    expect(dot()).toBeNull()
  })
})
