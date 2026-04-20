import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import { ScaleBadge } from '../ScaleBadge'

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
