import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiDownload } from '@/api/client'
import { isVideoPhoto, type ActivityPhoto } from '@/api/activityPhotos'

interface Props {
  photo: ActivityPhoto
  className?: string
  objectFit?: 'cover' | 'contain'
}

/**
 * Renders either an <img> or a <video> element depending on the photo's content type.
 *
 * For images: behaves identically to AuthenticatedImage — fetches via authenticated
 * API client and renders via blob URL.
 *
 * For videos: fetches the video via authenticated API client and renders a <video>
 * element with controls, playsInline, and preload="metadata". The thumb_url is used
 * as the poster image (shown before playback starts).
 *
 * Note: blob URLs for large video files are held in memory only while the component
 * is mounted and are revoked on unmount.
 */
export default function AuthenticatedMedia({ photo, className, objectFit = 'contain' }: Props) {
  const isVideo = isVideoPhoto(photo)

  // ── Poster (thumbnail) blob URL for video ───────────────────────────────────
  const [posterUrl, setPosterUrl] = useState<string>('')
  const { data: posterBlob } = useQuery({
    queryKey: ['auth-image', photo.thumb_url],
    queryFn: async () => {
      const { blob } = await apiDownload(photo.thumb_url)
      return blob
    },
    staleTime: 60 * 60 * 1000,
    enabled: isVideo && !!photo.thumb_url,
  })

  useEffect(() => {
    if (!posterBlob) return
    const url = URL.createObjectURL(posterBlob)
    setPosterUrl(url)
    return () => URL.revokeObjectURL(url)
  }, [posterBlob])

  // ── Main media blob URL (image or video) ────────────────────────────────────
  const mediaSrc = isVideo ? photo.full_url : (photo.preview_url || photo.full_url)

  const [objectUrl, setObjectUrl] = useState<string>('')
  const { data: mediaBlob } = useQuery({
    queryKey: ['auth-image', mediaSrc],
    queryFn: async () => {
      const { blob } = await apiDownload(mediaSrc)
      return blob
    },
    staleTime: 60 * 60 * 1000,
    enabled: !!mediaSrc,
  })

  useEffect(() => {
    if (!mediaBlob) return
    const url = URL.createObjectURL(mediaBlob)
    setObjectUrl(url)
    return () => URL.revokeObjectURL(url)
  }, [mediaBlob])

  if (isVideo) {
    // Set width/height HTML attributes from the stored video dimensions so the
    // element has its correct intrinsic size before the src blob URL loads.
    // Without these, the browser uses the poster thumbnail's small size (~200px)
    // as the intrinsic dimensions, causing the element to render tiny in the
    // lightbox. max-w-full / max-h-full CSS then scales the 1920×1080 intrinsic
    // size down to fit the container correctly.
    const intrinsicW = photo.video_width ?? 1920
    const intrinsicH = photo.video_height ?? 1080

    return (
      <video
        src={objectUrl || undefined}
        poster={posterUrl || undefined}
        width={intrinsicW}
        height={intrinsicH}
        controls
        playsInline
        preload="metadata"
        className={className}
        style={objectFit ? { objectFit } : undefined}
      />
    )
  }

  return (
    <img
      src={objectUrl || undefined}
      alt={photo.original_filename}
      className={className}
      style={objectFit ? { objectFit } : undefined}
    />
  )
}
