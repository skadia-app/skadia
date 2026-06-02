import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Workout } from '@/api/types'
import { fetchWorkout } from '@/api/workouts'
import { i18n } from '@/i18n/i18n'
import WorkoutDetailPage from './WorkoutDetailPage'

const userPrefs = vi.hoisted(() => ({ locale: 'en', units: 'imperial' as 'metric' | 'imperial' }))

vi.mock('@/api/workouts', () => ({
  fetchWorkout: vi.fn(),
}))

vi.mock('@/components/activity/WorkoutOverview', () => ({
  WorkoutOverview: () => null,
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => userPrefs,
}))

function renderWorkoutDetail() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/workouts/workout-1']}>
        <Routes>
          <Route path="/workouts/:id" element={<WorkoutDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function workout(): Workout {
  return {
    id: 'workout-1',
    name: 'Tempo',
    num_valid_steps: 1,
    pool_length_m: 25,
    steps: [{
      message_index: 0,
      step_name: 'Interval',
      duration_type: 1,
      duration_value: 1609.344,
      intensity: 0,
    }],
    activities: [{
      id: 'activity-1',
      title: 'Done run',
      started_at: '2026-05-01T08:00:00Z',
      sport_type: 'running',
      distance_m: 3218.688,
      elapsed_time_s: 1800,
    }],
  }
}

describe('WorkoutDetailPage unit preferences', () => {
  beforeEach(async () => {
    userPrefs.locale = 'en'
    userPrefs.units = 'imperial'
    vi.clearAllMocks()
    await i18n.changeLanguage('en')
  })

  it('formats workout step and completed activity distances with selected user units', async () => {
    vi.mocked(fetchWorkout).mockResolvedValue(workout())

    renderWorkoutDetail()

    expect(await screen.findByText('Tempo')).toBeInTheDocument()
    expect(screen.getByText('Pool length: 82 ft')).toBeInTheDocument()
    expect(screen.getByText('1.0 mi')).toBeInTheDocument()
    expect(screen.getByText('2.0 mi')).toBeInTheDocument()
    expect(screen.queryByText('1.61 km')).not.toBeInTheDocument()
    expect(screen.queryByText('3.2 km')).not.toBeInTheDocument()
  })
})
