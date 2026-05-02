import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'
import { GlobalSportSettingsProvider } from '@/lib/globalSportSettings'
import { SportFilter, type SportFilterValue } from '../SportFilter'

vi.mock('@/api/stats', () => ({
  fetchSportTypeStats: vi.fn(async () => sportStats),
}))

let sportStats: Array<{ sport_type: string; sub_type: string | null; count: number }> = [
  { sport_type: 'cycling', sub_type: 'mountain', count: 1 },
]

function renderFilter(value: SportFilterValue, onChange = vi.fn(), preset: 'ACTIVITY_ALL' | 'MAP_PANEL_GPS' = 'ACTIVITY_ALL') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <GlobalSportSettingsProvider>
        <SportFilter preset={preset} value={value} onChange={onChange} />
      </GlobalSportSettingsProvider>
    </QueryClientProvider>,
  )
  return onChange
}

describe('SportFilter', () => {
  it('splits water sports into generated water groups', () => {
    renderFilter({ sportTypes: null })

    expect(screen.getByRole('button', { name: 'Schwimmen' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Paddeln' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Surfen/Board' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Segeln/Boot' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Tauchen/Schnorcheln' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Angeln' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Surfen & Segeln' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Wassersport' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Paddeln' }))
    expect(screen.getByText('Paddeln (allgemein)')).toBeInTheDocument()
  })

  it('keeps non-GPS water sports in the activity water split', () => {
    renderFilter({ sportTypes: null })

    fireEvent.click(screen.getByRole('button', { name: 'Tauchen/Schnorcheln' }))
    expect(screen.getByText('Tauchen')).toBeInTheDocument()
    expect(screen.getByText('Schnorcheln')).toBeInTheDocument()
  })

  it('uses the water split in map filters too', () => {
    renderFilter({ sportTypes: null }, vi.fn(), 'MAP_PANEL_GPS')

    expect(screen.getByRole('button', { name: 'Schwimmen' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Paddeln' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Surfen/Board' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Segeln/Boot' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Tauchen/Schnorcheln' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Angeln' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Surfen & Segeln' })).not.toBeInTheDocument()
  })

  it('renders canonical top sport groups without deprecated top-level options', () => {
    renderFilter({ sportTypes: null })
    expect(screen.getByRole('button', { name: 'Radfahrten' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Radfahrten' }))
    expect(screen.getByText('MTB')).toBeInTheDocument()
    expect(screen.queryByText('mountain_biking')).not.toBeInTheDocument()
  })

  it('emits sport keys and modifier filters together', () => {
    const onChange = renderFilter({ sportTypes: null, modifiers: {} })
    fireEvent.change(screen.getByLabelText(/E-Bike/), { target: { value: 'true' } })
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({
      modifiers: expect.objectContaining({ is_electric: true }),
    }))
  })

  it('hide-unused keeps only existing sport tuples', async () => {
    sportStats = [{ sport_type: 'cycling', sub_type: 'mountain', count: 1 }]
    localStorage.setItem('skadia-hide-unused-sports', 'true')
    renderFilter({ sportTypes: null })

    fireEvent.click(await screen.findByRole('button', { name: 'Radfahrten' }))
    expect(screen.getByText('MTB')).toBeInTheDocument()
    expect(screen.queryByText('Gravel')).not.toBeInTheDocument()
    expect(screen.queryByText('Radfahren')).not.toBeInTheDocument()
    localStorage.removeItem('skadia-hide-unused-sports')
  })

  it('hide-unused keeps bare parent options for activities without sub type', async () => {
    sportStats = [{ sport_type: 'cycling', sub_type: null, count: 1 }]
    localStorage.setItem('skadia-hide-unused-sports', 'true')
    renderFilter({ sportTypes: null })

    fireEvent.click(await screen.findByRole('button', { name: 'Radfahrten' }))
    expect(screen.getByText('Radfahren')).toBeInTheDocument()
    expect(screen.queryByText('MTB')).not.toBeInTheDocument()
    localStorage.removeItem('skadia-hide-unused-sports')
  })
})
