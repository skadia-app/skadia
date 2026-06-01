import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ComponentProps } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { SegmentSidebar, type SegmentMetadata } from './SegmentSidebar'
import { fmtDist, fmtElevation } from '@/lib/format'

vi.mock('@/api/accessScopes', () => ({
  listObjectAccessScopes: vi.fn().mockResolvedValue({ scopes: [] }),
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

function renderSidebar(
  mode: 'track' | 'draw' | 'lap' | 'checkpoint',
  info: ComponentProps<typeof SegmentSidebar>['info'] = null,
) {
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

describe('SegmentSidebar live metrics', () => {
  it('formats distance and elevation metrics through the central unit formatters', () => {
    renderSidebar('track', {
      distanceKm: 1.234,
      gainM: 235.4,
      lossM: 56.2,
      elevationMaxM: 2001.8,
    })

    expect(screen.getByText(fmtDist(1234))).toBeInTheDocument()
    expect(screen.getByText(`+${fmtElevation(235.4)}`)).toBeInTheDocument()
    expect(screen.getByText(`−${fmtElevation(56.2)}`)).toBeInTheDocument()
    expect(screen.getByText(fmtElevation(2001.8))).toBeInTheDocument()
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
