import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getGearDuplicateCandidates, mergeGear } from '@/api/importWizard'
import { fetchGear } from '@/api/gear'
import type { Gear } from '@/api/types'

export default function GearMergePanel({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient()
  const [ignored, setIgnored] = useState<Set<string>>(new Set())

  const { data: pairs = [], isLoading: pairsLoading } = useQuery({
    queryKey: ['gear-duplicate-candidates'],
    queryFn: getGearDuplicateCandidates,
  })

  const { data: allGear = [] } = useQuery<Gear[]>({
    queryKey: ['gear'],
    queryFn: () => fetchGear(undefined),
  })

  const mergeMut = useMutation({
    mutationFn: ({ primary, from }: { primary: string; from: string }) =>
      mergeGear(primary, from),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gear'] })
      queryClient.invalidateQueries({ queryKey: ['gear-duplicate-candidates'] })
    },
  })

  const visiblePairs = pairs.filter(([a, b]) => !ignored.has(`${a}:${b}`))

  if (pairsLoading) {
    return <div className="text-center py-8 text-neutral-400 text-sm">Suche nach Duplikaten…</div>
  }

  if (visiblePairs.length === 0) {
    return (
      <div className="text-center py-8">
        <p className="text-neutral-500 dark:text-neutral-400 text-sm">Keine möglichen Duplikate gefunden.</p>
        <button onClick={onClose} className="mt-4 text-sm text-orange-500 hover:text-orange-600 font-medium transition-colors">
          Schließen
        </button>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-neutral-500 dark:text-neutral-400">
        {visiblePairs.length} mögliche Duplikate gefunden. Wähle welches als primär behalten werden soll.
      </p>
      {visiblePairs.map(([idA, idB]) => {
        const gA = allGear.find((g) => g.id === idA)
        const gB = allGear.find((g) => g.id === idB)
        if (!gA || !gB) return null
        const key = `${idA}:${idB}`
        const isPending = mergeMut.isPending

        return (
          <div key={key} className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4">
            <div className="grid grid-cols-2 gap-4 mb-3">
              {([gA, gB] as Gear[]).map((g) => (
                <div key={g.id} className="space-y-0.5">
                  <div className="font-medium text-sm text-neutral-900 dark:text-neutral-100">{g.name}</div>
                  {(g.brand || g.model) && (
                    <div className="text-xs text-neutral-500">{[g.brand, g.model].filter(Boolean).join(' ')}</div>
                  )}
                  <div className="text-xs text-neutral-400">
                    {((g.total_distance_m ?? 0) / 1000).toFixed(0)} km
                  </div>
                </div>
              ))}
            </div>
            <div className="flex gap-2">
              <button
                onClick={() => mergeMut.mutate({ primary: idA, from: idB })}
                disabled={isPending}
                className="flex-1 text-xs py-1.5 rounded-lg bg-orange-500 hover:bg-orange-600 disabled:opacity-50 text-white transition-colors"
              >
                {gA.name} behalten
              </button>
              <button
                onClick={() => mergeMut.mutate({ primary: idB, from: idA })}
                disabled={isPending}
                className="flex-1 text-xs py-1.5 rounded-lg bg-orange-500 hover:bg-orange-600 disabled:opacity-50 text-white transition-colors"
              >
                {gB.name} behalten
              </button>
              <button
                onClick={() => setIgnored(s => new Set([...s, key]))}
                className="px-3 text-xs py-1.5 rounded-lg border border-neutral-300 dark:border-neutral-600 text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 transition-colors"
              >
                Ignorieren
              </button>
            </div>
          </div>
        )
      })}
    </div>
  )
}
