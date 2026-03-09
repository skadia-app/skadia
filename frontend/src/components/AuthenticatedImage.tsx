import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiDownload } from '@/api/client'

interface Props {
  src: string
  alt?: string
  className?: string
}

/**
 * Loads a protected image URL via the authenticated API client and displays it
 * via a blob URL. Necessary because <img src="..."> cannot send Authorization headers.
 * Results are cached by React Query (staleTime: 1h).
 */
export default function AuthenticatedImage({ src, alt, className }: Props) {
  const [objectUrl, setObjectUrl] = useState<string>('')

  const { data: blob } = useQuery({
    queryKey: ['auth-image', src],
    queryFn: async () => {
      const { blob } = await apiDownload(src)
      return blob
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
    />
  )
}
