import { beforeEach, describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import { SymbolListPopover } from '../SymbolListPopover'
import type { SummaryItem } from '../StatsRow'
import { i18n } from '@/i18n/i18n'

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ locale: 'en', units: 'imperial' }),
}))

const SUMMITS: SummaryItem[] = [
  { osm_id: 1, name: 'Großglockner', track_pos: 5, ele_m: 3798 },
  { osm_id: 2, name: 'Watzmann',     track_pos: 10, ele_m: 2713 },
]

const WATERS: SummaryItem[] = [
  { osm_id: 10, name: 'Gosausee', track_pos: 3, ele_m: 933 },
]

function summit(label = '2 Gipfel') {
  return render(
    <SymbolListPopover
      kind="summit"
      icon={<span>⛰</span>}
      label={label}
      items={SUMMITS}
    />,
  )
}

describe('SymbolListPopover', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('de')
  })

  it('renders the trigger button with the label', () => {
    summit()
    expect(screen.getByRole('button', { name: /gipfelliste öffnen/i })).toBeInTheDocument()
  })

  it('mouseEnter opens the popover and shows items', () => {
    summit()
    const btn = screen.getByRole('button')
    fireEvent.mouseEnter(btn)
    expect(screen.getByText('Großglockner')).toBeInTheDocument()
    expect(screen.getByText('Watzmann')).toBeInTheDocument()
  })

  it('shows ele_m formatted with selected user units', () => {
    summit()
    fireEvent.mouseEnter(screen.getByRole('button'))
    expect(screen.getByText('12,461 ft')).toBeInTheDocument()
    expect(screen.queryByText(/3.*798.*m/)).not.toBeInTheDocument()
  })

  it('mouseLeave closes the popover (relatedTarget outside sentinel)', () => {
    summit()
    const btn = screen.getByRole('button')
    fireEvent.mouseEnter(btn)
    expect(screen.getByText('Großglockner')).toBeInTheDocument()
    // relatedTarget = document.body (outside sentinel) → should close
    fireEvent.mouseLeave(btn, { relatedTarget: document.body })
    expect(screen.queryByText('Großglockner')).not.toBeInTheDocument()
  })

  it('Escape closes the popover', () => {
    summit()
    const btn = screen.getByRole('button')
    fireEvent.mouseEnter(btn)
    expect(screen.getByText('Großglockner')).toBeInTheDocument()
    fireEvent.keyDown(btn, { key: 'Escape' })
    expect(screen.queryByText('Großglockner')).not.toBeInTheDocument()
  })

  it('onHover fires only for kind=summit, not for water', () => {
    const onHover = vi.fn()
    render(
      <SymbolListPopover
        kind="water"
        icon={<span>💧</span>}
        label="1 Gewässer"
        items={WATERS}
        onHover={onHover}
      />,
    )
    fireEvent.mouseEnter(screen.getByRole('button'))
    expect(onHover).not.toHaveBeenCalled()
  })

  it('onHover fires for kind=summit with correct args', () => {
    const onHover = vi.fn()
    render(
      <SymbolListPopover
        kind="summit"
        icon={<span>⛰</span>}
        label="2 Gipfel"
        items={SUMMITS}
        onHover={onHover}
      />,
    )
    fireEvent.mouseEnter(screen.getByRole('button'))
    expect(onHover).toHaveBeenCalledTimes(1)
    expect(onHover).toHaveBeenCalledWith('summit', SUMMITS)
  })

  it('onHover does NOT fire for kind=settlement', () => {
    const onHover = vi.fn()
    render(
      <SymbolListPopover
        kind="settlement"
        icon={<span>🏘</span>}
        label="1 Ortschaft"
        items={[{ osm_id: 99, name: 'Hallstatt', track_pos: 1, ele_m: null }]}
        onHover={onHover}
      />,
    )
    fireEvent.mouseEnter(screen.getByRole('button'))
    expect(onHover).not.toHaveBeenCalled()
  })

  it('empty items → popover content not rendered after open', () => {
    render(
      <SymbolListPopover
        kind="summit"
        icon={<span>⛰</span>}
        label="0 Gipfel"
        items={[]}
      />,
    )
    fireEvent.mouseEnter(screen.getByRole('button'))
    // No tooltip because items is empty
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  })
})
