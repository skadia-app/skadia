import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import VisibilityAccessField from '@/components/target-objects/VisibilityAccessField'
import { collectionKindLabel } from '@/components/target-objects/targetLabels'
import { createTargetCollection, listTargetCollections, type TargetCollectionKind, type TargetVisibility } from '@/api/targetObjects'
import { useToast } from '@/lib/toast'

export default function TargetCollectionsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const [name, setName] = useState('')
  const [kind, setKind] = useState<TargetCollectionKind>('private_map')
  const [visibility, setVisibility] = useState<TargetVisibility>('private')
  const [accessScopeID, setAccessScopeID] = useState<string | null>(null)
  const collections = useQuery({ queryKey: ['target-collections'], queryFn: () => listTargetCollections({ limit: 100 }) })
  const createMutation = useMutation({
    mutationFn: () => createTargetCollection({ name, collection_kind: kind, visibility, access_scope_id: accessScopeID, origin_key: 'manual' }),
    onSuccess: () => {
      setName('')
      queryClient.invalidateQueries({ queryKey: ['target-collections'] })
      addToast({ type: 'success', title: t('targetCollections.created') })
    },
    onError: error => addToast({ type: 'error', title: t('targetCollections.createFailed'), message: errorMessage(error, t), duration: 0 }),
  })

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-normal">{t('targetCollections.title')}</h1>
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">{t('targetCollections.description')}</p>
      </div>

      <form
        className="space-y-4 rounded-lg border border-neutral-200 p-4 dark:border-neutral-800"
        onSubmit={(event) => {
          event.preventDefault()
          createMutation.mutate()
        }}
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <label>
            <span className="mb-1 block text-sm font-medium text-neutral-700 dark:text-neutral-200">{t('targetCollections.name')}</span>
            <input
              value={name}
              onChange={event => setName(event.target.value)}
              required
              className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
            />
          </label>
          <label>
            <span className="mb-1 block text-sm font-medium text-neutral-700 dark:text-neutral-200">{t('targetCollections.type')}</span>
            <select
              value={kind}
              onChange={event => setKind(event.target.value as TargetCollectionKind)}
              className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
            >
              {collectionKindValues.map(value => <option key={value} value={value}>{t(`targetCollections.kinds.${value}`)}</option>)}
            </select>
          </label>
        </div>
        <VisibilityAccessField
          visibility={visibility}
          accessScopeId={accessScopeID}
          onChange={(next) => {
            setVisibility(next.visibility)
            setAccessScopeID(next.access_scope_id ?? null)
          }}
        />
        <button
          type="submit"
          disabled={createMutation.isPending || !name.trim() || (visibility === 'shared' && !accessScopeID)}
          className="rounded-md bg-accent-DEFAULT px-4 py-2 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-50"
        >
          {createMutation.isPending ? t('targetCollections.creating') : t('targetCollections.create')}
        </button>
      </form>

      <div className="divide-y divide-neutral-200 rounded-lg border border-neutral-200 dark:divide-neutral-800 dark:border-neutral-800">
        {(collections.data?.collections ?? []).map(collection => (
          <Link key={collection.id} to={`/collections/${collection.id}`} className="flex items-center justify-between gap-3 p-3 hover:bg-neutral-50 dark:hover:bg-neutral-900">
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">{collection.name}</p>
              <p className="text-xs text-neutral-500 dark:text-neutral-400">
                {collectionKindLabel(collection.collection_kind, t)} · {t('targetCollections.entryCount', { count: collection.owner_member_count ?? 0 })}
              </p>
            </div>
            <span className="text-sm text-neutral-500">{t('targetCollections.open')}</span>
          </Link>
        ))}
      </div>
    </div>
  )
}

const collectionKindValues: TargetCollectionKind[] = [
  'private_map',
  'public_pool',
  'venue_pool',
  'route_set',
  'course_set',
  'rule_set',
  'privacy_set',
  'custom',
]

function errorMessage(error: unknown, t: ReturnType<typeof useTranslation>['t']): string {
  return error instanceof Error ? error.message : t('targetCollections.tryAgain')
}
