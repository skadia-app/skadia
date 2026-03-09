import { useState, useRef, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listActivityPhotos, uploadActivityPhoto, deleteActivityPhoto, type ActivityPhoto } from '@/api/activityPhotos'
import { fetchActivityStreams } from '@/api/activities'
import { useModal } from '@/lib/modal'
import { useToast } from '@/lib/toast'
import AuthenticatedImage from '@/components/AuthenticatedImage'
import PhotoGeoAssignmentOverlay from '@/components/PhotoGeoAssignmentOverlay'
import type { StreamPoint } from '@/api/types'

interface Props {
  activityId: string
}

export default function ActivityPhotoSection({ activityId }: Props) {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [collapsed, setCollapsed] = useState<boolean | null>(null)
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null)
  const [showGeoAssign, setShowGeoAssign] = useState(false)
  const { openModal } = useModal()

  const { data: photos = [], isLoading } = useQuery({
    queryKey: ['activity-photos', activityId],
    queryFn: () => listActivityPhotos(activityId),
  })

  const selectedPhoto = selectedIndex !== null ? (photos[selectedIndex] ?? null) : null

  function goToPrev() {
    if (selectedIndex === null || photos.length <= 1) return
    setSelectedIndex((selectedIndex - 1 + photos.length) % photos.length)
  }

  function goToNext() {
    if (selectedIndex === null || photos.length <= 1) return
    setSelectedIndex((selectedIndex + 1) % photos.length)
  }

  useEffect(() => {
    if (selectedIndex === null) return
    function handleKey(e: KeyboardEvent) {
      if (e.key === 'ArrowLeft') goToPrev()
      else if (e.key === 'ArrowRight') goToNext()
      else if (e.key === 'Escape') setSelectedIndex(null)
    }
    window.addEventListener('keydown', handleKey)
    return () => window.removeEventListener('keydown', handleKey)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedIndex, photos.length])

  // Streams are fetched lazily when the geo-assign overlay is opened
  const { data: streams } = useQuery<StreamPoint[]>({
    queryKey: ['activity-streams', activityId],
    queryFn: () => fetchActivityStreams(activityId),
    enabled: showGeoAssign,
  })

  const photosWithoutPosition = photos.filter(p => p.lat == null || p.lng == null)

  const uploadMutation = useMutation({
    mutationFn: (file: File) => uploadActivityPhoto(activityId, file),
    onSuccess: (result) => {
      if (result.status === 'duplicate') {
        addToast({ type: 'warning', title: 'Foto bereits vorhanden', message: 'Dieses Foto wurde bereits hochgeladen.' })
      } else {
        queryClient.invalidateQueries({ queryKey: ['activity-photos', activityId] })
      }
    },
    onError: () => {
      addToast({ type: 'error', title: 'Upload fehlgeschlagen', duration: 0 })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (photoId: string) => deleteActivityPhoto(activityId, photoId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activity-photos', activityId] })
    },
  })

  function handleFiles(files: FileList | null) {
    if (!files) return
    for (let i = 0; i < files.length; i++) {
      uploadMutation.mutate(files[i])
    }
  }

  function handleDeletePhoto(photo: ActivityPhoto) {
    openModal({
      title: `Foto "${photo.original_filename}" löschen?`,
      message: 'Löschen ist nicht rückgängig zu machen.',
      buttons: [
        {
          label: 'Abbrechen',
          variant: 'default',
          onClick: () => {}
        },
        {
          label: 'Löschen',
          variant: 'destructive',
          onClick: async () => {
            deleteMutation.mutate(photo.id)
            if (selectedPhoto?.id === photo.id) setSelectedIndex(null)
          }
        }
      ]
    })
  }

  // Default collapsed state: collapsed if no photos, expanded if photos
  const isCollapsed = collapsed ?? photos.length === 0

  if (isLoading) return null

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <button
        onClick={() => setCollapsed(!isCollapsed)}
        className="w-full flex items-center justify-between px-4 py-3 text-sm font-medium text-neutral-900 dark:text-neutral-100 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 transition-colors cursor-pointer"
      >
        <span>Fotos {photos.length > 0 && <span className="text-neutral-400">({photos.length})</span>}</span>
        <svg
          className={`w-4 h-4 text-neutral-400 transition-transform ${isCollapsed ? '-rotate-90' : ''}`}
          viewBox="0 0 20 20"
          fill="currentColor"
        >
          <path fillRule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clipRule="evenodd" />
        </svg>
      </button>

      {!isCollapsed && (
        <div className="px-4 pb-4 space-y-3 border-t border-neutral-100 dark:border-neutral-800">
          {/* Thumbnail grid */}
          {photos.length > 0 && (
            <div className="grid grid-cols-4 sm:grid-cols-6 md:grid-cols-8 gap-2 pt-3">
              {photos.map((photo, index) => (
                <div key={photo.id} className="relative group aspect-square">
                  <button
                    onClick={() => setSelectedIndex(index)}
                    className="w-full h-full rounded-lg overflow-hidden cursor-pointer border border-neutral-200 dark:border-neutral-700 hover:border-accent-400 transition-colors"
                  >
                    <AuthenticatedImage
                      src={photo.thumb_url}
                      alt={photo.original_filename}
                      className="w-full h-full object-cover"
                    />
                  </button>
                  <button
                    onClick={() => handleDeletePhoto(photo)}
                    className="absolute -top-1 -right-1 w-5 h-5 rounded-full bg-red-500 text-white flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer shadow-sm"
                    title="Foto löschen"
                  >
                    <svg className="w-3 h-3" viewBox="0 0 20 20" fill="currentColor">
                      <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
                    </svg>
                  </button>
                </div>
              ))}
            </div>
          )}

          {/* No-position warning + geo-assign trigger */}
          {photosWithoutPosition.length > 0 && (
            <div className="flex items-center gap-2 px-1">
              <svg className="w-3.5 h-3.5 text-amber-500 shrink-0" viewBox="0 0 20 20" fill="currentColor">
                <path fillRule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clipRule="evenodd" />
              </svg>
              <button
                onClick={() => setShowGeoAssign(true)}
                className="text-xs text-amber-500 hover:text-amber-400 cursor-pointer transition-colors text-left"
              >
                {photosWithoutPosition.length} {photosWithoutPosition.length === 1 ? 'Foto' : 'Fotos'} ohne Position — Auf Karte zuweisen
              </button>
            </div>
          )}

          {/* Upload button */}
          <div className="pt-1">
            <input
              ref={fileInputRef}
              type="file"
              accept="image/*"
              multiple
              className="hidden"
              onChange={(e) => handleFiles(e.target.files)}
            />
            <button
              onClick={() => fileInputRef.current?.click()}
              disabled={uploadMutation.isPending}
              className="text-xs text-accent-DEFAULT hover:text-accent-600 cursor-pointer disabled:opacity-50"
            >
              {uploadMutation.isPending ? 'Wird hochgeladen...' : '+ Fotos hochladen'}
            </button>
          </div>
        </div>
      )}

      {/* Geo-assignment overlay */}
      {showGeoAssign && (
        !streams ? (
          <div className="fixed inset-0 z-[200] flex items-center justify-center bg-neutral-950/95">
            <div className="flex flex-col items-center gap-3 text-neutral-400">
              <div className="w-6 h-6 border-2 border-neutral-500 border-t-accent-DEFAULT rounded-full animate-spin" />
              <span className="text-sm">Track wird geladen…</span>
            </div>
          </div>
        ) : (() => {
          const trackCoords: [number, number][] = (streams ?? [])
            .filter((s): s is StreamPoint & { lat: number; lng: number } => s.lat != null && s.lng != null)
            .map(s => [s.lng, s.lat])
          const trackTimes: string[] = (streams ?? [])
            .filter((s): s is StreamPoint & { lat: number; lng: number } => s.lat != null && s.lng != null)
            .map(s => s.time)
          return (
            <PhotoGeoAssignmentOverlay
              activityId={activityId}
              trackCoords={trackCoords}
              trackTimes={trackTimes}
              unpositionedPhotos={photosWithoutPosition}
              onClose={() => setShowGeoAssign(false)}
            />
          )
        })()
      )}

      {/* Lightbox */}
      {selectedPhoto && selectedIndex !== null && (
        <div
          className="fixed inset-0 z-50 flex flex-col items-center justify-center bg-black/90"
          onClick={() => setSelectedIndex(null)}
        >
          {/* Top bar */}
          <div className="w-full flex items-center justify-between px-4 py-2 shrink-0" onClick={(e) => e.stopPropagation()}>
            <div className="text-sm text-neutral-300 truncate max-w-xs">
              {selectedPhoto.original_filename}
              {selectedPhoto.taken_at && (
                <span className="ml-2 text-neutral-500">
                  {new Date(selectedPhoto.taken_at).toLocaleString('de-DE')}
                </span>
              )}
            </div>
            <div className="flex items-center gap-3">
              {photos.length > 1 && (
                <span className="text-sm text-neutral-400 select-none">
                  {selectedIndex + 1} / {photos.length}
                </span>
              )}
              <button
                onClick={() => setSelectedIndex(null)}
                className="w-8 h-8 flex items-center justify-center rounded-full bg-white/10 hover:bg-white/20 text-white transition-colors cursor-pointer"
                aria-label="Schließen"
              >
                <svg className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
                  <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
                </svg>
              </button>
            </div>
          </div>

          {/* Image area */}
          <div
            className="relative flex-1 w-full flex items-center justify-center px-14 min-h-0"
            onClick={(e) => e.stopPropagation()}
          >
            <AuthenticatedImage
              src={selectedPhoto.preview_url || selectedPhoto.full_url}
              alt={selectedPhoto.original_filename}
              className="max-w-full object-contain rounded-lg shadow-2xl"
            />

            {photos.length > 1 && (
              <>
                <button
                  onClick={(e) => { e.stopPropagation(); goToPrev() }}
                  className="absolute left-2 top-1/2 -translate-y-1/2 w-10 h-10 flex items-center justify-center rounded-full bg-black/50 hover:bg-black/70 text-white transition-colors cursor-pointer"
                  aria-label="Vorheriges Foto"
                >
                  <svg className="w-6 h-6" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M12.79 5.23a.75.75 0 01-.02 1.06L8.832 10l3.938 3.71a.75.75 0 11-1.04 1.08l-4.5-4.25a.75.75 0 010-1.08l4.5-4.25a.75.75 0 011.06.02z" clipRule="evenodd" />
                  </svg>
                </button>
                <button
                  onClick={(e) => { e.stopPropagation(); goToNext() }}
                  className="absolute right-2 top-1/2 -translate-y-1/2 w-10 h-10 flex items-center justify-center rounded-full bg-black/50 hover:bg-black/70 text-white transition-colors cursor-pointer"
                  aria-label="Nächstes Foto"
                >
                  <svg className="w-6 h-6" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M7.21 14.77a.75.75 0 01.02-1.06L11.168 10 7.23 6.29a.75.75 0 111.04-1.08l4.5 4.25a.75.75 0 010 1.08l-4.5 4.25a.75.75 0 01-1.06-.02z" clipRule="evenodd" />
                  </svg>
                </button>
              </>
            )}
          </div>

          {/* Bottom bar */}
          {(selectedPhoto.lat != null && selectedPhoto.lng != null) && (
            <div className="shrink-0 py-2 text-xs text-neutral-500" onClick={(e) => e.stopPropagation()}>
              {selectedPhoto.lat.toFixed(5)}, {selectedPhoto.lng.toFixed(5)}
              {selectedPhoto.location_source && (
                <span className="ml-1">({selectedPhoto.location_source === 'exif' ? 'EXIF' : 'Track'})</span>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
