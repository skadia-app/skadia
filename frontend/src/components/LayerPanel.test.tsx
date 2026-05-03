import { fireEvent, render, screen } from '@testing-library/react'
import { BrowserRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { LayerPanel } from './LayerPanel'
import type { UserMapLayer } from '@/api/mapLayers'
import type { CoverageOverlayState } from '@/hooks/useCoverageOverlays'
import type { MapOverlayState } from '@/pages/MapPage'

vi.mock('@/hooks/useSystemLayerVisibility', () => ({
  useSystemLayerVisibility: () => ({
    isLoading: false,
    visibility: () => ({
      enabled: true,
      defaultEnabledForNewUsers: false,
      defaultOpacity: null,
      label: null,
    }),
  }),
}))

const overlayState: MapOverlayState = {
  showSummits: false,
  summitsOnlyVisited: false,
  showWaters: false,
  showForests: false,
  showVisitedPois: false,
  showAmenities: false,
  showSegmentsOverlay: false,
}

const coverageState: CoverageOverlayState = {
  nature: { enabled: false, opacity: 50, layers: {} },
  noise: { enabled: false, opacity: 50, layers: {} },
  trafficUrban: { enabled: false, opacity: 50, layers: {} },
}

function userLayer(defaultOpacity: number): UserMapLayer {
  return {
    id: 1,
    name: 'User Overlay',
    role: 'overlay',
    source_kind: 'raster_tiles',
    url_template: 'https://tiles.example.test/{z}/{x}/{y}.png',
    default_opacity: defaultOpacity,
    enabled: true,
    sort_order: 0,
    created_at: '',
    updated_at: '',
  }
}

function renderPanel(defaultOpacity: number, onPatch = vi.fn(), onLiveOpacityPreview = vi.fn()) {
  return render(
    <BrowserRouter>
      <LayerPanel
        open
        onOpenChange={vi.fn()}
        overlayState={overlayState}
        onOverlayChange={vi.fn()}
        coverageState={coverageState}
        onCoverageUpdate={vi.fn()}
        onCoverageToggleLayer={vi.fn()}
        systemOverlays={[]}
        systemOverlayEnabled={{}}
        systemOverlayOpacity={{}}
        onToggleSystemOverlay={vi.fn()}
        onSetSystemOverlayOpacity={vi.fn()}
        userOverlays={[userLayer(defaultOpacity)]}
        onPatchUserLayer={onPatch}
        onLiveOpacityPreview={onLiveOpacityPreview}
        layerEntries={[{ key: 'user-1', zone: 'below', maplibreIds: ['user-overlay-1'] }]}
        onMoveTo={vi.fn()}
        onChangeZone={vi.fn()}
      />
    </BrowserRouter>,
  )
}

describe('LayerPanel user opacity slider', () => {
  it('keeps the draft value during an active drag when parent props briefly stay stale', () => {
    const onPatch = vi.fn()
    const onLiveOpacityPreview = vi.fn()
    const { rerender } = renderPanel(80, onPatch, onLiveOpacityPreview)

    fireEvent.click(screen.getByRole('button', { name: 'Optionen ausklappen' }))
    const slider = screen.getByLabelText('Deckkraft') as HTMLInputElement

    fireEvent.pointerDown(slider)
    fireEvent.change(slider, { target: { value: '55' } })

    rerender(
      <BrowserRouter>
        <LayerPanel
          open
          onOpenChange={vi.fn()}
          overlayState={overlayState}
          onOverlayChange={vi.fn()}
          coverageState={coverageState}
          onCoverageUpdate={vi.fn()}
          onCoverageToggleLayer={vi.fn()}
          systemOverlays={[]}
          systemOverlayEnabled={{}}
          systemOverlayOpacity={{}}
          onToggleSystemOverlay={vi.fn()}
          onSetSystemOverlayOpacity={vi.fn()}
          userOverlays={[userLayer(80)]}
          onPatchUserLayer={onPatch}
          onLiveOpacityPreview={onLiveOpacityPreview}
          layerEntries={[{ key: 'user-1', zone: 'below', maplibreIds: ['user-overlay-1'] }]}
          onMoveTo={vi.fn()}
          onChangeZone={vi.fn()}
        />
      </BrowserRouter>,
    )

    expect(slider.value).toBe('55')
    expect(onLiveOpacityPreview).toHaveBeenCalledWith(1, 55)
    expect(onPatch).not.toHaveBeenCalled()
  })

  it('commits the draft value even when live preview already updated the parent prop', () => {
    const onPatch = vi.fn()
    const onLiveOpacityPreview = vi.fn()
    const { rerender } = renderPanel(80, onPatch, onLiveOpacityPreview)

    fireEvent.click(screen.getByRole('button', { name: 'Optionen ausklappen' }))
    const slider = screen.getByLabelText('Deckkraft') as HTMLInputElement

    fireEvent.pointerDown(slider)
    fireEvent.change(slider, { target: { value: '55' } })

    rerender(
      <BrowserRouter>
        <LayerPanel
          open
          onOpenChange={vi.fn()}
          overlayState={overlayState}
          onOverlayChange={vi.fn()}
          coverageState={coverageState}
          onCoverageUpdate={vi.fn()}
          onCoverageToggleLayer={vi.fn()}
          systemOverlays={[]}
          systemOverlayEnabled={{}}
          systemOverlayOpacity={{}}
          onToggleSystemOverlay={vi.fn()}
          onSetSystemOverlayOpacity={vi.fn()}
          userOverlays={[userLayer(55)]}
          onPatchUserLayer={onPatch}
          onLiveOpacityPreview={onLiveOpacityPreview}
          layerEntries={[{ key: 'user-1', zone: 'below', maplibreIds: ['user-overlay-1'] }]}
          onMoveTo={vi.fn()}
          onChangeZone={vi.fn()}
        />
      </BrowserRouter>,
    )
    fireEvent.pointerUp(screen.getByLabelText('Deckkraft'))

    expect(onPatch).toHaveBeenCalledTimes(1)
    expect(onPatch).toHaveBeenCalledWith(1, { default_opacity: 55 })
  })
})
