/**
 * Tests for LiftHelpModal — Phase 11.10
 *
 * Verifies:
 * - Modal renders Methode-A and Methode-B sections when open.
 * - ASCII bidirektional diagram is visible (contains "Bergstation A").
 * - Close button works (calls onClose callback).
 * - Modal is not mounted when open=false.
 * - Phase 11.10: heading is "Heuristik (bidirektional)" (reverted from Talquerung).
 * - Phase 11.10: "Wichtig:" altitude-vs-direction clarification is rendered.
 */
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import LiftHelpModal from './LiftHelpModal'

describe('LiftHelpModal', () => {
  it('renders nothing when open=false', () => {
    const { container } = render(
      <LiftHelpModal open={false} onClose={() => {}} />,
    )
    // ModalShell is not rendered — no dialog role in DOM
    expect(container.querySelector('[role="dialog"]')).toBeNull()
  })

  it('renders both detection method sections when open=true', () => {
    render(<LiftHelpModal open={true} onClose={() => {}} />)
    expect(screen.getByText(/Methode A — OSM-Match/)).toBeTruthy()
    expect(screen.getByText(/Methode B — Heuristik aus dem Stream/)).toBeTruthy()
  })

  it('renders the bidirektional heuristic class section (Phase 11.10)', () => {
    render(<LiftHelpModal open={true} onClose={() => {}} />)
    expect(screen.getByText(/Heuristik \(bidirektional\)/)).toBeTruthy()
    // The old Talquerung heading must not appear
    expect(screen.queryByText(/Heuristik \(Talquerung\)/)).toBeNull()
  })

  it('renders the ASCII bidirektional diagram with "Bergstation A"', () => {
    render(<LiftHelpModal open={true} onClose={() => {}} />)
    // The pre block contains the ASCII art; test for a distinctive string.
    expect(screen.getByText(/Bergstation A/)).toBeTruthy()
  })

  it('renders the "Wichtig:" route-vs-direction clarification', () => {
    render(<LiftHelpModal open={true} onClose={() => {}} />)
    expect(screen.getByText(/Wichtig:/)).toBeTruthy()
    // Content explaining bidir refers to the lift route shape, not user direction
    expect(screen.getByText(/Verlauf der/)).toBeTruthy()
    expect(screen.getByText(/nicht auf die Richtung der Fahrt/)).toBeTruthy()
  })

  it('renders the pause-gaps section', () => {
    render(<LiftHelpModal open={true} onClose={() => {}} />)
    expect(screen.getByText(/Pause-Lücken/)).toBeTruthy()
  })

  it('close button calls onClose', () => {
    const onClose = vi.fn()
    render(<LiftHelpModal open={true} onClose={onClose} />)
    const closeBtn = screen.getByRole('button', { name: /Schließen/ })
    fireEvent.click(closeBtn)
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('Escape key calls onClose via ModalShell panel', () => {
    const onClose = vi.fn()
    render(<LiftHelpModal open={true} onClose={onClose} />)
    // ModalShell registers the keydown handler on the inner panel div (tabIndex=-1),
    // not on the outer role="dialog" wrapper.
    const panel = document.querySelector('[tabindex="-1"]') as HTMLElement
    expect(panel).not.toBeNull()
    fireEvent.keyDown(panel, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('header title reads "Wie Skadia Lifte erkennt"', () => {
    render(<LiftHelpModal open={true} onClose={() => {}} />)
    expect(screen.getByText(/Wie Skadia Lifte erkennt/)).toBeTruthy()
  })
})
