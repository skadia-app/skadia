import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createObjectAccessScope, listObjectAccessScopes, updateObjectAccessScope, type ObjectAccessPrincipalKind } from '@/api/accessScopes'
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
  const queryClient = useQueryClient()
  const [newScopeLabel, setNewScopeLabel] = useState('')
  const [principalKind, setPrincipalKind] = useState<ObjectAccessPrincipalKind>('user')
  const [principalRef, setPrincipalRef] = useState('')
  const { data } = useQuery({
    queryKey: ['object-access-scopes'],
    queryFn: listObjectAccessScopes,
    enabled: visibility === 'shared',
    staleTime: 60_000,
  })
  const createScopeMutation = useMutation({
    mutationFn: () => createObjectAccessScope({
      scope_kind: 'manual',
      label: newScopeLabel.trim() || undefined,
      grants: [{
        principal_kind: principalKind,
        principal_ref: principalRef.trim() || undefined,
      }],
    }),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ['object-access-scopes'] })
      onChange({ visibility: 'shared', access_scope_id: result.scope.id })
      setNewScopeLabel('')
      setPrincipalRef('')
    },
  })
  const revokeScopeMutation = useMutation({
    mutationFn: (scopeId: string) => updateObjectAccessScope(scopeId, { revoke_scope: true }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['object-access-scopes'] })
      onChange({ visibility: 'shared', access_scope_id: null })
    },
  })

  return (
    <div className="space-y-3">
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

      {visibility === 'shared' && (
        <div className="space-y-3 rounded-md border border-neutral-200 p-3 dark:border-neutral-800">
          <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,150px)_minmax(0,1fr)_auto]">
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-neutral-300">Neue Freigabe</span>
              <input
                value={newScopeLabel}
                onChange={event => setNewScopeLabel(event.target.value)}
                placeholder="Name"
                className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
              />
            </label>
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-neutral-300">Art</span>
              <select
                value={principalKind}
                onChange={event => setPrincipalKind(event.target.value as ObjectAccessPrincipalKind)}
                className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
              >
                <option value="user">Nutzer</option>
                <option value="group">Gruppe</option>
                <option value="team">Team</option>
                <option value="organization">Organisation</option>
                <option value="role">Rolle</option>
                <option value="link_token">Link</option>
              </select>
            </label>
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-neutral-300">Adressat</span>
              <input
                value={principalRef}
                onChange={event => setPrincipalRef(event.target.value)}
                placeholder="ID oder Referenz"
                className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
              />
            </label>
            <button
              type="button"
              onClick={() => createScopeMutation.mutate()}
              disabled={createScopeMutation.isPending || !principalRef.trim()}
              className="self-end rounded-md border border-neutral-300 px-3 py-2 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
            >
              Anlegen
            </button>
          </div>
          <button
            type="button"
            onClick={() => accessScopeId && revokeScopeMutation.mutate(accessScopeId)}
            disabled={!accessScopeId || revokeScopeMutation.isPending}
            className="rounded-md border border-red-300 px-3 py-1.5 text-sm text-red-600 disabled:cursor-not-allowed disabled:opacity-50 dark:border-red-800 dark:text-red-400"
          >
            Ausgewählte Freigabe deaktivieren
          </button>
        </div>
      )}
    </div>
  )
}
