import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchGear } from '@/api/gear'
import { apiClient } from '@/api/client'

interface GearSelectorProps {
  activityId: string
  currentGearId?: string | null
}

export default function GearSelector({ activityId, currentGearId }: GearSelectorProps) {
  const queryClient = useQueryClient()

  const { data: gearList } = useQuery({
    queryKey: ['gear-list'],
    queryFn: () => fetchGear(false),
  })

  const mutation = useMutation({
    mutationFn: (gearId: string | null) =>
      apiClient(`/activities/${activityId}`, {
        method: 'PATCH',
        body: { gear_id: gearId },
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activity', activityId] })
    },
  })

  const activeGear = gearList?.filter((g) => !g.is_retired) ?? []

  return (
    <div className="space-y-2">
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Ausrüstung</h2>
      <div className="flex items-center gap-3">
        <select
          value={currentGearId ?? ''}
          onChange={(e) => mutation.mutate(e.target.value || null)}
          disabled={mutation.isPending}
          className="flex-1 border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
        >
          <option value="">Keine Ausrüstung</option>
          {activeGear.map((g) => (
            <option key={g.id} value={g.id}>
              {g.name} ({gearTypeLabel(g.type)})
            </option>
          ))}
        </select>
        {currentGearId && (
          <button
            onClick={() => mutation.mutate(null)}
            disabled={mutation.isPending}
            className="px-3 py-2 rounded-lg text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
          >
            Entfernen
          </button>
        )}
      </div>
    </div>
  )
}

function gearTypeLabel(type: string): string {
  const labels: Record<string, string> = {
    bike: 'Fahrrad',
    shoes: 'Laufschuhe',
    goggles: 'Schwimmbrille',
    ski: 'Ski',
    paddle: 'Paddel',
    other: 'Sonstiges',
  }
  return labels[type] ?? type
}
