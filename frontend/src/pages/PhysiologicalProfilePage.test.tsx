import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import '@/i18n/i18n'
import { i18n } from '@/i18n/i18n'
import PhysiologicalProfilePage from './PhysiologicalProfilePage'

const userPrefs = vi.hoisted(() => ({ locale: 'en', units: 'imperial' as 'metric' | 'imperial' }))
const updateUserProfileMock = vi.hoisted(() => vi.fn())

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => userPrefs,
}))

vi.mock('@/api/settings', () => ({
  getUserProfile: vi.fn(async () => ({
    birth_date: null,
    gender: '',
    height_cm: 178,
    weight_kg: 70,
    ftp_w: null,
    lthr_bpm: null,
    max_hr_bpm: null,
    resting_hr_bpm: null,
    max_hr_formula: null,
    max_hr_detected: null,
    effective_max_hr: 190,
  })),
  updateUserProfile: updateUserProfileMock,
}))

vi.mock('@/api/trainingZones', () => ({
  fetchTrainingZones: vi.fn(async () => ({ sports: [] })),
}))

vi.mock('@/components/zones/ZoneEditor', () => ({
  default: () => null,
}))

function renderPage(children: ReactNode) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{children}</MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('PhysiologicalProfilePage units', () => {
  beforeEach(async () => {
    userPrefs.locale = 'en'
    userPrefs.units = 'imperial'
    updateUserProfileMock.mockReset()
    updateUserProfileMock.mockResolvedValue({})
    await i18n.changeLanguage('en')
  })

  it('displays profile weight in user units and stores kg', async () => {
    renderPage(<PhysiologicalProfilePage />)

    const weightInput = await screen.findByLabelText('Weight (lb)')
    await waitFor(() => expect(weightInput).toHaveValue(154.3))

    fireEvent.change(weightInput, { target: { value: '176.4' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save profile' }))

    await waitFor(() => expect(updateUserProfileMock).toHaveBeenCalled())
    expect(updateUserProfileMock.mock.calls[0][0].weight_kg).toBeCloseTo(80, 1)
  })
})
