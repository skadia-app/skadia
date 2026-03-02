import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { fetchGear, createGear } from '@/api/gear'
import type { Gear } from '@/api/types'
import { formatDistance, formatDuration } from '@/lib/utils'
import GearForm from '@/components/GearForm'
import MaintenanceAlerts from '@/components/MaintenanceAlerts'

export default function GearPage() {
  const queryClient = useQueryClient()
  const [showRetired, setShowRetired] = useState(false)
  const [showForm, setShowForm] = useState(false)

  const { data: gear, isLoading } = useQuery({
    queryKey: ['gear', showRetired],
    queryFn: () => fetchGear(showRetired),
  })

  const createMutation = useMutation({
    mutationFn: (data: Partial<Gear>) => createGear(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gear'] })
      setShowForm(false)
    },
  })

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Ausruestung...</div>
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Ausruestung</h1>
        <div className="flex items-center gap-3">
          <label className="flex items-center gap-2 cursor-pointer text-sm text-neutral-500 dark:text-neutral-400">
            <input
              type="checkbox"
              checked={showRetired}
              onChange={() => setShowRetired(!showRetired)}
              className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-orange-500 focus:ring-orange-500 focus:ring-1"
            />
            Ausgemustert anzeigen
          </label>
          <button
            onClick={() => setShowForm(true)}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 transition-colors"
          >
            Ausruestung hinzufuegen
          </button>
        </div>
      </div>

      <MaintenanceAlerts />

      {/* Create form */}
      {showForm && (
        <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100 mb-4">Neue Ausruestung</h2>
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
          Noch keine Ausruestung vorhanden.
        </div>
      )}
    </div>
  )
}

function GearCard({ gear }: { gear: Gear }) {
  return (
    <Link
      to={`/gear/${gear.id}`}
      className="block rounded-xl border border-neutral-200 dark:border-neutral-800 p-4 hover:border-orange-300 dark:hover:border-orange-700 transition-colors"
    >
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
        {gear.is_retired && (
          <span className="flex-shrink-0 px-2 py-0.5 rounded text-xs font-medium bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400">
            Ausgemustert
          </span>
        )}
      </div>

      <div className="grid grid-cols-3 gap-2 mt-3">
        <div>
          <div className="text-xs text-neutral-500 dark:text-neutral-400">Distanz</div>
          <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 tabular-nums">
            {formatDistance(gear.total_distance_m)}
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
    </Link>
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
