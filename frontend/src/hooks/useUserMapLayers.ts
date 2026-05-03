import { useCallback, useEffect, useMemo, useRef } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  fetchUserMapLayers,
  createUserMapLayer,
  patchUserMapLayer,
  deleteUserMapLayer,
  bulkSortUserMapLayers,
  type UserMapLayer,
} from '@/api/mapLayers'
import { useCurrentUserId } from '@/providers/UserProvider'

export function useUserMapLayers() {
  // user_id in the query key prevents cross-user cache hits if logout clear()
  // ever fails — defense-in-depth for the multi-user safety guarantee.
  const userId = useCurrentUserId()
  // `select` runs on every cache read and guarantees consumers always see an
  // array, even if the cache holds a null (e.g. a JSON `null` slipping past
  // fetchUserMapLayers, or a stale cache entry written by older code).
  // Without this, a null cache value would survive past the `data = []`
  // default at call sites (which only triggers on `undefined`).
  return useQuery<UserMapLayer[] | null, Error, UserMapLayer[]>({
    queryKey: ['user-map-layers', userId],
    queryFn: fetchUserMapLayers,
    staleTime: 60_000,
    select: (data) => data ?? [],
  })
}

export interface UpdateUserMapLayerVars {
  id: number
  body: Partial<UserMapLayer>
  optimisticPrev?: UserMapLayer[]
}

interface OptimisticUpdateOptions {
  debounceMs?: number
}

export function useUserMapLayersMutations() {
  const qc = useQueryClient()
  const userId = useCurrentUserId()
  const queryKey = useMemo(() => ['user-map-layers', userId] as const, [userId])
  const invalidate = () => qc.invalidateQueries({ queryKey })
  const pendingTimers = useRef(new Map<number, ReturnType<typeof setTimeout>>())
  const pendingBodies = useRef(new Map<number, Partial<UserMapLayer>>())
  const pendingSnapshots = useRef(new Map<number, UserMapLayer[] | undefined>())

  const create = useMutation({ mutationFn: createUserMapLayer, onSuccess: invalidate })
  const update = useMutation({
      mutationKey: ['user-map-layer-update'],
      mutationFn: ({ id, body }: UpdateUserMapLayerVars) =>
        patchUserMapLayer(id, body),
      onMutate: async (vars) => {
        await qc.cancelQueries({ queryKey })
        return {
          prev: vars.optimisticPrev ?? qc.getQueryData<UserMapLayer[]>(queryKey),
        }
      },
      onError: (_err, _vars, ctx) => {
        if (ctx?.prev) qc.setQueryData(queryKey, ctx.prev)
      },
      onSettled: () => {
        if (qc.isMutating({ mutationKey: ['user-map-layer-update'] }) === 1) {
          invalidate()
        }
      },
    })
  const remove = useMutation({ mutationFn: deleteUserMapLayer, onSuccess: invalidate })
  const sort = useMutation({ mutationFn: bulkSortUserMapLayers, onSuccess: invalidate })

  useEffect(() => {
    return () => {
      pendingTimers.current.forEach(timer => clearTimeout(timer))
      pendingTimers.current.clear()
      pendingBodies.current.clear()
      pendingSnapshots.current.clear()
    }
  }, [])

  const applyOptimistic = useCallback((id: number, body: Partial<UserMapLayer>) => {
    void qc.cancelQueries({ queryKey })
    const prev = qc.getQueryData<UserMapLayer[]>(queryKey)
    qc.setQueryData<UserMapLayer[]>(queryKey, (old) =>
      (old ?? []).map((layer) => (layer.id === id ? { ...layer, ...body } : layer)),
    )
    return prev
  }, [qc, queryKey])

  const flushPending = useCallback((id: number, body: Partial<UserMapLayer>, optimisticPrev?: UserMapLayer[]) => {
    const existingTimer = pendingTimers.current.get(id)
    if (existingTimer) clearTimeout(existingTimer)
    pendingTimers.current.delete(id)

    const pending = pendingBodies.current.get(id)
    const pendingPrev = pendingSnapshots.current.get(id)
    pendingBodies.current.delete(id)
    pendingSnapshots.current.delete(id)

    const merged = { ...(pending ?? {}), ...body }
    update.mutate({ id, body: merged, optimisticPrev: pendingPrev ?? optimisticPrev })
  }, [update])

  const updateOptimistic = useCallback((
    id: number,
    body: Partial<UserMapLayer>,
    options: OptimisticUpdateOptions = {},
  ) => {
    const optimisticPrev = applyOptimistic(id, body)
    const debounceMs = options.debounceMs ?? 0
    if (debounceMs <= 0) {
      flushPending(id, body, optimisticPrev)
      return
    }

    if (!pendingBodies.current.has(id)) {
      pendingSnapshots.current.set(id, optimisticPrev)
    }
    pendingBodies.current.set(id, {
      ...(pendingBodies.current.get(id) ?? {}),
      ...body,
    })

    const existingTimer = pendingTimers.current.get(id)
    if (existingTimer) clearTimeout(existingTimer)
    pendingTimers.current.set(id, setTimeout(() => {
      const pending = pendingBodies.current.get(id)
      const pendingPrev = pendingSnapshots.current.get(id)
      pendingTimers.current.delete(id)
      pendingBodies.current.delete(id)
      pendingSnapshots.current.delete(id)
      if (pending) update.mutate({ id, body: pending, optimisticPrev: pendingPrev })
    }, debounceMs))
  }, [applyOptimistic, flushPending, update])

  return {
    create,
    update,
    updateOptimistic,
    remove,
    sort,
  }
}
