import { useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { addGovernanceRole, addTargetCollectionMember, createGovernanceScope, getTargetCollection, removeTargetCollectionMember } from '@/api/targetObjects'
import CollectionMemberPicker from '@/components/target-objects/CollectionMemberPicker'

const MEMBER_KIND_LABELS: Record<string, string> = {
  course: 'Segment/Kurs',
  route_publication: 'Veröffentlichter Kurs',
  place: 'Ort',
  region: 'Bereich',
  collection: 'Sammlung',
  privacy_zone: 'Privacy-Zone',
  tag_zone: 'Tag-Zone',
  external_ref: 'Externe Referenz',
}

export default function TargetCollectionDetailPage() {
  const { id = '' } = useParams()
  const queryClient = useQueryClient()
  const detail = useQuery({
    queryKey: ['target-collection', id],
    queryFn: () => getTargetCollection(id, { include_members: true }),
    enabled: Boolean(id),
  })
  const addMemberMutation = useMutation({
    mutationFn: (member: Parameters<typeof addTargetCollectionMember>[1]) => addTargetCollectionMember(id, member),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['target-collection', id] }),
  })
  const removeMemberMutation = useMutation({
    mutationFn: (memberID: string) => removeTargetCollectionMember(id, memberID),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['target-collection', id] }),
  })
  const bootstrapGovernanceMutation = useMutation({
    mutationFn: async () => {
      const scope = await createGovernanceScope({ kind: 'collection_scope', anchor_collection_id: id, owner_scope: 'user', visibility: 'private', origin_key: 'manual' })
      const scopeID = String((scope.scope as { id?: string }).id ?? '')
      if (scopeID) {
        await addGovernanceRole(scopeID, { actor_type: 'user', role: 'maintainer', permissions: ['manage_members', 'manage_roles'] })
      }
      return scope
    },
  })

  const collection = detail.data?.collection

  if (detail.isLoading) return <p className="text-sm text-neutral-500">Lädt...</p>
  if (!collection) return <p className="text-sm text-neutral-500">Sammlung nicht gefunden.</p>

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-normal">{collection.name}</h1>
          <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">{collection.visibility} · {collection.owner_member_count ?? collection.members?.length ?? 0} Einträge</p>
        </div>
        <button
          onClick={() => bootstrapGovernanceMutation.mutate()}
          disabled={bootstrapGovernanceMutation.isPending}
          className="rounded-md border border-neutral-300 px-3 py-2 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
        >
          Governance vorbereiten
        </button>
      </div>

      <CollectionMemberPicker onAdd={member => addMemberMutation.mutate(member)} busy={addMemberMutation.isPending} />

      <section className="space-y-3">
        <h2 className="text-lg font-semibold">Einträge</h2>
        <div className="divide-y divide-neutral-200 rounded-lg border border-neutral-200 dark:divide-neutral-800 dark:border-neutral-800">
          {(collection.members ?? []).map(member => (
            <div key={member.id} className="flex items-center justify-between gap-3 p-3">
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{member.display_name_override || member.member_id || member.member_ref}</p>
                <p className="text-xs text-neutral-500 dark:text-neutral-400">
                  {MEMBER_KIND_LABELS[member.member_kind] ?? member.member_kind} · {member.role}
                </p>
              </div>
              <button
                onClick={() => removeMemberMutation.mutate(member.id)}
                className="rounded-md border border-neutral-300 px-3 py-1.5 text-sm dark:border-neutral-700"
              >
                Entfernen
              </button>
            </div>
          ))}
          {(collection.members ?? []).length === 0 && <p className="p-3 text-sm text-neutral-500">Keine Einträge.</p>}
        </div>
      </section>
    </div>
  )
}
