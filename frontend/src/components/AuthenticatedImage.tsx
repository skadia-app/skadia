import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiDownload } from '@/api/client'

interface Props {
  src: string
  fallbackSrc?: string
  alt?: string
  className?: string
}

/**
 * Loads a protected image URL via the authenticated API client and displays it
 * via a blob URL. Necessary because <img src="..."> cannot send Authorization headers.
 * Results are cached by React Query (staleTime: 1h).
 */
export default function AuthenticatedImage({ src, fallbackSrc, alt, className }: Props) {
  const [objectUrl, setObjectUrl] = useState<string>('')

  const { data: blob } = useQuery({
    queryKey: ['auth-image', src, fallbackSrc],
    queryFn: async () => {
      try {
        const { blob } = await apiDownload(src)
        return blob
      } catch (err) {
        if (fallbackSrc && fallbackSrc !== src) {
          const { blob } = await apiDownload(fallbackSrc)
          return blob
        }
        throw err
      }
    },
    staleTime: 60 * 60 * 1000, // 1 hour — photo content doesn't change
    enabled: !!src,
  })

  useEffect(() => {
    if (!blob) return
    const url = URL.createObjectURL(blob)
    setObjectUrl(url)
    return () => URL.revokeObjectURL(url)
  }, [blob])

  return (
    <img
      src={objectUrl || undefined}
      alt={alt}
      className={className}
      draggable={false}
    />
  )
}
