import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SegmentSidebar, type SegmentMetadata } from './SegmentSidebar'

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

function renderSidebar(mode: 'track' | 'draw' | 'lap' | 'checkpoint') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <SegmentSidebar
        mode={mode}
        meta={baseMeta}
        onMetaChange={vi.fn()}
        info={null}
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
