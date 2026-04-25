import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '@/api/client'
import { labels } from '@/lib/trackSummaryLabels'

type SurfaceExtensionBannerProps = {
  show: boolean
}

export function SurfaceExtensionBanner({ show }: SurfaceExtensionBannerProps) {
  const queryClient = useQueryClient()

  const acknowledgeMutation = useMutation({
    mutationFn: () => apiClient<void>('/users/me/acknowledge/surface-extension', { method: 'POST' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['me'] })
    },
  })

  if (!show) return null

  return (
    <div
      role="status"
      className="rounded-lg border border-amber-200 dark:border-amber-800/60 bg-amber-50 dark:bg-amber-900/20 px-4 py-3 text-sm text-amber-800 dark:text-amber-200"
    >
      <p>{labels.banner.surfaceExtensionBody}</p>
      <button
        type="button"
        onClick={() => acknowledgeMutation.mutate()}
        disabled={acknowledgeMutation.isPending}
        className="mt-3 cursor-pointer font-medium underline underline-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {labels.banner.surfaceExtensionAck}
      </button>
    </div>
  )
}
