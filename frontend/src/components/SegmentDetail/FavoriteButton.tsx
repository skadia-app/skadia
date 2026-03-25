import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Heart } from 'lucide-react'
import { apiClient } from '@/api/client'

interface FavoriteButtonProps {
  segmentId: string
}

export function FavoriteButton({ segmentId }: FavoriteButtonProps) {
  const queryClient = useQueryClient()

  const { data } = useQuery({
    queryKey: ['segment-favorite', segmentId],
    queryFn: () =>
      apiClient<{ favorite: boolean }>(`/segments/${segmentId}/favorite`),
  })

  const isFavorite = data?.favorite ?? false

  const mutation = useMutation({
    mutationFn: (fav: boolean) =>
      apiClient<unknown>(`/segments/${segmentId}/favorite`, {
        method: fav ? 'DELETE' : 'POST',
      }),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['segment-favorite', segmentId] }),
  })

  return (
    <button
      onClick={() => mutation.mutate(isFavorite)}
      disabled={mutation.isPending}
      className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed ${
        isFavorite
          ? 'border-accent-DEFAULT/50 text-accent-DEFAULT bg-accent-DEFAULT/10'
          : 'border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800'
      }`}
    >
      <Heart
        size={14}
        fill={isFavorite ? 'currentColor' : 'none'}
      />
      {isFavorite ? 'Favorit' : 'Favorit'}
    </button>
  )
}
