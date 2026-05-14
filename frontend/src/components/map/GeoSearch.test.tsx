import '@testing-library/jest-dom'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import GeoSearch from './GeoSearch'
import { apiFetch } from '@/lib/auth'

vi.mock('@/lib/auth', () => ({
  apiFetch: vi.fn(),
}))

const mockedApiFetch = vi.mocked(apiFetch)

describe('GeoSearch', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mockedApiFetch.mockResolvedValue(new Response(JSON.stringify({
      results: [
        {
          name: 'Synthetic Place',
          type: 'settlement',
          lat: 1.2,
          lng: 3.4,
          source: 'local',
        },
      ],
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('searches through the backend endpoint and flies to the selected result', async () => {
    const flyTo = vi.fn()
    const mapRef = { current: { flyTo } } as any
    render(<GeoSearch mapRef={mapRef} embedded />)

    fireEvent.change(screen.getByPlaceholderText('Ort suchen...'), {
      target: { value: 'Syn' },
    })
    await act(async () => {
      vi.advanceTimersByTime(500)
      await Promise.resolve()
    })

    expect(mockedApiFetch).toHaveBeenCalledTimes(1)
    const [url] = mockedApiFetch.mock.calls[0]
    expect(url).toContain('/api/v1/geo/search?')
    expect(url).toContain('q=Syn')
    expect(url).toContain('sources=local')
    expect(url).not.toContain('external=1')
    expect(url).not.toMatch(/^https?:\/\//)

    vi.useRealTimers()
    await waitFor(() => expect(screen.getByText('Synthetic Place')).toBeInTheDocument())
    fireEvent.click(await screen.findByText('Synthetic Place'))

    expect(flyTo).toHaveBeenCalledWith({
      center: [3.4, 1.2],
      zoom: 14,
      duration: 1500,
    })
  })
})
