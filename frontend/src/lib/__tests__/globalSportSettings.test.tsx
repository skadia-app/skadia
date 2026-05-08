import { act, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it } from 'vitest'

import { GlobalSportSettingsProvider, useGlobalSportSettings } from '../globalSportSettings'

const wrapper = ({ children }: { children: ReactNode }) => (
  <GlobalSportSettingsProvider>{children}</GlobalSportSettingsProvider>
)

describe('globalSportSettings', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('uses custom sport colors before generated taxonomy defaults', () => {
    localStorage.setItem('skadia-sport-colors-global', JSON.stringify({ running: '#123456' }))

    const { result } = renderHook(() => useGlobalSportSettings(), { wrapper })

    expect(result.current.getSportColor('running')).toBe('#123456')
    expect(result.current.getSportColor('cycling')).toBe('#F97316')
  })

  it('persists and resets custom sport colors without changing generated defaults', () => {
    const { result } = renderHook(() => useGlobalSportSettings(), { wrapper })

    act(() => {
      result.current.setSportColor('running', '#ABCDEF')
    })

    expect(JSON.parse(localStorage.getItem('skadia-sport-colors-global') ?? '{}')).toEqual({ running: '#ABCDEF' })
    expect(result.current.getSportColor('running')).toBe('#ABCDEF')

    act(() => {
      result.current.resetSportColor('running')
    })

    expect(result.current.getSportColor('running')).toBe('#EF4444')
  })
})
