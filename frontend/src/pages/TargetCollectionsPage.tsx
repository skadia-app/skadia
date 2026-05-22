import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import VisibilityAccessField from '@/components/target-objects/VisibilityAccessField'
import { createTargetCollection, listTargetCollections, type TargetCollectionKind, type TargetVisibility } from '@/api/targetObjects'
import { useToast } from '@/lib/toast'

const COLLECTION_KIND_LABELS: Record<string, string> = {
  private_map: 'Private Karte',
  public_pool: 'Öffentlicher Pool',
  venue_pool: 'Gelände-Pool',
  route_set: 'Routen-Set',
  course_set: 'Kurs-Set',
  rule_set: 'Regel-Set',
  privacy_set: 'Privacy-Set',
  custom: 'Eigene Sammlung',
}

export default function TargetCollectionsPage() {
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
      addToast({ type: 'success', title: 'Sammlung erstellt' })
    },
    onError: error => addToast({ type: 'error', title: 'Sammlung konnte nicht erstellt werden', message: errorMessage(error), duration: 0 }),
  })

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-normal">Sammlungen</h1>
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">Gruppen aus Segmenten, Routen, Orten, Bereichen und Referenzen.</p>
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
            <span className="mb-1 block text-sm font-medium text-neutral-700 dark:text-neutral-200">Name</span>
            <input
              value={name}
              onChange={event => setName(event.target.value)}
              required
              className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
            />
          </label>
          <label>
            <span className="mb-1 block text-sm font-medium text-neutral-700 dark:text-neutral-200">Typ</span>
            <select
              value={kind}
              onChange={event => setKind(event.target.value as TargetCollectionKind)}
              className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
            >
              {Object.entries(COLLECTION_KIND_LABELS).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
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
          {createMutation.isPending ? 'Erstellt...' : 'Sammlung erstellen'}
        </button>
      </form>

      <div className="divide-y divide-neutral-200 rounded-lg border border-neutral-200 dark:divide-neutral-800 dark:border-neutral-800">
        {(collections.data?.collections ?? []).map(collection => (
          <Link key={collection.id} to={`/collections/${collection.id}`} className="flex items-center justify-between gap-3 p-3 hover:bg-neutral-50 dark:hover:bg-neutral-900">
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">{collection.name}</p>
              <p className="text-xs text-neutral-500 dark:text-neutral-400">
                {COLLECTION_KIND_LABELS[collection.collection_kind] ?? collection.collection_kind} · {collection.owner_member_count ?? 0} Einträge
              </p>
            </div>
            <span className="text-sm text-neutral-500">Öffnen</span>
          </Link>
        ))}
      </div>
    </div>
  )
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Bitte versuche es erneut.'
}
