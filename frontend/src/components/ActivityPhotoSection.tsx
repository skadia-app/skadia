import { useState, useRef } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listActivityPhotos, uploadActivityPhoto, deleteActivityPhoto, type ActivityPhoto } from '@/api/activityPhotos'
import { useModal } from '@/lib/modal'

interface Props {
  activityId: string
}

export default function ActivityPhotoSection({ activityId }: Props) {
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [collapsed, setCollapsed] = useState<boolean | null>(null)
  const [selectedPhoto, setSelectedPhoto] = useState<ActivityPhoto | null>(null)
  const { openModal } = useModal()

  const { data: photos = [], isLoading } = useQuery({
    queryKey: ['activity-photos', activityId],
    queryFn: () => listActivityPhotos(activityId),
  })

  const uploadMutation = useMutation({
    mutationFn: (file: File) => uploadActivityPhoto(activityId, file),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activity-photos', activityId] })
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
            if (selectedPhoto?.id === photo.id) setSelectedPhoto(null)
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
              {photos.map((photo) => (
                <div key={photo.id} className="relative group aspect-square">
                  <button
                    onClick={() => setSelectedPhoto(photo)}
                    className="w-full h-full rounded-lg overflow-hidden cursor-pointer border border-neutral-200 dark:border-neutral-700 hover:border-accent-400 transition-colors"
                  >
                    <img
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

      {/* Lightbox */}
      {selectedPhoto && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/70"
          onClick={() => setSelectedPhoto(null)}
        >
          <div
            className="bg-white dark:bg-neutral-900 rounded-xl shadow-2xl max-w-lg w-full mx-4 overflow-hidden"
            onClick={(e) => e.stopPropagation()}
          >
            <img
              src={selectedPhoto.full_url}
              alt={selectedPhoto.original_filename}
              className="w-full object-contain max-h-96"
            />
            <div className="p-4">
              <p className="text-sm text-neutral-700 dark:text-neutral-300">{selectedPhoto.original_filename}</p>
              {selectedPhoto.taken_at && (
                <p className="text-xs text-neutral-500 mt-1">
                  {new Date(selectedPhoto.taken_at).toLocaleString('de-DE')}
                </p>
              )}
              {selectedPhoto.lat != null && selectedPhoto.lng != null && (
                <p className="text-xs text-neutral-400 mt-0.5">
                  {selectedPhoto.lat.toFixed(5)}, {selectedPhoto.lng.toFixed(5)}
                  {selectedPhoto.location_source && (
                    <span className="ml-1">({selectedPhoto.location_source === 'exif' ? 'EXIF' : 'Track'})</span>
                  )}
                </p>
              )}
              <button
                onClick={() => setSelectedPhoto(null)}
                className="mt-3 w-full text-xs text-neutral-400 hover:text-neutral-600 cursor-pointer"
              >
                Schliessen
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
