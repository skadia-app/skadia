import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import PrivacySettingsPage from './PrivacySettingsPage'

const mocks = vi.hoisted(() => ({
  listPrivacyZones: vi.fn(),
}))

vi.mock('@/api/privacy', () => ({
  listPrivacyZones: mocks.listPrivacyZones,
  deletePrivacyZone: vi.fn(),
  updatePrivacyZone: vi.fn(),
}))

vi.mock('@/api/rules', () => ({
  createCommuteRuleFromZones: vi.fn(),
}))

vi.mock('@/components/PrivacyZoneEditor', () => ({
  default: () => <div data-testid="privacy-zone-editor" />,
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}))

vi.mock('@/lib/modal', () => ({
  useModal: () => ({ openModal: vi.fn() }),
}))

vi.mock('maplibre-gl', () => {
  class Map {
    addControl = vi.fn()
    remove = vi.fn()
    on = vi.fn((event: string, cb: () => void) => {
      if (event === 'load') cb()
      return this
    })
    once = vi.fn((event: string, cb: () => void) => {
      if (event === 'style.load') cb()
      return this
    })
    isStyleLoaded = vi.fn(() => true)
    getLayer = vi.fn(() => false)
    getSource = vi.fn(() => undefined)
    addSource = vi.fn()
    addLayer = vi.fn()
    removeLayer = vi.fn()
    removeSource = vi.fn()
    fitBounds = vi.fn()
  }
  class Marker {
    setLngLat = vi.fn(() => this)
    setPopup = vi.fn(() => this)
    addTo = vi.fn(() => this)
    remove = vi.fn()
    getElement = vi.fn(() => document.createElement('div'))
  }
  class Popup {
    setHTML = vi.fn(() => this)
  }
  return {
    default: {
      Map,
      Marker,
      Popup,
      NavigationControl: class {},
      LngLatBounds: class {
        extend = vi.fn(() => this)
        isEmpty = vi.fn(() => false)
      },
    },
  }
})

function renderPrivacySettings() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <PrivacySettingsPage />
    </QueryClientProvider>,
  )
}

describe('PrivacySettingsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('fetch', vi.fn())
    mocks.listPrivacyZones.mockResolvedValue([{
      id: 'zone-1',
      name: 'Private Zone',
      lat: 47.12345,
      lng: 11.12345,
      radius_m: 250,
      color: '#f97316',
      is_active: true,
      zone_type: 'circle',
      polygon: null,
    }])
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('does not reverse-geocode private-zone coordinates through a hard-coded external endpoint', async () => {
    renderPrivacySettings()

    expect(await screen.findByText('Private Zone')).toBeInTheDocument()
    await waitFor(() => expect(mocks.listPrivacyZones).toHaveBeenCalled())
    expect(fetch).not.toHaveBeenCalled()
  })
})
