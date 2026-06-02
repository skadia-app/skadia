import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError } from '@/api/client'
import type { ActivityGearAssignment } from '@/api/types'
import { addActivityGearAssignment, fetchActivity, removeActivityGearAssignment } from '@/api/activities'
import { fetchGear } from '@/api/gear'
import { getActiveSchema } from '@/lib/gearSchema'
import { gearTypeLabel as schemaGearTypeLabel } from '@/lib/gearSchemaHelpers'
import { isSupportedLanguage, localeForLanguage, type SupportedLanguage } from '@/i18n/languages'

interface ActivityGearSectionProps {
  activityId: string
  assignments?: ActivityGearAssignment[]
}

function currentLanguage(language: string): SupportedLanguage {
  const normalized = language.split('-')[0]
  return isSupportedLanguage(normalized) ? normalized : 'de'
}

function gearTypeLabel(type: string, language: SupportedLanguage): string {
  return schemaGearTypeLabel(getActiveSchema(), type, language)
}

export default function ActivityGearSection({ activityId, assignments = [] }: ActivityGearSectionProps) {
  const queryClient = useQueryClient()
  const { t, i18n } = useTranslation()
  const language = currentLanguage(i18n.language)
  const locale = localeForLanguage(language)
  const [adding, setAdding] = useState(false)
  const [errorMessage, setErrorMessage] = useState<string | null>(null)

  const { data: gearList } = useQuery({
    queryKey: ['gear-list'],
    queryFn: () => fetchGear(false),
    staleTime: 60_000,
  })

  const assignedGearIds = new Set(assignments.map(a => a.gear_id))
  const selectableGear = (gearList ?? [])
    .filter(g => !g.is_retired)
    .filter(g => !assignedGearIds.has(g.id))
    .sort((a, b) => a.name.localeCompare(b.name, locale))

  const refreshActivity = async () => {
    const updated = await fetchActivity(activityId)
    queryClient.setQueryData(['activity', activityId], updated)
    queryClient.invalidateQueries({ queryKey: ['gear-list'] })
  }

  const addMut = useMutation({
    mutationFn: (gearId: string) => addActivityGearAssignment(activityId, gearId),
    onMutate: () => {
      setErrorMessage(null)
    },
    onSuccess: async () => {
      setAdding(false)
      await refreshActivity()
    },
    onError: (error) => {
      if (error instanceof ApiError && error.status === 404) {
        setErrorMessage(t('activityGear.errors.endpointUnavailable'))
        return
      }
      setErrorMessage(t('activityGear.errors.retry'))
    },
  })

  const removeMut = useMutation({
    mutationFn: (assignmentId: string) => removeActivityGearAssignment(activityId, assignmentId),
    onMutate: () => {
      setErrorMessage(null)
    },
    onSuccess: refreshActivity,
    onError: () => {
      setErrorMessage(t('activityGear.errors.retry'))
    },
  })

  const busy = addMut.isPending || removeMut.isPending

  return (
    <div className="space-y-2">
      <div className="flex items-start gap-2">
        <button
          type="button"
          onClick={() => {
            setErrorMessage(null)
            setAdding(open => !open)
          }}
          disabled={busy}
          className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-neutral-100 text-xs font-medium text-neutral-500 transition-colors hover:bg-neutral-200 disabled:opacity-50 dark:bg-neutral-800 dark:text-neutral-400 dark:hover:bg-neutral-700"
          title={t('activityGear.add')}
          aria-label={t('activityGear.add')}
        >
          +
        </button>
        <div className="min-w-0 flex-1 space-y-1">
          {assignments.length === 0 && !adding && (
            <div className="py-1 text-sm text-neutral-500 dark:text-neutral-400">{t('activityGear.none')}</div>
          )}

          {assignments.map(assignment => (
            <div
              key={assignment.id}
              className="group flex min-h-7 items-center justify-between gap-2 rounded-md px-0.5 py-1 text-sm hover:bg-neutral-50 dark:hover:bg-neutral-800"
            >
              <Link
                to={`/gear/${assignment.gear_id}`}
                className="min-w-0 flex-1 truncate font-medium text-neutral-800 hover:text-orange-600 dark:text-neutral-100"
              >
                {assignment.gear_name}
              </Link>
              <span className="shrink-0 text-xs text-neutral-400">{assignment.role_label || gearTypeLabel(assignment.gear_type, language)}</span>
              <button
                type="button"
                onClick={() => removeMut.mutate(assignment.id)}
                disabled={busy}
                className="shrink-0 rounded px-1 text-sm text-neutral-400 opacity-0 transition-opacity hover:text-red-600 focus:opacity-100 group-hover:opacity-100 disabled:opacity-40"
                title={t('activityGear.remove', { name: assignment.gear_name })}
                aria-label={t('activityGear.remove', { name: assignment.gear_name })}
              >
                &times;
              </button>
            </div>
          ))}

          {adding && (
            <select
              autoFocus
              aria-label={t('activityGear.choose')}
              value=""
              disabled={busy}
              onChange={(event) => {
                if (event.target.value) addMut.mutate(event.target.value)
              }}
              className="w-full rounded-md border border-neutral-200 bg-white px-2 py-1.5 text-sm text-neutral-900 outline-none focus:border-orange-400 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100"
            >
              <option value="">{selectableGear.length ? t('activityGear.choose') : t('activityGear.noActiveGear')}</option>
              {selectableGear.map(gear => (
                <option key={gear.id} value={gear.id}>
                  {gear.name} ({gearTypeLabel(gear.type, language)})
                </option>
              ))}
            </select>
          )}

          {busy && (
            <div className="text-xs text-neutral-500 dark:text-neutral-400" aria-live="polite">
              {t('activityGear.updating')}
            </div>
          )}

          {errorMessage && (
            <div className="rounded-md bg-red-50 px-2 py-1.5 text-xs text-red-700 dark:bg-red-950/30 dark:text-red-300" role="alert">
              <div className="font-semibold">{t('activityGear.errors.addFailed')}</div>
              <div>{errorMessage}</div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
