import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { OSMRegionManager } from './OSMRegionManager'
import { ModalProvider } from '@/lib/modal'
import { ToastProvider } from '@/lib/toast'

const fetchOSMRegionStatus = vi.fn()
const fetchOSMDiskUsage = vi.fn()
const fetchOSMRegions = vi.fn()

vi.mock('@/api/osm', () => ({
  fetchOSMRegionStatus: () => fetchOSMRegionStatus(),
  fetchOSMDiskUsage: () => fetchOSMDiskUsage(),
  fetchOSMRegions: () => fetchOSMRegions(),
  deleteOSMRegion: vi.fn(),
  cancelOSMImport: vi.fn(),
  retryOSMEnrichment: vi.fn(),
  rebuildTiles: vi.fn(),
  buildBRouterSegments: vi.fn(),
  downloadBRouterSegments: vi.fn(),
  buildCoverageGrids: vi.fn(),
  triggerOSMRegionUpdate: vi.fn(),
  batchUpdateAllRegions: vi.fn(),
}))

vi.mock('@/lib/useActiveJobs', () => ({
  useActiveJobs: () => ({ jobs: [] }),
}))

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <ToastProvider>
        <ModalProvider>
          <QueryClientProvider client={qc}>{children}</QueryClientProvider>
        </ModalProvider>
      </ToastProvider>
    )
  }
}

describe('OSMRegionManager capability mode', () => {
  it('keeps Core RD5 download without querying local OSM pipeline state', async () => {
    fetchOSMRegions.mockResolvedValue({
      regions: [{ id: 'austria', label: 'Österreich', continent: 'Europa' }],
    })

    render(<OSMRegionManager fullGeodataAvailable={false} />, { wrapper: wrapper() })

    expect(await screen.findByText('BRouter-Segmente')).toBeInTheDocument()
    expect(screen.getByText('RD5-Download')).toBeInTheDocument()
    expect(screen.queryByText('Importierte Regionen')).not.toBeInTheDocument()

    await waitFor(() => expect(fetchOSMRegions).toHaveBeenCalled())
    expect(fetchOSMRegionStatus).not.toHaveBeenCalled()
    expect(fetchOSMDiskUsage).not.toHaveBeenCalled()
  })
})
