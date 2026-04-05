import { useState, useRef, useEffect, type CSSProperties } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useClickOutside } from '@/hooks/useClickOutside'
import { fetchTags, createTag, unassignActivitiesFromTag, assignActivitiesToTag } from '@/api/tags'
import { updateActivity } from '@/api/activities'
import { getTagBadgeStyle } from '@/lib/tagUtils'
import { fetchRules } from '@/api/rules'
import type { AutomationRule } from '@/api/types'
import { haversineM } from '@/lib/geoUtils'
import CommuteModal from './CommuteModal'

interface TagsBadgesProps {
  tags: string[]
  activityId: string
  isVirtual?: boolean
  hasGps?: boolean
  distanceM?: number | null
  startLat?: number | null
  startLon?: number | null
  endLat?: number | null
  endLon?: number | null
}

function commuteRouteExists(rule: AutomationRule, sLat: number, sLon: number, eLat: number, eLon: number): boolean {
  if (!rule.branches) return false
  const threshold = 200
  for (const branch of rule.branches) {
    if (!branch.condition_groups?.[0]?.conditions) continue
    let bsLat = 0, bsLon = 0, beLat = 0, beLon = 0
    let hasStart = false, hasEnd = false
    for (const c of branch.condition_groups[0].conditions) {
      if (c.field !== 'geofence' || !c.value || typeof c.value !== 'object') continue
      const v = c.value as Record<string, unknown>
      if (v['check'] === 'start') { bsLat = v['lat'] as number; bsLon = v['lon'] as number; hasStart = true }
      if (v['check'] === 'end') { beLat = v['lat'] as number; beLon = v['lon'] as number; hasEnd = true }
    }
    if (!hasStart || !hasEnd) continue
    if ((haversineM(sLat, sLon, bsLat, bsLon) <= threshold && haversineM(eLat, eLon, beLat, beLon) <= threshold) ||
        (haversineM(sLat, sLon, beLat, beLon) <= threshold && haversineM(eLat, eLon, bsLat, bsLon) <= threshold)) {
      return true
    }
  }
  return false
}

export default function TagsBadges({ tags, activityId, isVirtual, hasGps, distanceM, startLat, startLon, endLat, endLon }: TagsBadgesProps) {
  const queryClient = useQueryClient()
  const [addOpen, setAddOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [showCommuteModal, setShowCommuteModal] = useState(false)
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
    onSuccess: async (updated, tagName) => {
      queryClient.setQueryData(['activity', activityId], updated)
      setSearch('')
      setAddOpen(false)

      // Show commute modal when Pendeln tag is added to GPS activity with sufficient distance
      if (tagName.toLowerCase() === 'pendeln' && hasGps && (distanceM ?? 0) >= 400
          && startLat != null && startLon != null && endLat != null && endLon != null) {
        try {
          const allRules = await fetchRules()
          const commuteRule = allRules.find(r => r.system_type === 'commute_auto')
          if (!commuteRule || !commuteRouteExists(commuteRule, startLat, startLon, endLat, endLon)) {
            setShowCommuteModal(true)
          }
        } catch {
          setShowCommuteModal(true)
        }
      }
    },
  })

  const createAndAddMut = useMutation({
    mutationFn: async (name: string) => {
      await createTag({ name })
      return updateActivity(activityId, { tags: [...tags, name] })
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
      {/* Add tag button — left of first tag */}
      <div className="relative" ref={popoverRef}>
        <button
          onClick={() => setAddOpen(!addOpen)}
          className="inline-flex items-center justify-center w-7 h-7 rounded-full text-xs font-medium cursor-pointer
                     bg-neutral-100 text-neutral-500 hover:bg-neutral-200 dark:bg-neutral-800 dark:text-neutral-400 dark:hover:bg-neutral-700 transition-colors"
          title="Tag hinzufügen"
        >
          +
        </button>

        {addOpen && (
          <div className="absolute left-0 top-full mt-1 z-50 w-56 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-lg">
            <div className="p-2">
              <input
                ref={searchRef}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Tag suchen..."
                className="w-full px-2 py-1.5 text-sm rounded border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400"
              />
            </div>
            <div className="max-h-48 overflow-y-auto">
              {filtered.map((t) => (
                <button
                  key={t.id}
                  onClick={() => addMut.mutate(t.name)}
                  className="w-full text-left px-3 py-1.5 text-sm text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer flex items-center justify-between"
                >
                  <span>{t.name}</span>
                  <span className="text-xs text-neutral-400">{t.activity_count}</span>
                </button>
              ))}
              {showCreate && (
                <button
                  onClick={() => createAndAddMut.mutate(searchTrimmed)}
                  className="w-full text-left px-3 py-1.5 text-sm text-accent-600 dark:text-orange-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer"
                >
                  &ldquo;{searchTrimmed}&rdquo; erstellen
                </button>
              )}
              {filtered.length === 0 && !showCreate && (
                <div className="px-3 py-2 text-sm text-neutral-400">Keine Tags gefunden</div>
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
            className="group inline-flex items-center px-2.5 py-1 rounded-full text-xs font-medium"
            style={getTagStyle(tag, i)}
          >
            {tag}
            {!isHardcoded && (
              <button
                onClick={() => removeMut.mutate(tag)}
                className="text-xs ml-1.5 opacity-0 group-hover:opacity-100 cursor-pointer transition-opacity"
                title={`Tag "${tag}" entfernen`}
              >
                &times;
              </button>
            )}
          </span>
        )
      })}

      {showCommuteModal && (
        <CommuteModal
          activityId={activityId}
          onClose={() => setShowCommuteModal(false)}
        />
      )}
    </>
  )
}
