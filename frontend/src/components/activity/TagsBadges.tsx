import { useState, useRef, useEffect, type CSSProperties } from 'react'
import { useTranslation } from 'react-i18next'

const COMMUTE_RADIUS_M = 200
import { haversineM } from '@/lib/geoUtils'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useClickOutside } from '@/hooks/useClickOutside'
import { fetchTags, createTag, unassignActivitiesFromTag, assignActivitiesToTag } from '@/api/tags'
import { updateActivity } from '@/api/activities'
import { getTagBadgeStyle } from '@/lib/tagUtils'
import CommuteModal from './CommuteModal'

interface CommuteContext {
  hasGps: boolean
  distanceM: number
  startLat: number
  startLon: number
  endLat: number
  endLon: number
}

interface TagsBadgesProps {
  tags: string[]
  activityId: string
  isVirtual?: boolean
  commuteContext?: CommuteContext
}

export default function TagsBadges({ tags, activityId, isVirtual, commuteContext }: TagsBadgesProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [addOpen, setAddOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [showCommuteModal, setShowCommuteModal] = useState(false)
  const [commuteRoundTrip, setCommuteRoundTrip] = useState(false)
  const popoverRef = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)

  const { data: allTags } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
    staleTime: 60_000, // 1 min — avoids redundant refetches while staying on the page
  })

  const fallbackColors = ['#F97316', '#3B82F6', '#22C55E', '#A855F7', '#F43F5E', '#06B6D4']

  function getTagStyle(tagName: string, index: number): CSSProperties {
    const tagObj = (allTags ?? []).find(t => t.name.toLowerCase() === tagName.toLowerCase())
    if (tagObj?.color) return getTagBadgeStyle(tagObj.color)
    return getTagBadgeStyle(fallbackColors[index % fallbackColors.length])
  }

  const removeMut = useMutation({
    mutationFn: async (tagToRemove: string) => {
      // Read fresh data from cache to avoid stale closure over allTags.
      // Tags with id === '' are "orphan" tags (in activities.tags TEXT[] but not in tags table).
      const currentTags = queryClient.getQueryData<typeof allTags>(['tags']) ?? []
      const tagObj = currentTags.find(t => t.name.toLowerCase() === tagToRemove.toLowerCase())
      if (tagObj && tagObj.id) {
        // Proper removal: delete from activity_tags junction table + sync TEXT[] cache.
        await unassignActivitiesFromTag(tagObj.id, [activityId])
        // Return fresh activity data for cache update.
        return updateActivity(activityId, {})
      } else {
        // Orphan tag (not in tags table): update TEXT[] cache directly.
        return updateActivity(activityId, { tags: tags.filter(t => t !== tagToRemove) })
      }
    },
    onSuccess: (updated) => {
      queryClient.setQueryData(['activity', activityId], updated)
      queryClient.invalidateQueries({ queryKey: ['tags'] })
    },
  })

  const addMut = useMutation({
    mutationFn: async (tagToAdd: string) => {
      // Read fresh data from cache to avoid stale closure over allTags.
      const currentTags = queryClient.getQueryData<typeof allTags>(['tags']) ?? []
      const tagObj = currentTags.find(t => t.name.toLowerCase() === tagToAdd.toLowerCase())
      if (tagObj?.id) {
        // Existing tag → use proper API (clears removal records)
        await assignActivitiesToTag(tagObj.id, [activityId])
        return updateActivity(activityId, {})
      }
      // Orphan tag or edge case → direct TEXT[] update
      return updateActivity(activityId, { tags: [...tags, tagToAdd] })
    },
    onSuccess: (updated, tagName) => {
      queryClient.setQueryData(['activity', activityId], updated)
      setSearch('')
      setAddOpen(false)

      // Show commute modal when Pendeln tag is added to GPS activity with sufficient distance
      if (tagName.toLowerCase() === 'pendeln' && commuteContext && commuteContext.distanceM >= 400) {
        const startEndDist = haversineM(
          commuteContext.startLat, commuteContext.startLon,
          commuteContext.endLat, commuteContext.endLon
        )
        if (startEndDist > COMMUTE_RADIUS_M) {
          setShowCommuteModal(true)
          setCommuteRoundTrip(false)
        } else {
          // Round-trip: offer dwell-based rule with destination zone picker
          setShowCommuteModal(true)
          setCommuteRoundTrip(true)
        }
      }
    },
  })

  const createAndAddMut = useMutation({
    mutationFn: async (name: string) => {
      const tag = await createTag({ name })
      await assignActivitiesToTag(tag.id, [activityId])
      return updateActivity(activityId, {})
    },
    onSuccess: (updated) => {
      queryClient.setQueryData(['activity', activityId], updated)
      queryClient.invalidateQueries({ queryKey: ['tags'] })
      setSearch('')
      setAddOpen(false)
    },
  })

  // Close popover on click outside
  useClickOutside(popoverRef, () => { setAddOpen(false); setSearch('') }, addOpen)

  // Focus search on open
  useEffect(() => {
    if (addOpen) searchRef.current?.focus()
  }, [addOpen])

  const tagsLower = new Set(tags.map(t => t.toLowerCase()))
  const filtered = (allTags ?? [])
    .filter(t => !tagsLower.has(t.name.toLowerCase()))
    .filter(t => !search || t.name.toLowerCase().includes(search.toLowerCase()))
    .sort((a, b) => b.activity_count - a.activity_count)

  const searchTrimmed = search.trim()
  const exactMatch = (allTags ?? []).some(t => t.name.toLowerCase() === searchTrimmed.toLowerCase())
  const showCreate = searchTrimmed.length > 0 && !exactMatch

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        {/* Add tag button — left of first tag */}
        <div className="relative inline-flex" ref={popoverRef}>
          <button
            onClick={() => setAddOpen(!addOpen)}
            className="inline-flex h-7 w-7 items-center justify-center rounded-full bg-neutral-100 text-xs font-medium text-neutral-500 transition-colors hover:bg-neutral-200 dark:bg-neutral-800 dark:text-neutral-400 dark:hover:bg-neutral-700"
            title={t('tagsBadges.add')}
          >
            +
          </button>

          {addOpen && (
            <div className="absolute left-0 top-full z-50 mt-1 w-56 rounded-lg border border-neutral-200 bg-white shadow-lg dark:border-neutral-700 dark:bg-neutral-900">
              <div className="p-2">
                <input
                  ref={searchRef}
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder={t('tagsBadges.searchPlaceholder')}
                  className="w-full rounded border border-neutral-200 bg-transparent px-2 py-1.5 text-sm text-neutral-900 outline-none focus:border-accent-400 dark:border-neutral-700 dark:text-neutral-100"
                />
              </div>
              <div className="max-h-48 overflow-y-auto">
                {filtered.map((t) => (
                  <button
                    key={t.id}
                    onClick={() => addMut.mutate(t.name)}
                    className="flex w-full items-center justify-between px-3 py-1.5 text-left text-sm text-neutral-700 hover:bg-neutral-100 dark:text-neutral-300 dark:hover:bg-neutral-800"
                  >
                    <span>{t.name}</span>
                    <span className="text-xs text-neutral-400">{t.activity_count}</span>
                  </button>
                ))}
                {showCreate && (
                  <button
                    onClick={() => createAndAddMut.mutate(searchTrimmed)}
                    className="w-full px-3 py-1.5 text-left text-sm text-accent-600 hover:bg-neutral-100 dark:text-orange-400 dark:hover:bg-neutral-800"
                  >
                    {t('tagsBadges.create', { tag: searchTrimmed })}
                  </button>
                )}
                {filtered.length === 0 && !showCreate && (
                  <div className="px-3 py-2 text-sm text-neutral-400">{t('tagsBadges.empty')}</div>
                )}
              </div>
            </div>
          )}
        </div>

        {tags.map((tag, i) => {
          const isHardcoded = tag === 'virtuell' && isVirtual
          return (
            <span
              key={tag}
              className="group inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium"
              style={getTagStyle(tag, i)}
            >
              {tag}
              {!isHardcoded && (
                <button
                  onClick={() => removeMut.mutate(tag)}
                  className="ml-1.5 text-xs opacity-0 transition-opacity group-hover:opacity-100"
                  title={t('tagsBadges.remove', { tag })}
                >
                  &times;
                </button>
              )}
            </span>
          )
        })}
      </div>

      {showCommuteModal && (
        <CommuteModal
          activityId={activityId}
          isRoundTrip={commuteRoundTrip}
          onClose={() => setShowCommuteModal(false)}
        />
      )}
    </>
  )
}
