import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Heart } from 'lucide-react'

interface FavoriteButtonProps {
  segmentId: number
}

export function FavoriteButton({ segmentId }: FavoriteButtonProps) {
  const queryClient = useQueryClient()

  const { data } = useQuery({
    queryKey: ['segment-favorite', segmentId],
    queryFn: () =>
      fetch(`/api/v1/segments/${segmentId}/favorite`).then(r => r.json()),
  })

  const isFavorite = data?.favorite ?? false

  const mutation = useMutation({
    mutationFn: (fav: boolean) =>
      fetch(`/api/v1/segments/${segmentId}/favorite`, {
        method: fav ? 'DELETE' : 'POST',
      }).then(r => r.json()),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['segment-favorite', segmentId] }),
  })

  return (
    <button
      onClick={() => mutation.mutate(isFavorite)}
      disabled={mutation.isPending}
      className="flex items-center gap-2 px-4 py-2 rounded-lg border transition-all disabled:opacity-50"
      style={{
        borderColor: isFavorite ? 'var(--color-accent-DEFAULT, #FF6B35)' : undefined,
        color: isFavorite ? 'var(--color-accent-DEFAULT, #FF6B35)' : undefined,
        backgroundColor: isFavorite
          ? 'color-mix(in srgb, var(--color-accent-DEFAULT, #FF6B35) 10%, transparent)'
          : undefined,
      }}
    >
      <Heart
        size={18}
        fill={isFavorite ? 'var(--color-accent-DEFAULT, #FF6B35)' : 'none'}
        stroke={isFavorite ? 'var(--color-accent-DEFAULT, #FF6B35)' : 'currentColor'}
      />
      {isFavorite ? 'Favorit' : 'Zu Favoriten'}
    </button>
  )
}
