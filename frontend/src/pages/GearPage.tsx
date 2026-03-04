import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { fetchGear, createGear, fetchGearDocuments, getGearDocumentUrl } from '@/api/gear'
import type { Gear, GearStatus } from '@/api/types'
import { formatDistance, formatDuration, formatElevationGain } from '@/lib/utils'
import GearForm from '@/components/GearForm'
import MaintenanceAlerts from '@/components/MaintenanceAlerts'
import GearMergePanel from '@/components/GearMergePanel'

const STATUS_CHIPS: { value: GearStatus | undefined; label: string }[] = [
  { value: undefined, label: 'Alle' },
  { value: 'active', label: 'Aktiv' },
  { value: 'stored', label: 'Gelagert' },
  { value: 'retired', label: 'Ausgemustert' },
]

export default function GearPage() {
  const queryClient = useQueryClient()
  const [statusFilter, setStatusFilter] = useState<GearStatus | undefined>('active')
  const [showForm, setShowForm] = useState(false)
  const [showMerge, setShowMerge] = useState(false)

  const { data: gear, isLoading } = useQuery({
    queryKey: ['gear', statusFilter],
    queryFn: () => fetchGear(statusFilter),
  })

  const createMutation = useMutation({
    mutationFn: (data: Partial<Gear>) => createGear(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gear'] })
      setShowForm(false)
    },
  })

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Ausrüstung...</div>
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Ausrüstung</h1>
        <div className="flex items-center gap-3">
          {/* Status filter chips */}
          <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700">
            {STATUS_CHIPS.map(({ value, label }) => (
              <button
                key={label}
                onClick={() => setStatusFilter(value)}
                className={`px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer ${
                  statusFilter === value
                    ? 'bg-orange-500 text-white'
                    : 'text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                }`}
              >
                {label}
              </button>
            ))}
          </div>
          <button
            onClick={() => setShowMerge(s => !s)}
            className="text-sm px-3 py-2 rounded-lg border border-neutral-200 dark:border-neutral-800 text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100 transition-colors cursor-pointer"
          >
            Duplikate zusammenführen
          </button>
          <button
            onClick={() => setShowForm(true)}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 transition-colors cursor-pointer"
          >
            Ausrüstung hinzufügen
          </button>
        </div>
      </div>

      <MaintenanceAlerts />

      {/* Gear merge panel */}
      {showMerge && (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-base font-medium text-neutral-900 dark:text-neutral-100">Gear-Duplikate</h2>
          </div>
          <GearMergePanel onClose={() => setShowMerge(false)} />
        </div>
      )}

      {/* Create form */}
      {showForm && (
        <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100 mb-4">Neue Ausrüstung</h2>
          <GearForm
            onSave={(data) => createMutation.mutate(data)}
            onCancel={() => setShowForm(false)}
            isSaving={createMutation.isPending}
          />
          {createMutation.isError && (
            <p className="text-sm text-rose-500 mt-2">
              Fehler beim Erstellen: {createMutation.error instanceof Error ? createMutation.error.message : 'Unbekannt'}
            </p>
          )}
        </div>
      )}

      {/* Gear grid */}
      {gear && gear.length > 0 ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {gear.map((item) => (
            <GearCard key={item.id} gear={item} />
          ))}
        </div>
      ) : (
        <div className="text-center py-16 text-neutral-400">
          Noch keine Ausrüstung vorhanden.
        </div>
      )}
    </div>
  )
}

function GearCardImage({ gearId }: { gearId: string }) {
  const { data: docs } = useQuery({
    queryKey: ['gear-documents', gearId],
    queryFn: () => fetchGearDocuments(gearId),
  })
  const firstPhoto = docs?.find(d => d.content_type.startsWith('image/'))
  if (!firstPhoto) {
    return (
      <div className="w-full h-full flex items-center justify-center bg-neutral-100 dark:bg-neutral-800 text-neutral-400">
        <svg className="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M6.827 6.175A2.31 2.31 0 015.186 7.23c-.38.054-.757.112-1.134.175C2.999 7.58 2.25 8.507 2.25 9.574V18a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9.574c0-1.067-.75-1.994-1.802-2.169a47.865 47.865 0 00-1.134-.175 2.31 2.31 0 01-1.64-1.055l-.822-1.316a2.192 2.192 0 00-1.736-1.039 48.774 48.774 0 00-5.232 0 2.192 2.192 0 00-1.736 1.039l-.821 1.316z" />
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M16.5 12.75a4.5 4.5 0 11-9 0 4.5 4.5 0 019 0z" />
        </svg>
      </div>
    )
  }
  return (
    <img
      src={getGearDocumentUrl(firstPhoto.id)}
      alt=""
      className="w-full h-full object-cover"
      loading="lazy"
    />
  )
}

function GearCard({ gear }: { gear: Gear }) {
  const elevGain = formatElevationGain(gear.total_elevation_gain_m)

  return (
    <Link
      to={`/gear/${gear.id}`}
      className="block rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden hover:border-orange-300 dark:hover:border-orange-700 transition-colors"
    >
      <div className="h-32 w-full">
        <GearCardImage gearId={gear.id} />
      </div>

      <div className="p-4">
        <div className="flex items-start justify-between mb-2">
          <div className="min-w-0">
            <h3 className="text-base font-medium text-neutral-900 dark:text-neutral-100 truncate">
              {gear.name}
            </h3>
            <p className="text-sm text-neutral-500 dark:text-neutral-400">
              {gearTypeLabel(gear.type)}
              {gear.brand && ` - ${gear.brand}`}
              {gear.model && ` ${gear.model}`}
            </p>
          </div>
          <StatusBadge status={gear.status} />
        </div>

        <div className="grid grid-cols-4 gap-2 mt-3">
          <div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400">Distanz</div>
            <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 tabular-nums">
              {formatDistance(gear.total_distance_m)}
            </div>
          </div>
          <div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400">Hoehe</div>
            <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 tabular-nums">
              {elevGain ?? '\u2014'}
            </div>
          </div>
          <div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400">Zeit</div>
            <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 tabular-nums">
              {formatDuration(gear.total_time_s)}
            </div>
          </div>
          <div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400">Aktivitaeten</div>
            <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 tabular-nums">
              {gear.total_activities}
            </div>
          </div>
        </div>

        {gear.default_for_sports.length > 0 && (
          <div className="flex flex-wrap gap-1 mt-3">
            {gear.default_for_sports.map((sport) => (
              <span
                key={sport}
                className="px-1.5 py-0.5 rounded text-xs bg-orange-50 text-orange-600 dark:bg-orange-900/20 dark:text-orange-400"
              >
                {sport}
              </span>
            ))}
          </div>
        )}
      </div>
    </Link>
  )
}

function StatusBadge({ status }: { status: GearStatus }) {
  if (status === 'active') return null
  const styles: Record<string, string> = {
    stored: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400',
    retired: 'bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400',
  }
  const labels: Record<string, string> = {
    stored: 'Gelagert',
    retired: 'Ausgemustert',
  }
  return (
    <span className={`flex-shrink-0 px-2 py-0.5 rounded text-xs font-medium ${styles[status] ?? ''}`}>
      {labels[status] ?? status}
    </span>
  )
}

function gearTypeLabel(type: string): string {
  const labels: Record<string, string> = {
    bike: 'Fahrrad',
    shoes: 'Laufschuhe',
    goggles: 'Schwimmbrille',
    ski: 'Ski',
    paddle: 'Paddel',
    other: 'Sonstiges',
  }
  return labels[type] ?? type
}
