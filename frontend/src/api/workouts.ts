// frontend/src/api/workouts.ts
import { apiClient } from './client'
import type { Workout, WorkoutStep, WorkoutActivityRef } from './types'

export async function fetchWorkouts(limit = 20, offset = 0): Promise<Workout[]> {
  const res = await apiClient<{ workouts: Omit<Workout, 'steps' | 'activities'>[] }>(
    `/workouts?limit=${limit}&offset=${offset}`,
  )
  return (res.workouts ?? []).map(w => ({ ...w, steps: [] }))
}

export async function fetchWorkout(id: string): Promise<Workout> {
  const res = await apiClient<{
    workout: Omit<Workout, 'steps' | 'activities'>
    steps: WorkoutStep[]
    activities: WorkoutActivityRef[]
  }>(`/workouts/${id}`)
  return { ...res.workout, steps: res.steps ?? [], activities: res.activities ?? [] }
}
