import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { assignActivitiesToTag, unassignActivitiesFromTag } from '@/api/tags'
import type { Tag } from '@/api/tags'
import { getTagBadgeStyle } from '@/lib/tagUtils'

interface Props {
  activityId: string
  currentTags: string[]
  locationTags: Tag[]
  onUpdate: () => void
}

export default function ActivityLocationTagSection({ activityId, currentTags, locationTags, onUpdate }: Props) {
  const queryClient = useQueryClient()
  const [showPicker, setShowPicker] = useState(false)

  // Tags that are both a location tag AND already assigned to this activity
  const assignedLocationTags = locationTags.filter(t => currentTags.includes(t.name))
  // Tags available to assign
  const availableTags = locationTags.filter(t => !currentTags.includes(t.name))

  const assignMutation = useMutation({
    mutationFn: (tagId: string) =>
      assignActivitiesToTag(tagId, [activityId]),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activity', activityId] })
      onUpdate()
      setShowPicker(false)
    },
  })

  const removeMutation = useMutation({
    mutationFn: (tagId: string) =>
      unassignActivitiesFromTag(tagId, [activityId]),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activity', activityId] })
      onUpdate()
    },
  })

  if (locationTags.length === 0) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4">
        <div className="flex items-center gap-2 mb-2">
          <svg className="w-4 h-4 text-neutral-400" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M9.69 18.933l.003.001C9.89 19.02 10 19 10 19s.11.02.308-.066l.002-.001.006-.003.018-.008a5.741 5.741 0 00.281-.14c.186-.096.446-.24.757-.433.62-.384 1.445-.966 2.274-1.765C15.302 15.957 17 13.514 17 10c0-3.866-3.582-7-8-7S2 6.134 2 10c0 3.514 1.698 5.957 3.355 7.584a13.731 13.731 0 002.273 1.765 11.842 11.842 0 00.757.433 5.766 5.766 0 00.282.14l.018.008.006.003zM10 11.25a1.25 1.25 0 100-2.5 1.25 1.25 0 000 2.5z" clipRule="evenodd" />
          </svg>
          <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Standort</h3>
        </div>
        <p className="text-xs text-neutral-500 dark:text-neutral-400">
          Noch keine Standort-Tags vorhanden. Erstelle einen in{' '}
          <a href="/workshop/tags" className="text-accent-DEFAULT hover:underline">Tags</a> oder
          über das Indoor-Panel auf der{' '}
          <a href="/map" className="text-accent-DEFAULT hover:underline">Karte</a>.
        </p>
      </div>
    )
  }

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <svg className="w-4 h-4 text-neutral-400" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M9.69 18.933l.003.001C9.89 19.02 10 19 10 19s.11.02.308-.066l.002-.001.006-.003.018-.008a5.741 5.741 0 00.281-.14c.186-.096.446-.24.757-.433.62-.384 1.445-.966 2.274-1.765C15.302 15.957 17 13.514 17 10c0-3.866-3.582-7-8-7S2 6.134 2 10c0 3.514 1.698 5.957 3.355 7.584a13.731 13.731 0 002.273 1.765 11.842 11.842 0 00.757.433 5.766 5.766 0 00.282.14l.018.008.006.003zM10 11.25a1.25 1.25 0 100-2.5 1.25 1.25 0 000 2.5z" clipRule="evenodd" />
          </svg>
          <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Standort</h3>
        </div>
        {availableTags.length > 0 && !showPicker && (
          <button
            onClick={() => setShowPicker(true)}
            className="text-xs text-accent-DEFAULT hover:text-accent-600 cursor-pointer"
          >
            + Standort zuweisen
          </button>
        )}
      </div>

      {/* Assigned location tags */}
      {assignedLocationTags.length > 0 && (
        <div className="flex flex-wrap gap-2 mb-3">
          {assignedLocationTags.map(tag => (
            <div
              key={tag.id}
              className="flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium"
              style={getTagBadgeStyle(tag.color)}
            >
              <svg className="w-3 h-3 opacity-70" viewBox="0 0 20 20" fill="currentColor">
                <path fillRule="evenodd" d="M9.69 18.933l.003.001C9.89 19.02 10 19 10 19s.11.02.308-.066l.002-.001.006-.003.018-.008a5.741 5.741 0 00.281-.14c.186-.096.446-.24.757-.433.62-.384 1.445-.966 2.274-1.765C15.302 15.957 17 13.514 17 10c0-3.866-3.582-7-8-7S2 6.134 2 10c0 3.514 1.698 5.957 3.355 7.584a13.731 13.731 0 002.273 1.765 11.842 11.842 0 00.757.433 5.766 5.766 0 00.282.14l.018.008.006.003zM10 11.25a1.25 1.25 0 100-2.5 1.25 1.25 0 000 2.5z" clipRule="evenodd" />
              </svg>
              {tag.name}
              {tag.lat != null && (
                <span className="opacity-60 text-xs">
                  {tag.lat.toFixed(3)}, {tag.lng?.toFixed(3)}
                </span>
              )}
              <button
                onClick={() => removeMutation.mutate(tag.id)}
                disabled={removeMutation.isPending}
                className="opacity-60 hover:opacity-100 cursor-pointer ml-0.5"
                title="Standort entfernen"
              >
                <svg className="w-3 h-3" viewBox="0 0 20 20" fill="currentColor">
                  <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
                </svg>
              </button>
            </div>
          ))}
        </div>
      )}

      {assignedLocationTags.length === 0 && !showPicker && (
        <p className="text-xs text-neutral-500 dark:text-neutral-400">
          Kein Standort zugeordnet.{' '}
          {availableTags.length > 0 && (
            <button
              onClick={() => setShowPicker(true)}
              className="text-accent-DEFAULT hover:underline cursor-pointer"
            >
              Jetzt zuweisen
            </button>
          )}
        </p>
      )}

      {/* Location picker dropdown */}
      {showPicker && (
        <div className="mt-2 space-y-2">
          <div className="text-xs text-neutral-500 dark:text-neutral-400">Standort-Tag zuweisen:</div>
          <div className="flex flex-wrap gap-1.5">
            {availableTags.map(tag => (
              <button
                key={tag.id}
                onClick={() => assignMutation.mutate(tag.id)}
                disabled={assignMutation.isPending}
                className="px-2.5 py-1 rounded-full text-xs font-medium cursor-pointer hover:opacity-90 disabled:opacity-50 transition-opacity"
                style={getTagBadgeStyle(tag.color)}
              >
                {tag.name}
              </button>
            ))}
          </div>
          <button
            onClick={() => setShowPicker(false)}
            className="text-xs text-neutral-400 hover:text-neutral-600 cursor-pointer"
          >
            Abbrechen
          </button>
        </div>
      )}
    </div>
  )
}
