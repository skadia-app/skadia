import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { StatsRow } from '../StatsRow'

const SAC_AGG  = { max: 'T4' }
const MTB_AGG  = { max: 'S3' }
const VF_AGG   = { max: 'D', hasUnknown: false }

describe('StatsRow — sport gating for scale pills', () => {
  it('cycling → no SAC, MTB, VF pills', () => {
    render(<StatsRow sportType="cycling" sacAggregate={SAC_AGG} mtbAggregate={MTB_AGG} vfAggregate={VF_AGG} />)
    expect(screen.queryByText(/T4/)).not.toBeInTheDocument()
    expect(screen.queryByText(/S3/)).not.toBeInTheDocument()
    expect(screen.queryByText(/bis D/)).not.toBeInTheDocument()
  })

  it('mountain_biking → MTB pill visible, SAC not visible', () => {
    render(<StatsRow sportType="mountain_biking" sacAggregate={SAC_AGG} mtbAggregate={MTB_AGG} />)
    expect(screen.getByText(/S3/)).toBeInTheDocument()
    // SAC is also shown (mountain_biking is in SCALE_VISIBLE_SPORTS)
    expect(screen.getByText(/T4/)).toBeInTheDocument()
  })

  it('hiking → SAC pill visible', () => {
    render(<StatsRow sportType="hiking" sacAggregate={SAC_AGG} />)
    expect(screen.getByText(/T4/)).toBeInTheDocument()
  })

  it('via_ferrata → VF pill visible', () => {
    render(<StatsRow sportType="via_ferrata" vfAggregate={VF_AGG} />)
    expect(screen.getByText(/bis D/)).toBeInTheDocument()
  })

  it('trail_running → SAC pill visible', () => {
    render(<StatsRow sportType="trail_running" sacAggregate={SAC_AGG} />)
    expect(screen.getByText(/T4/)).toBeInTheDocument()
  })

  it('forestPct pill renders only when forestPct not null', () => {
    const { rerender } = render(<StatsRow sportType="hiking" forestPct={null} />)
    expect(screen.queryByText(/Wald/)).not.toBeInTheDocument()
    rerender(<StatsRow sportType="hiking" forestPct={42} />)
    expect(screen.getByText(/42.*Wald/)).toBeInTheDocument()
  })

  it('protectedPct pill renders only when protectedPct not null', () => {
    const { rerender } = render(<StatsRow sportType="hiking" protectedPct={null} />)
    expect(screen.queryByText(/Schutzgebiet/)).not.toBeInTheDocument()
    rerender(<StatsRow sportType="hiking" protectedPct={15} />)
    expect(screen.getByText(/15.*Schutzgebiet/)).toBeInTheDocument()
  })
})
