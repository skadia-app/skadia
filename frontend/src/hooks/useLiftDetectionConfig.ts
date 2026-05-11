import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getLiftDetectionConfig,
  putLiftDetectionConfig,
  deleteLiftDetectionConfig,
  type LiftDetectionConfigResponse,
} from '@/api/liftDetectionConfig'

export type { LiftDetectionConfigResponse }

export function useLiftDetectionConfig(activityId: string) {
  return useQuery<LiftDetectionConfigResponse>({
    queryKey: ['lift-detection-config', activityId],
    queryFn: () => getLiftDetectionConfig(activityId),
    staleTime: 30_000,
  })
}

export function usePutLiftDetectionConfig(activityId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { osm?: boolean; heuristic?: boolean }) =>
      putLiftDetectionConfig(activityId, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['lift-detection-config', activityId] })
    },
  })
}

export function useResetLiftDetectionConfig(activityId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => deleteLiftDetectionConfig(activityId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['lift-detection-config', activityId] })
    },
  })
}
