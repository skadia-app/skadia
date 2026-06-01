import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { fetchSegments } from '@/api/segments'
import { listRoutePublications } from '@/api/routes'
import { listPrivacyZones } from '@/api/privacy'
import { fetchTags } from '@/api/tags'
import { listTargetCollections, listTargetPlaces, listTargetRegions, type TargetCollectionMemberKind } from '@/api/targetObjects'
import { memberKindLabel } from './targetLabels'

interface CollectionMemberPickerProps {
  onAdd: (member: { member_kind: TargetCollectionMemberKind; member_id?: string; member_ref?: string; role: string }) => void
  busy?: boolean
}

const MEMBER_KIND_OPTIONS: TargetCollectionMemberKind[] = ['course', 'route_publication', 'place', 'region', 'collection', 'privacy_zone', 'tag_zone', 'external_ref']

export default function CollectionMemberPicker({ onAdd, busy = false }: CollectionMemberPickerProps) {
  const { t } = useTranslation()
  const [memberKind, setMemberKind] = useState<TargetCollectionMemberKind>('place')
  const [memberID, setMemberID] = useState('')
  const [memberRef, setMemberRef] = useState('')
  const [role, setRole] = useState('member')

  const segments = useQuery({ queryKey: ['target-member-segments'], queryFn: () => fetchSegments({ limit: 100 }), enabled: memberKind === 'course' })
  const publications = useQuery({ queryKey: ['target-member-publications'], queryFn: () => listRoutePublications({ limit: 100 }), enabled: memberKind === 'route_publication' })
  const places = useQuery({ queryKey: ['target-member-places'], queryFn: () => listTargetPlaces({ limit: 100 }), enabled: memberKind === 'place' })
  const regions = useQuery({ queryKey: ['target-member-regions'], queryFn: () => listTargetRegions({ limit: 100 }), enabled: memberKind === 'region' })
  const collections = useQuery({ queryKey: ['target-member-collections'], queryFn: () => listTargetCollections({ limit: 100 }), enabled: memberKind === 'collection' })
  const privacyZones = useQuery({ queryKey: ['target-member-privacy-zones'], queryFn: listPrivacyZones, enabled: memberKind === 'privacy_zone' })
  const tags = useQuery({ queryKey: ['target-member-tags'], queryFn: fetchTags, enabled: memberKind === 'tag_zone' })

  const options = useMemo(() => {
    switch (memberKind) {
      case 'course':
        return (segments.data?.segments ?? []).map(item => ({ id: item.id, label: item.name }))
      case 'route_publication':
        return (publications.data?.courses ?? []).map(item => ({ id: item.publication_id, label: item.name }))
      case 'place':
        return (places.data?.places ?? []).map(item => ({ id: item.id, label: item.name }))
      case 'region':
        return (regions.data?.regions ?? []).map(item => ({ id: item.id, label: item.name }))
      case 'collection':
        return (collections.data?.collections ?? []).map(item => ({ id: item.id, label: item.name }))
      case 'privacy_zone':
        return (privacyZones.data ?? []).map(item => ({ id: item.id, label: item.name }))
      case 'tag_zone':
        return (tags.data ?? []).filter(item => item.zone?.id).map(item => ({ id: item.zone!.id, label: item.name }))
      default:
        return []
    }
  }, [collections.data, memberKind, places.data, privacyZones.data, publications.data, regions.data, segments.data, tags.data])

  const isExternal = memberKind === 'external_ref'
  const canSubmit = isExternal ? memberRef.trim() !== '' : memberID.trim() !== ''

  return (
    <form
      className="grid gap-3 rounded-lg border border-neutral-200 p-4 dark:border-neutral-800 md:grid-cols-[180px_minmax(0,1fr)_140px_auto]"
      onSubmit={(event) => {
        event.preventDefault()
        onAdd({
          member_kind: memberKind,
          member_id: isExternal ? undefined : memberID,
          member_ref: isExternal ? memberRef : undefined,
          role,
        })
        setMemberID('')
        setMemberRef('')
      }}
    >
      <select
        value={memberKind}
        onChange={(event) => {
          setMemberKind(event.target.value as TargetCollectionMemberKind)
          setMemberID('')
          setMemberRef('')
        }}
        className="rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
      >
        {MEMBER_KIND_OPTIONS.map(option => <option key={option} value={option}>{memberKindLabel(option, t)}</option>)}
      </select>
      {isExternal ? (
        <input
          value={memberRef}
          onChange={event => setMemberRef(event.target.value)}
          placeholder={t('targetCollectionDetail.memberRefPlaceholder')}
          className="rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
        />
      ) : (
        <select
          value={memberID}
          onChange={event => setMemberID(event.target.value)}
          className="rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
        >
          <option value="">{t('targetCollectionDetail.chooseMember')}</option>
          {options.map(option => <option key={option.id} value={option.id}>{option.label}</option>)}
        </select>
      )}
      <input
        value={role}
        onChange={event => setRole(event.target.value)}
        className="rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
      />
      <button
        type="submit"
        disabled={busy || !canSubmit}
        className="rounded-md border border-neutral-300 px-3 py-2 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
      >
        {t('targetCollectionDetail.add')}
      </button>
    </form>
  )
}
