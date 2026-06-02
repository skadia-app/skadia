import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SegmentSidebar, type LiveInfo, type SegmentMetadata } from './SegmentSidebar'

vi.mock('@/api/accessScopes', () => ({
  listObjectAccessScopes: vi.fn().mockResolvedValue({ scopes: [] }),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ locale: 'en', units: 'imperial' }),
}))

const baseMeta: SegmentMetadata = {
  name: 'Checkpoint-Kurs',
  segType: 'standard',
  sportTypes: null,
  sacScale: null,
  mtbScale: null,
  surface: null,
  difficultyColor: null,
  bidirectional: true,
  userEditedBidirectional: false,
  isPublic: true,
  visibility: 'public',
  accessScopeId: null,
  climbType: 'standard',
}

function renderSidebar(mode: 'track' | 'draw' | 'lap' | 'checkpoint', info: LiveInfo | null = null) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <SegmentSidebar
        mode={mode}
        meta={baseMeta}
        onMetaChange={vi.fn()}
        info={info}
        allSportTypes={[{ key: 'ride', label: 'Radfahren', category: 'bike' }]}
        onEnrichmentRequest={vi.fn()}
        enrichmentState={{ loading: false }}
        enrichmentSources={{}}
        onClearEnrichmentSource={vi.fn()}
      />
    </QueryClientProvider>,
  )
}

describe('SegmentSidebar checkpoint mode', () => {
  it('hides controls that checkpoint course creation does not persist', () => {
    renderSidebar('checkpoint')

    expect(screen.queryByText('Typ')).not.toBeInTheDocument()
    expect(screen.queryByText('Schwierigkeit & Oberfläche')).not.toBeInTheDocument()
    expect(screen.queryByText('Höhenprofil')).not.toBeInTheDocument()
    expect(screen.queryByText('Richtung')).not.toBeInTheDocument()

    expect(screen.getByText('Sportarten')).toBeInTheDocument()
    expect(screen.getByText('Sichtbarkeit')).toBeInTheDocument()
  })
})

describe('SegmentSidebar metric summary', () => {
  it('formats distance and elevation with the active unit preference', () => {
    renderSidebar('track', {
      distanceKm: 5,
      gainM: 100,
      lossM: 25,
      elevationMaxM: 1234,
    })

    expect(screen.getByText('3.1 mi')).toBeInTheDocument()
    expect(screen.getByText('+328 ft')).toBeInTheDocument()
    expect(screen.getByText('−82 ft')).toBeInTheDocument()
    expect(screen.getByText('4,049 ft')).toBeInTheDocument()
    expect(screen.queryByText('5.0 km')).not.toBeInTheDocument()
    expect(screen.queryByText('+100 m')).not.toBeInTheDocument()
  })

  it('does not prefix empty zero-elevation values with a sign', () => {
    renderSidebar('track', {
      distanceKm: 0,
      gainM: 0,
      lossM: 0,
      elevationMaxM: null,
    })

    expect(screen.queryByText('+—')).not.toBeInTheDocument()
    expect(screen.queryByText('−—')).not.toBeInTheDocument()
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(2)
  })
})
