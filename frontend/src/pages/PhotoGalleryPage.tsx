import { useState, useEffect } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import {
  listPhotoFeed, searchPhotos,
  type PhotoFeedGroup, type PhotoSearchResult, type PhotoSortMode, type PhotoFeedPage
} from '@/api/photos'
import { isVideoPhoto, type ActivityPhoto } from '@/api/activityPhotos'
import ActivityMiniCard from '@/components/ActivityMiniCard'
import AuthenticatedImage from '@/components/AuthenticatedImage'
import AuthenticatedMedia from '@/components/AuthenticatedMedia'
import { cn } from '@/lib/utils'
import { useDebounce } from '@/lib/useDebounce'

// ─── Types ────────────────────────────────────────────────────────────────────

interface LightboxState {
  photos: ActivityPhoto[]
  index: number
}

// ─── Main Page ────────────────────────────────────────────────────────────────

export default function PhotoGalleryPage() {
  const [sortMode, setSortMode] = useState<PhotoSortMode>('date')
  const [searchQuery, setSearchQuery] = useState('')
  const [lightbox, setLightbox] = useState<LightboxState | null>(null)

  const debouncedQuery = useDebounce(searchQuery, 300)
  const isSearching = debouncedQuery.length >= 2

  // Feed — infinite scroll
  const {
    data: feedData,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading: isFeedLoading,
  } = useInfiniteQuery({
    queryKey: ['photo-feed', sortMode],
    queryFn: ({ pageParam }) => listPhotoFeed(sortMode, pageParam as number),
    getNextPageParam: (lastPage: PhotoFeedPage) => lastPage.next_page ?? undefined,
    initialPageParam: 1,
    enabled: !isSearching,
  })

  // Search
  const { data: searchResults = [], isLoading: isSearchLoading } = useQuery({
    queryKey: ['photo-search', debouncedQuery],
    queryFn: () => searchPhotos(debouncedQuery),
    enabled: isSearching,
  })

  const allGroups = feedData?.pages.flatMap(p => p.groups) ?? []

  function openLightbox(photos: ActivityPhoto[], index: number) {
    setLightbox({ photos, index })
  }

  return (
    <div className="flex flex-col gap-6">
      {/* Header */}
      <div className="flex gap-3 items-center flex-wrap">
        <div className="relative flex-1 min-w-48 max-w-lg">
          <SearchIcon className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-neutral-400 pointer-events-none" />
          <input
            type="text"
            value={searchQuery}
            onChange={e => setSearchQuery(e.target.value)}
            placeholder="Fotos durchsuchen (Stichwort oder Bildinhalt)…"
            className="w-full pl-9 pr-8 py-2 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 placeholder:text-neutral-400 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
          />
          {searchQuery && (
            <button
              onClick={() => setSearchQuery('')}
              className="cursor-pointer absolute right-2.5 top-1/2 -translate-y-1/2 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 text-lg leading-none"
              aria-label="Suche löschen"
            >
              ×
            </button>
          )}
        </div>

        {!isSearching && (
          <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-sm shrink-0">
            {(['date', 'added'] as PhotoSortMode[]).map(mode => (
              <button
                key={mode}
                onClick={() => setSortMode(mode)}
                className={cn(
                  'cursor-pointer px-3 py-2 transition-colors',
                  sortMode === mode
                    ? 'bg-neutral-100 dark:bg-neutral-700 text-neutral-900 dark:text-neutral-100 font-medium'
                    : 'text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-300 bg-white dark:bg-neutral-900',
                )}
              >
                {mode === 'date' ? 'Datum' : 'Hinzugefügt'}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Content */}
      {isSearching ? (
        <SearchResultsGrid
          results={searchResults}
          isLoading={isSearchLoading}
          onOpen={openLightbox}
          query={debouncedQuery}
        />
      ) : (
        <FeedView
          groups={allGroups}
          sortMode={sortMode}
          isLoading={isFeedLoading}
          hasNextPage={!!hasNextPage}
          isFetchingNextPage={isFetchingNextPage}
          onFetchNextPage={fetchNextPage}
          onOpen={openLightbox}
        />
      )}

      {lightbox && (
        <GalleryLightbox
          photos={lightbox.photos}
          initialIndex={lightbox.index}
          onClose={() => setLightbox(null)}
        />
      )}
    </div>
  )
}

// ─── Feed View ────────────────────────────────────────────────────────────────

function FeedView({
  groups, sortMode, isLoading, hasNextPage, isFetchingNextPage, onFetchNextPage, onOpen,
}: {
  groups: PhotoFeedGroup[]
  sortMode: PhotoSortMode
  isLoading: boolean
  hasNextPage: boolean
  isFetchingNextPage: boolean
  onFetchNextPage: () => void
  onOpen: (photos: ActivityPhoto[], index: number) => void
}) {
  if (isLoading) return <FeedSkeleton />

  if (groups.length === 0) {
    return (
      <div className="text-center py-16 text-neutral-400 dark:text-neutral-500">
        <CameraIcon className="w-12 h-12 mx-auto mb-3 opacity-40" />
        <p className="font-medium">Noch keine Fotos vorhanden</p>
        <p className="text-sm mt-1">Fotos können beim Bearbeiten einer Aktivität hochgeladen werden.</p>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-6">
      {groups.map(group => (
        <PhotoFeedGroupView
          key={group.group_key}
          group={group}
          sortMode={sortMode}
          onOpen={onOpen}
        />
      ))}

      {hasNextPage && (
        <div className="flex justify-center py-2">
          <button
            onClick={onFetchNextPage}
            disabled={isFetchingNextPage}
            className="px-4 py-2 text-sm rounded-lg bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 text-neutral-700 dark:text-neutral-300 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
          >
            {isFetchingNextPage ? 'Lädt…' : 'Mehr laden'}
          </button>
        </div>
      )}
    </div>
  )
}

// ─── Group View ───────────────────────────────────────────────────────────────

function PhotoFeedGroupView({
  group, sortMode, onOpen,
}: {
  group: PhotoFeedGroup
  sortMode: PhotoSortMode
  onOpen: (photos: ActivityPhoto[], index: number) => void
}) {
  const isActivityGroup = sortMode === 'date' && group.group_type === 'activity' && group.activity

  if (!isActivityGroup) {
    // Day-grouped view (sort=added or photos without activity)
    return (
      <div className="flex flex-col gap-3">
        <DaySeparator label={formatDayLabel(group.group_key)} />
        <PhotoGrid photos={group.photos} onOpen={onOpen} />
      </div>
    )
  }

  // Activity-grouped view (sort=date): 1/3 card + 2/3 photos
  return (
    <div>
      <div className="h-px bg-neutral-100 dark:bg-neutral-800 mb-4" />
      <div className="flex gap-4 items-start">
        {/* Left: Activity card (1/3), sticky while scrolling through photos */}
        <div className="w-1/3 shrink-0 sticky top-20 self-start">
          <ActivityMiniCard
            activity={group.activity!}
            photoCount={group.photos.length}
          />
        </div>

        {/* Right: Photos (2/3) */}
        <div className="flex-1 min-w-0">
          <PhotoGrid photos={group.photos} onOpen={onOpen} />
        </div>
      </div>
    </div>
  )
}

// ─── Photo Grid ───────────────────────────────────────────────────────────────
// Row 1: up to 2 photos side-by-side; rows 2+: full width

function PhotoGrid({
  photos, onOpen,
}: {
  photos: ActivityPhoto[]
  onOpen: (photos: ActivityPhoto[], index: number) => void
}) {
  if (photos.length === 0) return null

  return (
    <div className="flex flex-col gap-2">
      {/* First row: 2 columns */}
      {photos.length === 1 ? (
        <PhotoThumb photo={photos[0]} onClick={() => onOpen(photos, 0)} />
      ) : (
        <div className="grid grid-cols-2 gap-2">
          {photos.slice(0, 2).map((photo, idx) => (
            <PhotoThumb key={photo.id} photo={photo} onClick={() => onOpen(photos, idx)} />
          ))}
        </div>
      )}

      {/* Remaining rows: full width */}
      {photos.slice(2).map((photo, idx) => (
        <PhotoThumb
          key={photo.id}
          photo={photo}
          onClick={() => onOpen(photos, idx + 2)}
          fullWidth
        />
      ))}
    </div>
  )
}

// ─── Photo Thumbnail ──────────────────────────────────────────────────────────

function PhotoThumb({
  photo, onClick, fullWidth = false,
}: {
  photo: ActivityPhoto
  onClick: () => void
  fullWidth?: boolean
}) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'relative block overflow-hidden rounded-lg bg-neutral-100 dark:bg-neutral-800 group cursor-pointer w-full',
        fullWidth ? 'aspect-[21/9]' : 'aspect-[4/3]',
      )}
    >
      <AuthenticatedImage
        src={photo.preview_url}
        alt={photo.original_filename}
        className="w-full h-full object-cover group-hover:scale-[1.02] transition-transform duration-200"
      />
      {isVideoPhoto(photo) && (
        <>
          <div className="absolute inset-0 flex items-center justify-center">
            <div className="bg-black/50 rounded-full p-2.5">
              <PlayIcon className="w-5 h-5 text-white" />
            </div>
          </div>
          {photo.duration_s != null && (
            <span className="absolute bottom-2 right-2 text-xs text-white bg-black/60 rounded px-1.5 py-0.5">
              {formatVideoDuration(photo.duration_s)}
            </span>
          )}
        </>
      )}
    </button>
  )
}

// ─── Search Results ───────────────────────────────────────────────────────────

function SearchResultsGrid({
  results, isLoading, onOpen, query,
}: {
  results: PhotoSearchResult[]
  isLoading: boolean
  onOpen: (photos: ActivityPhoto[], index: number) => void
  query: string
}) {
  if (isLoading) return <FeedSkeleton />

  if (results.length === 0) {
    return (
      <div className="text-center py-12 text-neutral-400 dark:text-neutral-500">
        <p className="font-medium">Keine Fotos gefunden für „{query}"</p>
        <p className="text-xs mt-1 max-w-xs mx-auto">
          Tipp: Beschreibe den Bildinhalt auf Deutsch oder Englisch (z.B. „Bergpanorama", „Gruppe am Gipfel").
          Suche funktioniert auch als Titelsuche.
        </p>
      </div>
    )
  }

  const photos = results.map(r => r.photo)

  return (
    <div className="flex flex-col gap-4">
      <p className="text-xs text-neutral-400 dark:text-neutral-500">
        {results.length} Ergebnis{results.length !== 1 ? 'se' : ''} für „{query}"
      </p>
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-2">
        {results.map((result, idx) => (
          <div key={result.photo.id} className="flex flex-col gap-1">
            <PhotoThumb photo={result.photo} onClick={() => onOpen(photos, idx)} />
            {result.activity && (
              <p className="text-xs text-neutral-500 dark:text-neutral-400 truncate px-0.5">
                {result.activity.title}
              </p>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

// ─── Lightbox ─────────────────────────────────────────────────────────────────

function GalleryLightbox({
  photos, initialIndex, onClose,
}: {
  photos: ActivityPhoto[]
  initialIndex: number
  onClose: () => void
}) {
  const [index, setIndex] = useState(initialIndex)
  const photo = photos[index]

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
      if (e.key === 'ArrowLeft') setIndex(i => Math.max(0, i - 1))
      if (e.key === 'ArrowRight') setIndex(i => Math.min(photos.length - 1, i + 1))
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [photos.length, onClose])

  return (
    <div
      className="fixed inset-0 z-50 bg-black/90 flex flex-col items-center justify-center"
      onClick={onClose}
    >
      <div
        className="relative flex-1 w-full flex items-center justify-center px-14 min-h-0"
        onClick={e => e.stopPropagation()}
      >
        {isVideoPhoto(photo) ? (
          <AuthenticatedMedia
            photo={photo}
            objectFit="contain"
            className="max-w-full max-h-full rounded-lg shadow-2xl"
          />
        ) : (
          <AuthenticatedImage
            src={photo.preview_url || photo.full_url}
            alt={photo.original_filename}
            className="max-w-full max-h-full object-contain rounded-lg shadow-2xl"
          />
        )}

        {index > 0 && (
          <button
            onClick={() => setIndex(i => i - 1)}
            className="cursor-pointer absolute left-2 top-1/2 -translate-y-1/2 bg-black/50 hover:bg-black/70 text-white rounded-full p-2 transition-colors"
            aria-label="Vorheriges Foto"
          >
            <ChevronLeftIcon className="w-5 h-5" />
          </button>
        )}
        {index < photos.length - 1 && (
          <button
            onClick={() => setIndex(i => i + 1)}
            className="cursor-pointer absolute right-2 top-1/2 -translate-y-1/2 bg-black/50 hover:bg-black/70 text-white rounded-full p-2 transition-colors"
            aria-label="Nächstes Foto"
          >
            <ChevronRightIcon className="w-5 h-5" />
          </button>
        )}

        <button
          onClick={onClose}
          className="absolute top-2 right-2 bg-black/50 hover:bg-black/70 text-white rounded-full p-1.5 cursor-pointer transition-colors"
          aria-label="Schließen"
        >
          <XIcon className="w-4 h-4" />
        </button>

        <div className="absolute bottom-2 left-1/2 -translate-x-1/2 text-white/60 text-xs bg-black/40 rounded-full px-2 py-0.5 select-none">
          {index + 1} / {photos.length}
        </div>
      </div>
    </div>
  )
}

// ─── Skeleton ─────────────────────────────────────────────────────────────────

function FeedSkeleton() {
  return (
    <div className="flex flex-col gap-6">
      {[0, 1].map(i => (
        <div key={i} className="flex gap-4">
          <div className="w-1/3 shrink-0 flex flex-col gap-2">
            <div className="rounded-lg bg-neutral-100 dark:bg-neutral-800 aspect-[5/3] animate-pulse" />
            <div className="h-4 bg-neutral-100 dark:bg-neutral-800 rounded animate-pulse w-2/3" />
            <div className="h-3 bg-neutral-100 dark:bg-neutral-800 rounded animate-pulse w-1/2" />
          </div>
          <div className="flex-1 grid grid-cols-2 gap-2">
            <div className="aspect-[4/3] rounded-lg bg-neutral-100 dark:bg-neutral-800 animate-pulse" />
            <div className="aspect-[4/3] rounded-lg bg-neutral-100 dark:bg-neutral-800 animate-pulse" />
          </div>
        </div>
      ))}
    </div>
  )
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDayLabel(dateStr: string): string {
  const d = new Date(dateStr)
  const today = new Date()
  const yesterday = new Date(today)
  yesterday.setDate(today.getDate() - 1)
  if (d.toDateString() === today.toDateString()) return 'Heute'
  if (d.toDateString() === yesterday.toDateString()) return 'Gestern'
  return d.toLocaleDateString('de-DE', { day: 'numeric', month: 'long', year: 'numeric' })
}

function formatVideoDuration(s: number): string {
  const m = Math.floor(s / 60)
  const sec = Math.floor(s % 60)
  return `${m}:${sec.toString().padStart(2, '0')}`
}

function DaySeparator({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2 text-xs text-neutral-400 dark:text-neutral-500">
      <span className="font-medium">{label}</span>
      <div className="flex-1 h-px bg-neutral-200 dark:bg-neutral-800" />
    </div>
  )
}

// ─── Icons ────────────────────────────────────────────────────────────────────

function SearchIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
      <circle cx="11" cy="11" r="8" /><path d="m21 21-4.35-4.35" />
    </svg>
  )
}
function PlayIcon({ className }: { className?: string }) {
  return <svg className={className} viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z" /></svg>
}
function CameraIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
      <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z" />
      <circle cx="12" cy="13" r="4" />
    </svg>
  )
}
function ChevronLeftIcon({ className }: { className?: string }) {
  return <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="m15 18-6-6 6-6" /></svg>
}
function ChevronRightIcon({ className }: { className?: string }) {
  return <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="m9 18 6-6-6-6" /></svg>
}
function XIcon({ className }: { className?: string }) {
  return <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M18 6 6 18M6 6l12 12" /></svg>
}
