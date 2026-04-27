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
  // `select` runs on every cache read and guarantees consumers always see an
  // array, even if the cache holds a null (e.g. a JSON `null` slipping past
  // fetchUserMapLayers, or a stale cache entry written by older code).
  // Without this, a null cache value would survive past the `data = []`
  // default at call sites (which only triggers on `undefined`).
  return useQuery<UserMapLayer[] | null, Error, UserMapLayer[]>({
    queryKey: ['user-map-layers'],
    queryFn: fetchUserMapLayers,
    staleTime: 60_000,
    select: (data) => data ?? [],
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
