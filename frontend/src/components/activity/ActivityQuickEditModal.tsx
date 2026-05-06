import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Save, X } from 'lucide-react'
import { fetchActivityStreams, updateActivity } from '@/api/activities'
import { assignActivitiesToTag, createTag, fetchTags, unassignActivitiesFromTag, type Tag } from '@/api/tags'
import type { Activity, StreamPoint } from '@/api/types'
import { ModalShell } from '@/components/ui/ModalShell'
import Toggle from '@/components/Toggle'
import SportTuplePicker from './SportTuplePicker'
import {
  activityQuickEditDraftFromActivity,
  buildActivityQuickEditPatch,
  diffQuickEditTags,
  quickEditTagDraftAfterModifier,
  quickEditTagDraftAfterToggle,
  type ActivityQuickEditDraft,
} from './quickEdit'
import { formatDistance, formatDuration, formatElevationGain } from '@/lib/utils'
import { lineStringToSvgPoints } from '@/lib/trackSvg'

interface ActivityQuickEditModalProps {
  activity: Activity
  open: boolean
  onClose: () => void
  onSaved: (activity: Activity) => void
  showMoreEdit?: boolean
}

const VISIBILITY_OPTIONS: Array<{ value: Activity['visibility']; label: string }> = [
  { value: 'private', label: 'Privat' },
  { value: 'bekannte', label: 'Bekannte' },
  { value: 'instanz', label: 'Instanz' },
  { value: 'federation', label: 'Federation' },
  { value: 'public', label: 'Öffentlich' },
]

const FEEL_LABELS = ['Schlecht', 'Mäßig', 'Okay', 'Gut', 'Super']

function tagByName(tags: Tag[], name: string): Tag | undefined {
  return tags.find(tag => tag.name.toLowerCase() === name.toLowerCase())
}

function pointsFromStreams(streams: StreamPoint[]): string | null {
  const coords = streams
    .filter(s => s.lat != null && s.lng != null)
    .map(s => [s.lng!, s.lat!] as [number, number])
  if (coords.length < 2) return null
  return lineStringToSvgPoints(JSON.stringify({ type: 'LineString', coordinates: coords }), 200, 12)
}

function hasPatchValues(patch: Record<string, unknown>): boolean {
  return Object.keys(patch).length > 0
}

export default function ActivityQuickEditModal({ activity, open, onClose, onSaved, showMoreEdit = true }: ActivityQuickEditModalProps) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<ActivityQuickEditDraft>(() => activityQuickEditDraftFromActivity(activity))
  const [tagSearch, setTagSearch] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (open) {
      setDraft(activityQuickEditDraftFromActivity(activity))
      setTagSearch('')
      setError(null)
    }
  }, [activity, open])

  const { data: allTags } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
    staleTime: 60_000,
    enabled: open,
  })

  const { data: streams = [], isLoading: streamsLoading } = useQuery({
    queryKey: ['activity-streams', activity.id],
    queryFn: () => fetchActivityStreams(activity.id),
    enabled: open,
  })

  const minimapPoints = useMemo(() => pointsFromStreams(streams), [streams])
  const visibleSuggestions = useMemo(() => {
    const selected = new Set(draft.tags.map(tag => tag.toLowerCase()))
    const q = tagSearch.trim().toLowerCase()
    return (allTags ?? [])
      .filter(tag => !selected.has(tag.name.toLowerCase()))
      .filter(tag => !q || tag.name.toLowerCase().includes(q))
      .slice(0, 20)
  }, [allTags, draft.tags, tagSearch])

  if (!open) return null

  function patchDraft(patch: Partial<ActivityQuickEditDraft>) {
    setDraft(prev => ({ ...prev, ...patch }))
  }

  function toggleTag(tagName: string, selected: boolean) {
    setDraft(prev => quickEditTagDraftAfterToggle(prev, tagName, selected))
  }

  async function syncTagDiffs(currentTags: string[], nextTags: string[], latestActivity: Activity, knownTags: Tag[]): Promise<Activity> {
    const diff = diffQuickEditTags(currentTags, nextTags)
    let updated = latestActivity

    for (const tagName of diff.remove) {
      const tag = tagByName(knownTags, tagName)
      if (tag?.id) {
        await unassignActivitiesFromTag(tag.id, [activity.id])
      } else {
        const remaining = updated.tags.filter(tagValue => tagValue.toLowerCase() !== tagName.toLowerCase())
        updated = await updateActivity(activity.id, { tags: remaining })
      }
    }

    for (const tagName of diff.add) {
      const tag = tagByName(knownTags, tagName) ?? await createTag({ name: tagName })
      await assignActivitiesToTag(tag.id, [activity.id])
    }

    if (diff.add.length > 0 || diff.remove.length > 0) {
      queryClient.invalidateQueries({ queryKey: ['tags'] })
      return updateActivity(activity.id, {})
    }
    return updated
  }

  async function handleSave() {
    setSaving(true)
    setError(null)
    try {
      const patch = buildActivityQuickEditPatch(activity, draft)
      const afterPatch = hasPatchValues(patch) ? await updateActivity(activity.id, patch) : activity
      const knownTags: Tag[] = allTags ?? await queryClient.fetchQuery<Tag[]>({ queryKey: ['tags'], queryFn: fetchTags })
      const updated = await syncTagDiffs(activity.tags ?? [], draft.tags, afterPatch, knownTags)
      queryClient.setQueryData(['activity', activity.id], updated)
      queryClient.invalidateQueries({ queryKey: ['activities'] })
      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
      onSaved(updated)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Speichern fehlgeschlagen')
    } finally {
      setSaving(false)
    }
  }

  const tagSearchTrimmed = tagSearch.trim()
  const canCreateTag = tagSearchTrimmed.length > 0 &&
    !(allTags ?? []).some(tag => tag.name.toLowerCase() === tagSearchTrimmed.toLowerCase()) &&
    !draft.tags.some(tag => tag.toLowerCase() === tagSearchTrimmed.toLowerCase())

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-5xl" className="max-h-[92vh] w-[calc(100vw-2rem)] overflow-y-auto">
      <div className="space-y-5">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Aktivität bearbeiten</h2>
            <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
              Änderungen werden erst beim Speichern übernommen.
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg p-1.5 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-neutral-800 dark:hover:text-neutral-200"
            aria-label="Schnellbearbeitung schließen"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="grid gap-5 lg:grid-cols-[1.4fr_0.8fr]">
          <div className="space-y-4">
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-neutral-400">Titel</span>
              <input
                value={draft.title}
                onChange={e => patchDraft({ title: e.target.value })}
                className="w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none focus:border-accent-400 dark:border-neutral-700 dark:bg-neutral-800 dark:text-neutral-100"
              />
            </label>

            <div className="grid gap-4 md:grid-cols-2">
              <label className="block">
                <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-neutral-400">Sportart</span>
                <SportTuplePicker
                  sportType={draft.sportType}
                  subType={draft.subType}
                  onChange={(sportType, subType) => patchDraft({ sportType, subType })}
                />
              </label>
              <label className="block">
                <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-neutral-400">Sichtbarkeit</span>
                <select
                  value={draft.visibility}
                  onChange={e => patchDraft({ visibility: e.target.value as Activity['visibility'] })}
                  className="w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none focus:border-accent-400 dark:border-neutral-700 dark:bg-neutral-800 dark:text-neutral-100"
                >
                  {VISIBILITY_OPTIONS.map(option => (
                    <option key={option.value} value={option.value}>{option.label}</option>
                  ))}
                </select>
              </label>
            </div>

            <div className="grid gap-3 md:grid-cols-3">
              {([
                ['isIndoor', 'Indoor', '#3B82F6'],
                ['isVirtual', 'Virtuell', '#8B5CF6'],
                ['isElectric', 'E-Antrieb', '#16A34A'],
              ] as const).map(([key, label, color]) => (
                <div key={key} className="flex items-center justify-between rounded-lg border border-neutral-200 px-3 py-2 dark:border-neutral-700">
                  <span className="text-sm text-neutral-700 dark:text-neutral-300">{label}</span>
                  <Toggle
                    checked={draft[key]}
                    onChange={checked => setDraft(prev => quickEditTagDraftAfterModifier(prev, key, checked))}
                    color={color}
                    size="sm"
                    aria-label={label}
                  />
                </div>
              ))}
            </div>

            <div>
              <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-neutral-400">Tags</span>
              <div className="rounded-lg border border-neutral-200 p-3 dark:border-neutral-700">
                <div className="mb-2 flex flex-wrap gap-2">
                  {draft.tags.map(tag => {
                    const locked = tag.toLowerCase() === 'virtuell' && draft.isVirtual
                    return (
                      <span key={tag} className="inline-flex items-center gap-1 rounded-full bg-neutral-100 px-2.5 py-1 text-xs font-medium text-neutral-700 dark:bg-neutral-800 dark:text-neutral-300">
                        {tag}
                        {!locked && (
                          <button type="button" onClick={() => toggleTag(tag, false)} className="text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-100" aria-label={`${tag} entfernen`}>
                            <X className="h-3 w-3" />
                          </button>
                        )}
                      </span>
                    )
                  })}
                  {draft.tags.length === 0 && (
                    <span className="text-sm text-neutral-400">Keine Tags</span>
                  )}
                </div>
                <div className="flex gap-2">
                  <input
                    value={tagSearch}
                    onChange={e => setTagSearch(e.target.value)}
                    placeholder="Tag suchen oder erstellen..."
                    className="min-w-0 flex-1 rounded border border-neutral-200 bg-transparent px-2 py-1.5 text-sm text-neutral-900 outline-none focus:border-accent-400 dark:border-neutral-700 dark:text-neutral-100"
                  />
                  {canCreateTag && (
                    <button type="button" onClick={() => { toggleTag(tagSearchTrimmed, true); setTagSearch('') }} className="rounded border border-neutral-200 px-3 py-1.5 text-sm hover:bg-neutral-50 dark:border-neutral-700 dark:hover:bg-neutral-800">
                      Erstellen
                    </button>
                  )}
                </div>
                {visibleSuggestions.length > 0 && (
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {visibleSuggestions.map(tag => (
                      <button key={tag.id || tag.name} type="button" onClick={() => { toggleTag(tag.name, true); setTagSearch('') }} className="rounded-full bg-neutral-100 px-2 py-1 text-xs text-neutral-600 hover:bg-neutral-200 dark:bg-neutral-800 dark:text-neutral-300 dark:hover:bg-neutral-700">
                        {tag.name}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            </div>

            <label className="block">
              <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-neutral-400">Beschreibung</span>
              <textarea
                value={draft.description}
                onChange={e => patchDraft({ description: e.target.value })}
                rows={4}
                className="w-full resize-y rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none focus:border-accent-400 dark:border-neutral-700 dark:bg-neutral-800 dark:text-neutral-100"
              />
            </label>

            <div className="grid gap-4 md:grid-cols-2">
              <label className="block">
                <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-neutral-400">RPE</span>
                <input
                  type="range"
                  min={1}
                  max={10}
                  value={draft.workoutRpe ?? 5}
                  onChange={e => patchDraft({ workoutRpe: Number(e.target.value) })}
                  className="w-full accent-accent-DEFAULT"
                />
                <div className="mt-1 flex items-center justify-between text-xs text-neutral-400">
                  <span>{draft.workoutRpe == null ? 'Nicht gesetzt' : `${draft.workoutRpe}/10`}</span>
                  <button type="button" onClick={() => patchDraft({ workoutRpe: null })} className="hover:text-neutral-600 dark:hover:text-neutral-200">Entfernen</button>
                </div>
              </label>
              <div>
                <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-neutral-400">Gefühl</span>
                <div className="flex flex-wrap gap-1.5">
                  {FEEL_LABELS.map((label, index) => (
                    <button
                      key={label}
                      type="button"
                      onClick={() => patchDraft({ workoutFeel: draft.workoutFeel === index ? null : index })}
                      className={`rounded-lg border px-2.5 py-1.5 text-xs ${draft.workoutFeel === index
                        ? 'border-accent-DEFAULT bg-accent-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300'
                        : 'border-neutral-200 text-neutral-600 hover:bg-neutral-50 dark:border-neutral-700 dark:text-neutral-300 dark:hover:bg-neutral-800'
                      }`}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              </div>
            </div>
          </div>

          <aside className="space-y-3">
            <div className="rounded-lg border border-neutral-200 p-3 dark:border-neutral-700">
              <div className="mb-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Minimap</div>
              <div className="flex aspect-video items-center justify-center overflow-hidden rounded bg-neutral-100 dark:bg-neutral-800">
                {streamsLoading ? (
                  <Loader2 className="h-5 w-5 animate-spin text-neutral-400" />
                ) : minimapPoints ? (
                  <svg viewBox="0 0 200 200" className="h-full w-full">
                    <polyline points={minimapPoints} fill="none" stroke="currentColor" strokeWidth="4" strokeLinecap="round" strokeLinejoin="round" className="text-accent-DEFAULT" />
                  </svg>
                ) : (
                  <span className="text-sm text-neutral-400">Kein GPS-Track</span>
                )}
              </div>
            </div>
            <div className="grid grid-cols-2 gap-2 text-sm">
              <Stat label="Distanz" value={formatDistance(activity.distance_m)} />
              <Stat label="Zeit" value={formatDuration(activity.elapsed_time_s ?? 0)} />
              <Stat label="Höhenmeter" value={formatElevationGain(activity.elevation_gain_m) ?? '—'} />
              <Stat label="GPS" value={activity.has_gps ? 'Ja' : 'Nein'} />
            </div>
          </aside>
        </div>

        {error && (
          <div className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300">
            {error}
          </div>
        )}

        <div className="flex flex-wrap justify-between gap-3 border-t border-neutral-100 pt-4 dark:border-neutral-800">
          {showMoreEdit ? (
            <button
              type="button"
              onClick={() => navigate(`/activities/${activity.id}/edit`)}
              className="rounded-lg border border-neutral-200 px-4 py-2 text-sm text-neutral-700 hover:bg-neutral-50 dark:border-neutral-700 dark:text-neutral-300 dark:hover:bg-neutral-800"
            >
              Mehr bearbeiten
            </button>
          ) : <span />}
          <div className="flex gap-2">
            <button type="button" onClick={onClose} disabled={saving} className="rounded-lg border border-neutral-200 px-4 py-2 text-sm text-neutral-700 hover:bg-neutral-50 disabled:opacity-50 dark:border-neutral-700 dark:text-neutral-300 dark:hover:bg-neutral-800">
              Abbrechen
            </button>
            <button type="button" onClick={handleSave} disabled={saving} className="inline-flex items-center gap-2 rounded-lg bg-accent-DEFAULT px-4 py-2 text-sm font-medium text-white hover:bg-accent-600 disabled:opacity-50">
              {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
              Speichern
            </button>
          </div>
        </div>
      </div>
    </ModalShell>
  )
}

function Stat({ label, value }: { label: string; value: string | null | undefined }) {
  return (
    <div className="rounded-lg border border-neutral-200 px-3 py-2 dark:border-neutral-700">
      <div className="text-[11px] text-neutral-400">{label}</div>
      <div className="mt-0.5 truncate text-sm font-medium text-neutral-800 dark:text-neutral-200">{value ?? '—'}</div>
    </div>
  )
}
