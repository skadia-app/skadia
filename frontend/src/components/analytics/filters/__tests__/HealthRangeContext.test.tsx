import { describe, it, expect } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import type { ReactNode } from 'react'
import { HealthRangeProvider, useHealthRange } from '../HealthRangeContext'

function wrapper({ children }: { children: ReactNode }) {
  return <HealthRangeProvider>{children}</HealthRangeProvider>
}

describe('HealthRangeContext', () => {
  it('provides a default range of approximately last 30 days', () => {
    const { result } = renderHook(() => useHealthRange(), { wrapper })

    const from = new Date(result.current.from)
    const to = new Date(result.current.to)
    const diffDays = Math.round((to.getTime() - from.getTime()) / (1000 * 60 * 60 * 24))
    expect(diffDays).toBe(30)
  })

  it('default "to" date is today', () => {
    const { result } = renderHook(() => useHealthRange(), { wrapper })

    const today = new Date()
    const todayISO = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`
    expect(result.current.to).toBe(todayISO)
  })

  it('setRange propagates new range to all consumers', () => {
    const { result } = renderHook(() => useHealthRange(), { wrapper })

    act(() => {
      result.current.setRange({ from: '2024-01-01', to: '2024-06-30' })
    })

    expect(result.current.from).toBe('2024-01-01')
    expect(result.current.to).toBe('2024-06-30')
  })

  it('throws when used outside HealthRangeProvider', () => {
    expect(() => {
      renderHook(() => useHealthRange())
    }).toThrow('useHealthRange must be used within HealthRangeProvider')
  })
})
