// frontend/src/api/workouts.ts
import { apiClient } from './client'
import type { Workout, WorkoutStep } from './types'

export async function fetchWorkout(id: string): Promise<Workout> {
  const res = await apiClient<{ workout: Omit<Workout, 'steps'>; steps: WorkoutStep[] }>(`/workouts/${id}`)
  return { ...res.workout, steps: res.steps ?? [] }
}
