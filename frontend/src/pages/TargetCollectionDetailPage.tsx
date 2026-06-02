import { useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
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
import { memberKindLabel, targetVisibilityLabel } from '@/components/target-objects/targetLabels'
import TargetOverlayTabs from '@/components/target-overlays/TargetOverlayTabs'
import { useToast } from '@/lib/toast'

export default function TargetCollectionDetailPage() {
  const { t } = useTranslation()
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
      addToast({ type: 'success', title: t('targetCollectionDetail.memberAdded') })
    },
    onError: error => addToast({ type: 'error', title: t('targetCollectionDetail.memberAddFailed'), message: errorMessage(error, t), duration: 0 }),
  })
  const removeMemberMutation = useMutation({
    mutationFn: (memberID: string) => removeTargetCollectionMember(id, memberID),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['target-collection', id] })
      addToast({ type: 'success', title: t('targetCollectionDetail.memberRemoved') })
    },
    onError: error => addToast({ type: 'error', title: t('targetCollectionDetail.memberRemoveFailed'), message: errorMessage(error, t), duration: 0 }),
  })
  const updateMemberMutation = useMutation({
    mutationFn: ({ memberID, role }: { memberID: string; role: string }) => updateTargetCollectionMember(id, memberID, { role }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['target-collection', id] })
      addToast({ type: 'success', title: t('targetCollectionDetail.memberUpdated') })
    },
    onError: error => addToast({ type: 'error', title: t('targetCollectionDetail.memberUpdateFailed'), message: errorMessage(error, t), duration: 0 }),
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
      addToast({ type: 'success', title: t('targetCollectionDetail.saved') })
    },
    onError: error => addToast({ type: 'error', title: t('targetCollectionDetail.saveFailed'), message: errorMessage(error, t), duration: 0 }),
  })
  const archiveCollectionMutation = useMutation({
    mutationFn: () => deleteTargetCollection(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['target-collection', id] })
      queryClient.invalidateQueries({ queryKey: ['target-collections'] })
      addToast({ type: 'success', title: t('targetCollectionDetail.archived') })
    },
    onError: error => addToast({ type: 'error', title: t('targetCollectionDetail.archiveFailed'), message: errorMessage(error, t), duration: 0 }),
  })

  const collection = detail.data?.collection

  if (detail.isLoading) return <p className="text-sm text-neutral-500">{t('targetCollectionDetail.loading')}</p>
  if (!collection) return <p className="text-sm text-neutral-500">{t('targetCollectionDetail.notFound')}</p>
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
          <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
            {targetVisibilityLabel(collection.visibility, t)} · {t('targetCollections.entryCount', { count: collection.owner_member_count ?? collection.members?.length ?? 0 })}
          </p>
          {collection.overlay_summary && (
            <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
              {t('targetCollectionDetail.overlaySummary', {
                reports: collection.overlay_summary.public_open_report_count,
                comments: collection.overlay_summary.public_comment_count,
              })}
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
                {editing ? t('targetCollectionDetail.closeEdit') : t('targetCollectionDetail.edit')}
              </button>
            )}
            {canArchiveCollection && !collection.archived_at && (
              <button
                type="button"
                onClick={() => archiveCollectionMutation.mutate()}
                disabled={archiveCollectionMutation.isPending}
                className="rounded-md border border-neutral-300 px-3 py-2 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
              >
                {archiveCollectionMutation.isPending ? t('targetCollectionDetail.archiving') : t('targetCollectionDetail.archive')}
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
        <h2 className="text-lg font-semibold">{t('targetCollectionDetail.entries')}</h2>
        <div className="divide-y divide-neutral-200 rounded-lg border border-neutral-200 dark:divide-neutral-800 dark:border-neutral-800">
          {(collection.members ?? []).map(member => (
            <div key={member.id} className="flex items-center justify-between gap-3 p-3">
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{member.display_name_override || member.member_id || member.member_ref}</p>
                <p className="text-xs text-neutral-500 dark:text-neutral-400">
                  {memberKindLabel(member.member_kind, t)} · {member.role}
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
                    {removeMemberMutation.isPending ? t('targetCollectionDetail.removing') : t('targetCollectionDetail.remove')}
                  </button>
                </div>
              )}
            </div>
          ))}
          {(collection.members ?? []).length === 0 && <p className="p-3 text-sm text-neutral-500">{t('targetCollectionDetail.noEntries')}</p>}
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
  const { t } = useTranslation()
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
        {busy ? t('targetCollectionDetail.saving') : t('targetCollectionDetail.save')}
      </button>
    </form>
  )
}

function MemberRoleEditor({ memberID, role, busy, onSave }: { memberID: string; role: string; busy: boolean; onSave: (memberID: string, role: string) => void }) {
  const { t } = useTranslation()
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
        aria-label={t('targetCollectionDetail.role')}
        value={nextRole}
        onChange={event => setNextRole(event.target.value)}
        className="w-28 rounded-md border border-neutral-300 bg-white px-2 py-1.5 text-sm dark:border-neutral-700 dark:bg-neutral-900"
      />
      <button
        type="submit"
        disabled={busy || !nextRole.trim() || nextRole === role}
        className="rounded-md border border-neutral-300 px-3 py-1.5 text-sm disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
      >
        {t('targetCollectionDetail.saveRole')}
      </button>
    </form>
  )
}

function PermissionAudit({ permissions, isOwner, isAdmin }: { permissions: string[]; isOwner: boolean; isAdmin: boolean }) {
  const { t } = useTranslation()
  return (
    <section className="space-y-2 rounded-lg border border-neutral-200 p-4 dark:border-neutral-800">
      <h2 className="text-lg font-semibold">{t('targetCollectionDetail.permissionStatus')}</h2>
      <p className="text-sm text-neutral-600 dark:text-neutral-300">
        {isAdmin
          ? t('targetCollectionDetail.adminAccess')
          : isOwner
            ? t('targetCollectionDetail.ownCollection')
            : permissions.length > 0
              ? permissions.join(', ')
              : t('targetCollectionDetail.noDelegatedPermissions')}
      </p>
      <p className="text-xs text-neutral-500 dark:text-neutral-400">
        {t('targetCollectionDetail.permissionNote')}
      </p>
    </section>
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
  return error instanceof Error ? error.message : t('targetCollectionDetail.tryAgain')
}
