import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { ToastProvider } from '@/lib/toast'
import { createWeightEntry } from '@/api/healthMutations'
import { i18n } from '@/i18n/i18n'
import WeightEntryDialog from './WeightEntryDialog'

const userPrefs = vi.hoisted(() => ({ locale: 'en', units: 'imperial' as 'metric' | 'imperial' }))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => userPrefs,
}))

vi.mock('@/api/settings', () => ({
  getUserProfile: vi.fn(async () => ({ height_cm: 178 })),
}))

vi.mock('@/api/healthMutations', () => ({
  createWeightEntry: vi.fn(async () => ({
    user_id: 'user-1',
    measured_at_utc: '2026-05-31T00:00:00Z',
    weight_g: 70_000,
    bmi: null,
    body_fat_pct: null,
    muscle_mass_g: null,
    bone_mass_g: null,
    source_type: 'manual',
    created_at: '2026-05-31T00:00:00Z',
  })),
}))

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return (
    <QueryClientProvider client={client}>
      <ToastProvider>{children}</ToastProvider>
    </QueryClientProvider>
  )
}

describe('WeightEntryDialog', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    userPrefs.locale = 'en'
    userPrefs.units = 'imperial'
    await i18n.changeLanguage('en')
  })

  it('accepts displayed pounds but submits stored grams', async () => {
    render(<WeightEntryDialog onClose={vi.fn()} />, { wrapper })

    const weightGroup = screen.getByText('Weight').closest('div')
    expect(weightGroup).not.toBeNull()
    expect(within(weightGroup as HTMLElement).getByText('lb')).toBeInTheDocument()

    const numericInputs = screen.getAllByRole('spinbutton')
    fireEvent.change(numericInputs[0], { target: { value: '154.3' } })

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(createWeightEntry).toHaveBeenCalled())
    expect(vi.mocked(createWeightEntry).mock.calls[0][0]).toMatchObject({
      weight_g: 69_989,
    })
  })
})
