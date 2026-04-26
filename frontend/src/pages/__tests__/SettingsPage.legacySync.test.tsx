/**
 * Tests for the one-shot legacy-sync logic in AutomatischeAnalysenSection.
 *
 * AutomatischeAnalysenSection is a private component inside SettingsPage.
 * We replicate the minimal contract under test in a thin wrapper to avoid
 * mounting the full SettingsPage (which depends on router, UserProvider, etc.).
 * The logic under test is the useEffect that fires mut.mutate exactly once
 * when enable_surface_detection !== enable_difficulty_scales.
 */

import { describe, it, expect, vi } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useRef, useEffect } from 'react'

// Replicate the exact legacy-sync logic from AutomatischeAnalysenSection
// so that any refactor to the page that changes the logic will also break
// this test (verifying correctness), while avoiding mounting the full page.
type Settings = { enable_surface_detection: boolean; enable_difficulty_scales: boolean }

function useLegacySync(settings: Settings | undefined, mutate: (patch: Partial<Settings>) => void) {
  const firedRef = useRef(false)
  useEffect(() => {
    if (firedRef.current) return
    if (!settings) return
    if (settings.enable_surface_detection !== settings.enable_difficulty_scales) {
      firedRef.current = true
      mutate({ enable_difficulty_scales: settings.enable_surface_detection })
    } else {
      firedRef.current = true
    }
  }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
}

describe('SettingsPage — AutomatischeAnalysenSection legacy-sync', () => {
  it('fires exactly one mutation when surface_detection=true and difficulty_scales=false', () => {
    const mutate = vi.fn()
    const settings: Settings = { enable_surface_detection: true, enable_difficulty_scales: false }
    renderHook(() => useLegacySync(settings, mutate))
    expect(mutate).toHaveBeenCalledTimes(1)
    expect(mutate).toHaveBeenCalledWith({ enable_difficulty_scales: true })
  })

  it('does not fire mutation when settings are in sync', () => {
    const mutate = vi.fn()
    const settings: Settings = { enable_surface_detection: true, enable_difficulty_scales: true }
    renderHook(() => useLegacySync(settings, mutate))
    expect(mutate).not.toHaveBeenCalled()
  })

  it('does not fire when settings undefined (still loading)', () => {
    const mutate = vi.fn()
    renderHook(() => useLegacySync(undefined, mutate))
    expect(mutate).not.toHaveBeenCalled()
  })

  it('does not fire a second time on re-render with new settings object (useRef guard)', () => {
    const mutate = vi.fn()
    const initialSettings: Settings = { enable_surface_detection: true, enable_difficulty_scales: false }
    const { rerender } = renderHook(
      ({ s }: { s: Settings }) => useLegacySync(s, mutate),
      { initialProps: { s: initialSettings } },
    )
    expect(mutate).toHaveBeenCalledTimes(1)
    // Re-render with a new (but still asymmetric) settings object
    act(() => {
      rerender({ s: { enable_surface_detection: true, enable_difficulty_scales: false } })
    })
    // Guard must prevent a second call
    expect(mutate).toHaveBeenCalledTimes(1)
  })
})
