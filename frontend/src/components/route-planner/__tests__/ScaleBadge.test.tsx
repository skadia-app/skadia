import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import '@/i18n/i18n'
import { ScaleBadge } from '../ScaleBadge'

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ locale: 'de', units: 'metric' }),
}))

describe('ScaleBadge', () => {
  const breakdown = [
    { level: 'T1', pct: 0.7, km: 5.2 },
    { level: 'T3', pct: 0.3, km: 2.1 },
  ]

  it('renders max level label', () => {
    render(
      <ScaleBadge
        kind="sac"
        breakdown={breakdown}
        maxLevel="T3"
        isPinned={false}
        onTogglePin={() => {}}
        onHover={() => {}}
      />
    )
    expect(screen.getByRole('button')).toHaveTextContent('bis T3')
  })

  it('fires onTogglePin on click', () => {
    const onTogglePin = vi.fn()
    render(
      <ScaleBadge
        kind="sac"
        breakdown={breakdown}
        maxLevel="T3"
        isPinned={false}
        onTogglePin={onTogglePin}
        onHover={() => {}}
      />
    )
    fireEvent.click(screen.getByRole('button'))
    expect(onTogglePin).toHaveBeenCalledTimes(1)
  })

  it('has aria-pressed reflecting pin state', () => {
    const { rerender } = render(
      <ScaleBadge
        kind="sac"
        breakdown={breakdown}
        maxLevel="T3"
        isPinned={false}
        onTogglePin={() => {}}
        onHover={() => {}}
      />
    )
    expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'false')
    rerender(
      <ScaleBadge
        kind="sac"
        breakdown={breakdown}
        maxLevel="T3"
        isPinned={true}
        onTogglePin={() => {}}
        onHover={() => {}}
      />
    )
    expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'true')
  })

  it('root element carries scale-badge class', () => {
    render(
      <ScaleBadge
        kind="sac"
        breakdown={breakdown}
        maxLevel="T3"
        isPinned={false}
        onTogglePin={() => {}}
        onHover={() => {}}
      />
    )
    expect(screen.getByRole('button')).toHaveClass('scale-badge')
  })
})

describe('ScaleBadge — kind=vf (LF-#7 a11y-Parität)', () => {
  const vfBreakdown = [
    { level: 'K1', pct: 0.6, km: 1.4 },
    { level: 'K3', pct: 0.4, km: 0.9 },
  ]

  it('renders VF max level label', () => {
    render(
      <ScaleBadge
        kind="vf"
        breakdown={vfBreakdown}
        maxLevel="K3"
        isPinned={false}
        onTogglePin={() => {}}
        onHover={() => {}}
      />
    )
    expect(screen.getByRole('button')).toHaveTextContent('bis K3')
  })

  it('has type=button', () => {
    render(
      <ScaleBadge
        kind="vf"
        breakdown={vfBreakdown}
        maxLevel="K3"
        isPinned={false}
        onTogglePin={() => {}}
        onHover={() => {}}
      />
    )
    expect(screen.getByRole('button')).toHaveAttribute('type', 'button')
  })

  it('has aria-pressed reflecting pin state (vf)', () => {
    const { rerender } = render(
      <ScaleBadge
        kind="vf"
        breakdown={vfBreakdown}
        maxLevel="K3"
        isPinned={false}
        onTogglePin={() => {}}
        onHover={() => {}}
      />
    )
    expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'false')
    rerender(
      <ScaleBadge
        kind="vf"
        breakdown={vfBreakdown}
        maxLevel="K3"
        isPinned={true}
        onTogglePin={() => {}}
        onHover={() => {}}
      />
    )
    expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'true')
  })

  it('has aria-label containing expanded kind name and maxLevel (vf)', () => {
    render(
      <ScaleBadge
        kind="vf"
        breakdown={vfBreakdown}
        maxLevel="K3"
        isPinned={false}
        onTogglePin={() => {}}
        onHover={() => {}}
      />
    )
    const btn = screen.getByRole('button')
    expect(btn).toHaveAttribute('aria-label')
    const label = btn.getAttribute('aria-label') ?? ''
    // aria-label now expands abbreviations: "vf" → "Klettersteig-Skala"
    expect(label).toContain('Klettersteig-Skala')
    expect(label).toContain('K3')
  })

  it('fires onTogglePin via native click activation (Space/Enter on button triggers onClick)', () => {
    // Native <button> activates onClick on Space keyup and Enter keydown —
    // no custom onKeyDown handler needed. Verify the click path works.
    const onTogglePin = vi.fn()
    render(
      <ScaleBadge
        kind="vf"
        breakdown={vfBreakdown}
        maxLevel="K3"
        isPinned={false}
        onTogglePin={onTogglePin}
        onHover={() => {}}
      />
    )
    fireEvent.click(screen.getByRole('button'))
    expect(onTogglePin).toHaveBeenCalledTimes(1)
  })

  it('keyDown alone does NOT trigger onTogglePin (no custom handler — native fires on keyup)', () => {
    // The old custom onKeyDown handler called onTogglePin on keydown,
    // which combined with the native browser keyup→click caused double-toggle.
    // After removing the handler, keyDown alone must not call onTogglePin.
    const onTogglePin = vi.fn()
    render(
      <ScaleBadge
        kind="vf"
        breakdown={vfBreakdown}
        maxLevel="K3"
        isPinned={false}
        onTogglePin={onTogglePin}
        onHover={() => {}}
      />
    )
    fireEvent.keyDown(screen.getByRole('button'), { key: ' ' })
    expect(onTogglePin).not.toHaveBeenCalled()
  })

  it('calls onHover(true) on focus and onHover(false) on blur (vf)', () => {
    const onHover = vi.fn()
    render(
      <ScaleBadge
        kind="vf"
        breakdown={vfBreakdown}
        maxLevel="K3"
        isPinned={false}
        onTogglePin={() => {}}
        onHover={onHover}
      />
    )
    const btn = screen.getByRole('button')
    fireEvent.focus(btn)
    expect(onHover).toHaveBeenLastCalledWith(true)
    fireEvent.blur(btn)
    expect(onHover).toHaveBeenLastCalledWith(false)
  })

  it('carries scale-badge class (vf)', () => {
    render(
      <ScaleBadge
        kind="vf"
        breakdown={vfBreakdown}
        maxLevel="K3"
        isPinned={false}
        onTogglePin={() => {}}
        onHover={() => {}}
      />
    )
    expect(screen.getByRole('button')).toHaveClass('scale-badge')
  })
})
