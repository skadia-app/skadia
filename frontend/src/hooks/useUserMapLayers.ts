import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  fetchUserMapLayers,
  createUserMapLayer,
  patchUserMapLayer,
  deleteUserMapLayer,
  bulkSortUserMapLayers,
  type UserMapLayer,
} from '@/api/mapLayers'

export function useUserMapLayers() {
  return useQuery<UserMapLayer[]>({
    queryKey: ['user-map-layers'],
    queryFn: fetchUserMapLayers,
    staleTime: 60_000,
  })
}

export function useUserMapLayersMutations() {
  const qc = useQueryClient()
  const invalidate = () => qc.invalidateQueries({ queryKey: ['user-map-layers'] })

  return {
    create: useMutation({ mutationFn: createUserMapLayer, onSuccess: invalidate }),
    update: useMutation({
      mutationFn: ({ id, body }: { id: number; body: Partial<UserMapLayer> }) =>
        patchUserMapLayer(id, body),
      onSuccess: invalidate,
    }),
    remove: useMutation({ mutationFn: deleteUserMapLayer, onSuccess: invalidate }),
    sort: useMutation({ mutationFn: bulkSortUserMapLayers, onSuccess: invalidate }),
  }
}
