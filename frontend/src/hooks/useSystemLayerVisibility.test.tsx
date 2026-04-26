import { describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useSystemLayerVisibility } from './useSystemLayerVisibility'

vi.mock('./useMapLayers', () => ({
  useMapLayers: vi.fn(),
}))

import { useMapLayers } from './useMapLayers'

const mockMapLayers = (systemVisibility: Record<string, object>) => {
  ;(useMapLayers as ReturnType<typeof vi.fn>).mockReturnValue({
    isLoading: false,
    base: [],
    overlay: [],
    raw: [],
    systemVisibility,
    capabilitiesLoading: false,
  })
}

describe('useSystemLayerVisibility', () => {
  it('returns enabled=true with null label/opacity for unknown keys', () => {
    mockMapLayers({})
    const { result } = renderHook(() => useSystemLayerVisibility())
    const v = result.current.visibility('waters')
    expect(v.enabled).toBe(true)
    expect(v.defaultEnabledForNewUsers).toBe(false)
    expect(v.defaultOpacity).toBeNull()
    expect(v.label).toBeNull()
  })

  it('reflects admin-configured row when present', () => {
    mockMapLayers({
      waters: {
        layer_key: 'waters',
        enabled: false,
        default_enabled_for_new_users: true,
        default_opacity: 75,
        label_override: 'Gewässer',
        updated_at: '2026-04-26T00:00:00Z',
      },
    })
    const { result } = renderHook(() => useSystemLayerVisibility())
    const v = result.current.visibility('waters')
    expect(v.enabled).toBe(false)
    expect(v.defaultEnabledForNewUsers).toBe(true)
    expect(v.defaultOpacity).toBe(75)
    expect(v.label).toBe('Gewässer')
  })

  it('treats missing optional fields as null', () => {
    mockMapLayers({
      noise: {
        layer_key: 'noise',
        enabled: true,
        default_enabled_for_new_users: false,
        updated_at: '2026-04-26T00:00:00Z',
        // no default_opacity, no label_override
      },
    })
    const { result } = renderHook(() => useSystemLayerVisibility())
    const v = result.current.visibility('noise')
    expect(v.defaultOpacity).toBeNull()
    expect(v.label).toBeNull()
  })

  it('propagates isLoading from useMapLayers', () => {
    ;(useMapLayers as ReturnType<typeof vi.fn>).mockReturnValue({
      isLoading: true,
      base: [],
      overlay: [],
      raw: [],
      systemVisibility: {},
      capabilitiesLoading: true,
    })
    const { result } = renderHook(() => useSystemLayerVisibility())
    expect(result.current.isLoading).toBe(true)
  })
})
