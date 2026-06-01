import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  createObjectAccessScope,
  listObjectAccessScopes,
  type CreateObjectAccessGrantRequest,
  type CreateObjectAccessPrincipalKind,
  type ObjectAccessLinkTokenValue,
} from '@/api/accessScopes'
import type { TargetVisibility } from '@/api/targetObjects'
import { targetVisibilityLabel } from './targetLabels'

interface VisibilityAccessFieldProps {
  visibility: TargetVisibility
  accessScopeId?: string | null
  onChange: (next: { visibility: TargetVisibility; access_scope_id?: string | null }) => void
}

const VISIBILITY_OPTIONS: TargetVisibility[] = ['private', 'shared', 'public']

export default function VisibilityAccessField({ visibility, accessScopeId, onChange }: VisibilityAccessFieldProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [newScopeLabel, setNewScopeLabel] = useState('')
  const [principalKind, setPrincipalKind] = useState<CreateObjectAccessPrincipalKind>('user')
  const [principalRef, setPrincipalRef] = useState('')
  const [createdLinkTokens, setCreatedLinkTokens] = useState<ObjectAccessLinkTokenValue[]>([])
  const { data } = useQuery({
    queryKey: ['object-access-scopes'],
    queryFn: listObjectAccessScopes,
    enabled: visibility === 'shared',
    staleTime: 60_000,
  })
  const createScopeMutation = useMutation({
    mutationFn: () => {
      const grant: CreateObjectAccessGrantRequest = principalKind === 'user'
        ? { principal_kind: 'user', principal_user_id: principalRef.trim() }
        : { principal_kind: 'link_token' }
      return createObjectAccessScope({
        scope_kind: 'manual',
        label: newScopeLabel.trim() || undefined,
        grants: [grant],
      })
    },
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ['object-access-scopes'] })
      onChange({ visibility: 'shared', access_scope_id: result.scope.id })
      setCreatedLinkTokens(result.link_tokens ?? [])
      setNewScopeLabel('')
      setPrincipalRef('')
    },
  })
  return (
    <div className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-[minmax(0,220px)_minmax(0,1fr)]">
      <label className="block">
        <span className="mb-1 block text-sm font-medium text-neutral-700 dark:text-neutral-200">{t('targetObjects.access.visibility')}</span>
        <select
          value={visibility}
          onChange={(event) => {
            const next = event.target.value as TargetVisibility
            onChange({ visibility: next, access_scope_id: next === 'shared' ? accessScopeId ?? null : null })
          }}
          className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
        >
          {VISIBILITY_OPTIONS.map(option => (
            <option key={option} value={option}>{targetVisibilityLabel(option, t)}</option>
          ))}
        </select>
      </label>

      {visibility === 'shared' && (
        <label className="block">
          <span className="mb-1 block text-sm font-medium text-neutral-700 dark:text-neutral-200">{t('targetObjects.access.accessScope')}</span>
          <select
            value={accessScopeId ?? ''}
            onChange={event => onChange({ visibility: 'shared', access_scope_id: event.target.value || null })}
            className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
          >
            <option value="">{t('targetObjects.access.chooseScope')}</option>
            {(data?.scopes ?? []).map(scope => (
              <option key={scope.id} value={scope.id}>
                {scope.label?.trim() || t('targetObjects.access.scopeFallback', { id: scope.id.slice(0, 8) })}
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
              <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-neutral-300">{t('targetObjects.access.newScope')}</span>
              <input
                value={newScopeLabel}
                onChange={event => setNewScopeLabel(event.target.value)}
                placeholder={t('targetObjects.access.namePlaceholder')}
                className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
              />
            </label>
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-neutral-300">{t('targetObjects.access.principalKind')}</span>
              <select
                value={principalKind}
                onChange={event => {
                  const next = event.target.value as CreateObjectAccessPrincipalKind
                  setPrincipalKind(next)
                  if (next === 'link_token') setPrincipalRef('')
                }}
                className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
              >
                <option value="user">{t('targetObjects.access.user')}</option>
                <option value="link_token">{t('targetObjects.access.link')}</option>
              </select>
            </label>
            {principalKind === 'user' ? (
              <label className="block">
                <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-neutral-300">{t('targetObjects.access.recipient')}</span>
                <input
                  value={principalRef}
                  onChange={event => setPrincipalRef(event.target.value)}
                  placeholder={t('targetObjects.access.userIdPlaceholder')}
                  className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
                />
              </label>
            ) : (
              <div aria-hidden="true" />
            )}
            <button
              type="button"
              onClick={() => createScopeMutation.mutate()}
              disabled={createScopeMutation.isPending || (principalKind === 'user' && !principalRef.trim())}
              className="self-end rounded-md border border-neutral-300 px-3 py-2 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
            >
              {t('targetObjects.access.create')}
            </button>
          </div>
          {createdLinkTokens.length > 0 && (
            <div className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-100">
              <div className="font-medium">{t('targetObjects.access.linkTokenCreated')}</div>
              <div className="mt-1 font-mono text-xs break-all">{createdLinkTokens[0].token}</div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
