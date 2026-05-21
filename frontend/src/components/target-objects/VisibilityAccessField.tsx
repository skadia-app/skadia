import { useQuery } from '@tanstack/react-query'
import { listObjectAccessScopes } from '@/api/accessScopes'
import type { TargetVisibility } from '@/api/targetObjects'

interface VisibilityAccessFieldProps {
  visibility: TargetVisibility
  accessScopeId?: string | null
  onChange: (next: { visibility: TargetVisibility; access_scope_id?: string | null }) => void
}

const VISIBILITY_OPTIONS: { value: TargetVisibility; label: string }[] = [
  { value: 'private', label: 'Privat' },
  { value: 'shared', label: 'Geteilt' },
  { value: 'public', label: 'Öffentlich' },
]

export default function VisibilityAccessField({ visibility, accessScopeId, onChange }: VisibilityAccessFieldProps) {
  const { data } = useQuery({
    queryKey: ['object-access-scopes'],
    queryFn: listObjectAccessScopes,
    enabled: visibility === 'shared',
    staleTime: 60_000,
  })

  return (
    <div className="grid gap-3 sm:grid-cols-[minmax(0,220px)_minmax(0,1fr)]">
      <label className="block">
        <span className="mb-1 block text-sm font-medium text-neutral-700 dark:text-neutral-200">Sichtbarkeit</span>
        <select
          value={visibility}
          onChange={(event) => {
            const next = event.target.value as TargetVisibility
            onChange({ visibility: next, access_scope_id: next === 'shared' ? accessScopeId ?? null : null })
          }}
          className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
        >
          {VISIBILITY_OPTIONS.map(option => (
            <option key={option.value} value={option.value}>{option.label}</option>
          ))}
        </select>
      </label>

      {visibility === 'shared' && (
        <label className="block">
          <span className="mb-1 block text-sm font-medium text-neutral-700 dark:text-neutral-200">Freigabe</span>
          <select
            value={accessScopeId ?? ''}
            onChange={event => onChange({ visibility: 'shared', access_scope_id: event.target.value || null })}
            className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
          >
            <option value="">Freigabe wählen</option>
            {(data?.scopes ?? []).map(scope => (
              <option key={scope.id} value={scope.id}>
                {scope.label?.trim() || `Freigabe ${scope.id.slice(0, 8)}`}
              </option>
            ))}
          </select>
        </label>
      )}
    </div>
  )
}
