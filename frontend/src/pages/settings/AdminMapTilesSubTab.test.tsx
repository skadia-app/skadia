import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AdminMapTilesSubTab from './AdminMapTilesSubTab'
import { ToastProvider } from '@/lib/toast'

const useCapabilities = vi.fn()
const fetchOSMStatus = vi.fn()
const fetchOSMRegionStatus = vi.fn()
const getOSMSchedule = vi.fn()
const osmRegionManager = vi.fn()

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => useCapabilities(),
}))

vi.mock('@/api/osm', () => ({
  fetchOSMStatus: () => fetchOSMStatus(),
  importLifts: vi.fn(),
  fetchOSMRegionStatus: () => fetchOSMRegionStatus(),
  getOSMSchedule: () => getOSMSchedule(),
  setOSMSchedule: vi.fn(),
}))

vi.mock('@/components/admin/MapLayersAdminSection', () => ({
  MapLayersAdminSection: () => <div>MapLayersAdminSection</div>,
}))

vi.mock('@/components/admin/SystemLayerVisibilityAdminSection', () => ({
  SystemLayerVisibilityAdminSection: () => <div>SystemLayerVisibilityAdminSection</div>,
}))

vi.mock('@/components/DEMSettings', () => ({
  DEMSettings: () => <div>DEMSettings</div>,
}))

vi.mock('@/components/OfflineMapSettings', () => ({
  OfflineMapSettings: () => <div>OfflineMapSettings</div>,
}))

vi.mock('@/components/OSMRegionManager', () => ({
  OSMRegionManager: (props: { fullGeodataAvailable?: boolean }) => {
    osmRegionManager(props)
    return <div>OSMRegionManager {String(props.fullGeodataAvailable)}</div>
  },
}))

vi.mock('@/components/OSMImportModal', () => ({
  default: () => <div>OSMImportModal</div>,
}))

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <ToastProvider>
        <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      </ToastProvider>
    )
  }
}

describe('AdminMapTilesSubTab capability gates', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('fetch', vi.fn())
    useCapabilities.mockReturnValue({
      isLoading: false,
      data: {
        features: {
          osm_import: { configured: false, available: false, reason: 'core_profile' },
          dem_correction: { configured: false, available: false, reason: 'core_profile' },
          offline_map: { configured: false, available: false, reason: 'core_profile' },
          routing_tiles: { configured: false, available: false, reason: 'core_profile' },
          system_layers_vector: { configured: false, available: false, reason: 'core_profile' },
          brouter: { configured: true, available: true },
          brouter_segment_download: { configured: true, available: true },
        },
      },
    })
  })

  it('does not start Full-only admin queries in Core', async () => {
    render(<AdminMapTilesSubTab />, { wrapper: wrapper() })

    expect(await screen.findByText('BRouter-Segmente')).toBeInTheDocument()
    expect(screen.queryByText('DEMSettings')).not.toBeInTheDocument()
    expect(screen.queryByText('OfflineMapSettings')).not.toBeInTheDocument()

    await waitFor(() => expect(osmRegionManager).toHaveBeenCalledWith(expect.objectContaining({ fullGeodataAvailable: false })))
    expect(fetchOSMStatus).not.toHaveBeenCalled()
    expect(fetchOSMRegionStatus).not.toHaveBeenCalled()
    expect(getOSMSchedule).not.toHaveBeenCalled()
  })

  it('keeps configured bootstrap operations visible when runtime artifacts are unavailable', async () => {
    useCapabilities.mockReturnValue({
      isLoading: false,
      data: {
        features: {
          osm_import: { configured: true, available: false, reason: 'missing_artifacts' },
          dem_correction: { configured: true, available: false, reason: 'missing_artifacts' },
          offline_map: { configured: true, available: false, reason: 'missing_artifacts' },
          routing_tiles: { configured: true, available: false, reason: 'missing_artifacts' },
          system_layers_vector: { configured: true, available: false, reason: 'missing_artifacts' },
          segment_osm_lift_import: { configured: true, available: false, reason: 'missing_artifacts' },
        },
      },
    })

    render(<AdminMapTilesSubTab />, { wrapper: wrapper() })

    expect(await screen.findByText('OSM-Geodaten')).toBeInTheDocument()
    expect(screen.getAllByText('Nicht verfügbar').length).toBeGreaterThan(0)
    expect(screen.getByText('DEMSettings')).toBeInTheDocument()
    expect(screen.queryByText('OfflineMapSettings')).not.toBeInTheDocument()
    expect(screen.queryByText('Erneut prüfen')).not.toBeInTheDocument()
    expect(fetch).not.toHaveBeenCalled()
    expect(screen.getByText('Import-Assistent')).toBeInTheDocument()
    expect(screen.queryByText('Lift-Segmente importieren')).not.toBeInTheDocument()
  })
})
