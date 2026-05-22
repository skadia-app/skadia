import { useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { getMe } from '@/api/auth'
import {
  addTargetCollectionMember,
  deleteTargetCollection,
  getTargetCollection,
  removeTargetCollectionMember,
  updateTargetCollection,
  updateTargetCollectionMember,
  type TargetCollectionKind,
  type TargetVisibility,
} from '@/api/targetObjects'
import CollectionMemberPicker from '@/components/target-objects/CollectionMemberPicker'
import VisibilityAccessField from '@/components/target-objects/VisibilityAccessField'
import TargetOverlayTabs from '@/components/target-overlays/TargetOverlayTabs'
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
  const { addToast } = useToast()
  const [editing, setEditing] = useState(false)
  const { data: me } = useQuery({
    queryKey: ['me'],
    queryFn: getMe,
    staleTime: 60_000,
  })
  const detail = useQuery({
    queryKey: ['target-collection', id],
    queryFn: () => getTargetCollection(id, { include_members: true, include_overlay_summary: true }),
    enabled: Boolean(id),
  })
  const addMemberMutation = useMutation({
    mutationFn: (member: Parameters<typeof addTargetCollectionMember>[1]) => addTargetCollectionMember(id, member),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['target-collection', id] })
      addToast({ type: 'success', title: 'Eintrag hinzugefügt' })
    },
    onError: error => addToast({ type: 'error', title: 'Eintrag konnte nicht hinzugefügt werden', message: errorMessage(error), duration: 0 }),
  })
  const removeMemberMutation = useMutation({
    mutationFn: (memberID: string) => removeTargetCollectionMember(id, memberID),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['target-collection', id] })
      addToast({ type: 'success', title: 'Eintrag entfernt' })
    },
    onError: error => addToast({ type: 'error', title: 'Eintrag konnte nicht entfernt werden', message: errorMessage(error), duration: 0 }),
  })
  const updateMemberMutation = useMutation({
    mutationFn: ({ memberID, role }: { memberID: string; role: string }) => updateTargetCollectionMember(id, memberID, { role }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['target-collection', id] })
      addToast({ type: 'success', title: 'Eintrag aktualisiert' })
    },
    onError: error => addToast({ type: 'error', title: 'Eintrag konnte nicht aktualisiert werden', message: errorMessage(error), duration: 0 }),
  })
  const updateCollectionMutation = useMutation({
    mutationFn: (request: { name: string; collection_kind: TargetCollectionKind; visibility: TargetVisibility; access_scope_id?: string | null }) => {
      const ownerEdit = Boolean(me?.is_admin || me?.user_id === collection?.owner_user_id)
      return updateTargetCollection(id, {
        name: request.name,
        collection_kind: request.collection_kind,
        visibility: ownerEdit ? request.visibility : undefined,
        access_scope_id: ownerEdit ? request.access_scope_id : undefined,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['target-collection', id] })
      queryClient.invalidateQueries({ queryKey: ['target-collections'] })
      setEditing(false)
      addToast({ type: 'success', title: 'Sammlung gespeichert' })
    },
    onError: error => addToast({ type: 'error', title: 'Sammlung konnte nicht gespeichert werden', message: errorMessage(error), duration: 0 }),
  })
  const archiveCollectionMutation = useMutation({
    mutationFn: () => deleteTargetCollection(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['target-collection', id] })
      queryClient.invalidateQueries({ queryKey: ['target-collections'] })
      addToast({ type: 'success', title: 'Sammlung archiviert' })
    },
    onError: error => addToast({ type: 'error', title: 'Sammlung konnte nicht archiviert werden', message: errorMessage(error), duration: 0 }),
  })

  const collection = detail.data?.collection

  if (detail.isLoading) return <p className="text-sm text-neutral-500">Lädt...</p>
  if (!collection) return <p className="text-sm text-neutral-500">Sammlung nicht gefunden.</p>
  const effectivePermissions = new Set(collection.effective_permissions ?? [])
  const ownerEdit = Boolean(me?.is_admin || me?.user_id === collection.owner_user_id)
  const canManageMembers = Boolean(
    me?.is_admin ||
    me?.user_id === collection.owner_user_id ||
    effectivePermissions.has('manage_members'),
  )
  const canEditCollection = Boolean(me?.is_admin || me?.user_id === collection.owner_user_id || effectivePermissions.has('edit_metadata'))
  const canArchiveCollection = Boolean(me?.is_admin || me?.user_id === collection.owner_user_id || effectivePermissions.has('hide_or_archive'))
  const canManageReports = Boolean(
    me?.is_admin ||
    me?.user_id === collection.owner_user_id ||
    effectivePermissions.has('manage_reports'),
  )

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-normal">{collection.name}</h1>
          <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">{collection.visibility} · {collection.owner_member_count ?? collection.members?.length ?? 0} Einträge</p>
          {collection.overlay_summary && (
            <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
              {collection.overlay_summary.public_open_report_count} offene Meldungen · {collection.overlay_summary.public_comment_count} Kommentare
            </p>
          )}
        </div>
        {(canEditCollection || (canArchiveCollection && !collection.archived_at)) && (
          <div className="flex gap-2">
            {canEditCollection && (
              <button
                type="button"
                onClick={() => setEditing(current => !current)}
                disabled={updateCollectionMutation.isPending}
                className="rounded-md border border-neutral-300 px-3 py-2 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
              >
                {editing ? 'Bearbeiten schließen' : 'Bearbeiten'}
              </button>
            )}
            {canArchiveCollection && !collection.archived_at && (
              <button
                type="button"
                onClick={() => archiveCollectionMutation.mutate()}
                disabled={archiveCollectionMutation.isPending}
                className="rounded-md border border-neutral-300 px-3 py-2 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
              >
                {archiveCollectionMutation.isPending ? 'Archiviert...' : 'Archivieren'}
              </button>
            )}
          </div>
        )}
      </div>

      {editing && (
        <CollectionEditForm
          collection={collection}
          busy={updateCollectionMutation.isPending}
          showVisibility={ownerEdit}
          onSave={request => updateCollectionMutation.mutate(request)}
        />
      )}

      <PermissionAudit permissions={collection.effective_permissions ?? []} isOwner={Boolean(me?.user_id === collection.owner_user_id)} isAdmin={Boolean(me?.is_admin)} />

      {canManageMembers && (
        <CollectionMemberPicker onAdd={member => addMemberMutation.mutate(member)} busy={addMemberMutation.isPending} />
      )}

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
              {canManageMembers && (
                <div className="flex flex-wrap justify-end gap-2">
                  <MemberRoleEditor
                    memberID={member.id}
                    role={member.role}
                    busy={updateMemberMutation.isPending}
                    onSave={(memberID, role) => updateMemberMutation.mutate({ memberID, role })}
                  />
                  <button
                    onClick={() => removeMemberMutation.mutate(member.id)}
                    disabled={removeMemberMutation.isPending}
                    className="rounded-md border border-neutral-300 px-3 py-1.5 text-sm disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
                  >
                    {removeMemberMutation.isPending ? 'Entfernt...' : 'Entfernen'}
                  </button>
                </div>
              )}
            </div>
          ))}
          {(collection.members ?? []).length === 0 && <p className="p-3 text-sm text-neutral-500">Keine Einträge.</p>}
        </div>
      </section>

      <TargetOverlayTabs
        subjectKind="collection"
        subjectId={collection.id}
        canWrite={Boolean(me)}
        canManageReports={canManageReports}
      />
    </div>
  )
}

function CollectionEditForm({ collection, busy, showVisibility, onSave }: {
  collection: { name: string; collection_kind: TargetCollectionKind; visibility: TargetVisibility; access_scope_id?: string | null }
  busy: boolean
  showVisibility: boolean
  onSave: (request: { name: string; collection_kind: TargetCollectionKind; visibility: TargetVisibility; access_scope_id?: string | null }) => void
}) {
  const [name, setName] = useState(collection.name)
  const [kind, setKind] = useState<TargetCollectionKind>(collection.collection_kind)
  const [visibility, setVisibility] = useState<TargetVisibility>(collection.visibility)
  const [accessScopeID, setAccessScopeID] = useState<string | null>(collection.access_scope_id ?? null)

  return (
    <form
      className="space-y-4 rounded-lg border border-neutral-200 p-4 dark:border-neutral-800"
      onSubmit={(event) => {
        event.preventDefault()
        onSave({ name, collection_kind: kind, visibility, access_scope_id: accessScopeID })
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
      {showVisibility && (
        <VisibilityAccessField
          visibility={visibility}
          accessScopeId={accessScopeID}
          onChange={(next) => {
            setVisibility(next.visibility)
            setAccessScopeID(next.access_scope_id ?? null)
          }}
        />
      )}
      <button
        type="submit"
        disabled={busy || !name.trim() || (showVisibility && visibility === 'shared' && !accessScopeID)}
        className="rounded-md bg-accent-DEFAULT px-4 py-2 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-50"
      >
        {busy ? 'Speichert...' : 'Sammlung speichern'}
      </button>
    </form>
  )
}

function MemberRoleEditor({ memberID, role, busy, onSave }: { memberID: string; role: string; busy: boolean; onSave: (memberID: string, role: string) => void }) {
  const [nextRole, setNextRole] = useState(role)
  return (
    <form
      className="flex gap-2"
      onSubmit={(event) => {
        event.preventDefault()
        onSave(memberID, nextRole)
      }}
    >
      <input
        aria-label="Rolle"
        value={nextRole}
        onChange={event => setNextRole(event.target.value)}
        className="w-28 rounded-md border border-neutral-300 bg-white px-2 py-1.5 text-sm dark:border-neutral-700 dark:bg-neutral-900"
      />
      <button
        type="submit"
        disabled={busy || !nextRole.trim() || nextRole === role}
        className="rounded-md border border-neutral-300 px-3 py-1.5 text-sm disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
      >
        Speichern
      </button>
    </form>
  )
}

function PermissionAudit({ permissions, isOwner, isAdmin }: { permissions: string[]; isOwner: boolean; isAdmin: boolean }) {
  return (
    <section className="space-y-2 rounded-lg border border-neutral-200 p-4 dark:border-neutral-800">
      <h2 className="text-lg font-semibold">Berechtigungsstatus</h2>
      <p className="text-sm text-neutral-600 dark:text-neutral-300">
        {isAdmin
          ? 'Admin-Zugriff'
          : isOwner
            ? 'Eigene Sammlung'
            : permissions.length > 0
              ? permissions.join(', ')
              : 'Keine delegierten Rechte in der Detailantwort'}
      </p>
      <p className="text-xs text-neutral-500 dark:text-neutral-400">
        Governance-Verwaltung ist hier noch nicht vollständig verfügbar; angezeigt werden nur die effektiven Rechte aus der Detailantwort.
      </p>
    </section>
  )
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Bitte versuche es erneut.'
}
