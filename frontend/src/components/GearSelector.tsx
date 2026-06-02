import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { fetchGear } from '@/api/gear'
import { apiClient } from '@/api/client'
import { getActiveSchema } from '@/lib/gearSchema'
import { gearTypeLabel as schemaGearTypeLabel } from '@/lib/gearSchemaHelpers'

interface GearSelectorProps {
  activityId: string
  currentGearId?: string | null
}

export default function GearSelector({ activityId, currentGearId }: GearSelectorProps) {
  const { t } = useTranslation()
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
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">{t('activityOperations.gear.title')}</h2>
      <div className="flex items-center gap-3">
        <select
          value={currentGearId ?? ''}
          onChange={(e) => mutation.mutate(e.target.value || null)}
          disabled={mutation.isPending}
          className="flex-1 border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
        >
          <option value="">{t('activityOperations.gear.none')}</option>
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
            className="px-3 py-2 rounded-lg text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {t('activityOperations.gear.remove')}
          </button>
        )}
      </div>
    </div>
  )
}

function gearTypeLabel(type: string): string {
  return schemaGearTypeLabel(getActiveSchema(), type)
}
