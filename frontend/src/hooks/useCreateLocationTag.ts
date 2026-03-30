import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createTag, assignActivitiesToTag, syncTagLocation } from '../api/tags'

interface CreateLocationTagParams {
  name: string
  lat: number
  lng: number
  color?: string
  activityIds: string[]
}

export function useCreateLocationTag() {
  const qc = useQueryClient()

  return useMutation({
    mutationFn: async ({ name, lat, lng, color, activityIds }: CreateLocationTagParams) => {
      // 1. Create the location tag (backend auto-sets category='standort' when lat/lng present)
      const tag = await createTag({
        name,
        color: color ?? '#EF4444',
        text_color: '#FFFFFF',
        lat,
        lng,
      })

      // 2. Assign activities to it
      if (activityIds.length > 0) {
        await assignActivitiesToTag(tag.id, activityIds)
      }

      // 3. Sync coordinates to activities
      await syncTagLocation(tag.id)

      return tag
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['tags'] })
      qc.invalidateQueries({ queryKey: ['activities'] })
      qc.invalidateQueries({ queryKey: ['unlocated-activities'] })
    },
  })
}
