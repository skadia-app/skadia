import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import RouteMetadataForm from '../RouteMetadataForm'

vi.mock('@/api/tags', () => ({
  fetchTags: vi.fn().mockResolvedValue([]),
}))

function renderForm() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <RouteMetadataForm currentProfile="hiking-mountain" onChange={vi.fn()} />
    </QueryClientProvider>,
  )
}

describe('RouteMetadataForm standard route UI', () => {
  it('keeps route save metadata focused on route-relevant fields', () => {
    renderForm()

    expect(screen.getByText('E-Antrieb')).toBeInTheDocument()
    expect(screen.queryByText('Indoor')).not.toBeInTheDocument()
    expect(screen.queryByText('Virtuell')).not.toBeInTheDocument()
    expect(screen.queryByText('Pendeln')).not.toBeInTheDocument()
    expect(screen.getByText('Routing-Profil')).toBeInTheDocument()
    expect(screen.queryByText('Routing-Profil (Override)')).not.toBeInTheDocument()
  })
})
