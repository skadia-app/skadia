import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { Pencil, Check, X } from 'lucide-react'
import { fetchDevice, fetchDeviceActivities, updateDeviceCustomName } from '@/api/hardware'
import type { DeviceActivity } from '@/api/hardware'
import { formatDistance } from '@/lib/utils'
import { sportLabel } from '@/lib/utils'
import { connectionTypeLabel, deviceTypeBadgeLabel } from '@/lib/deviceUtils'

function ActivityRow({ activity }: { activity: DeviceActivity }) {
  const date = new Date(activity.started_at).toLocaleDateString('de-DE', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })

  return (
    <Link
      to={`/activities/${activity.id}`}
      className="flex items-center justify-between px-4 py-3 rounded-lg border border-neutral-200 dark:border-neutral-800
                 hover:border-accent-400 dark:hover:border-accent-600 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 transition-all"
    >
      <div className="min-w-0 flex-1">
        <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">
          {activity.title || 'Unbenannt'}
        </div>
        <div className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
          {sportLabel(activity.sport_type)} · {date}
        </div>
      </div>
      {activity.distance_m > 0 && (
        <div className="ml-3 text-sm font-medium text-neutral-700 dark:text-neutral-300 tabular-nums flex-shrink-0">
          {formatDistance(activity.distance_m)}
        </div>
      )}
    </Link>
  )
}

export default function DeviceDetailPage() {
  const { id } = useParams<{ id: string }>()
  const queryClient = useQueryClient()
  const [editingName, setEditingName] = useState(false)
  const [nameInput, setNameInput] = useState('')
  const [cursor, setCursor] = useState<string | undefined>(undefined)
  const [allActivities, setAllActivities] = useState<DeviceActivity[]>([])
  const [hasLoadedOnce, setHasLoadedOnce] = useState(false)

  const { data: device, isLoading: deviceLoading } = useQuery({
    queryKey: ['device', id],
    queryFn: () => fetchDevice(id!),
    enabled: !!id,
  })

  const { data: activities, isLoading: activitiesLoading, isFetching } = useQuery({
    queryKey: ['device-activities', id, cursor],
    queryFn: async () => {
      const result = await fetchDeviceActivities(id!, cursor)
      if (!hasLoadedOnce) {
        setAllActivities(result)
        setHasLoadedOnce(true)
      } else {
        setAllActivities(prev => {
          const existingIds = new Set(prev.map(a => a.id))
          const newOnes = result.filter(a => !existingIds.has(a.id))
          return [...prev, ...newOnes]
        })
      }
      return result
    },
    enabled: !!id,
  })

  const renameMutation = useMutation({
    mutationFn: (name: string) => updateDeviceCustomName(id!, name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['device', id] })
      queryClient.invalidateQueries({ queryKey: ['devices'] })
      setEditingName(false)
    },
  })

  if (deviceLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Gerät...</div>
  }

  if (!device) {
    return (
      <div className="text-center py-16 text-neutral-400">
        Gerät nicht gefunden.
      </div>
    )
  }

  const displayName = device.custom_name || device.product_name || device.manufacturer || 'Unbekannt'
  const backType = device.type ?? 'recorder'
  const backLabel = backType === 'recorder' ? 'Aufzeichnungsgeräte' : 'Sensoren'

  const firstSeen = device.first_seen_at
    ? new Date(device.first_seen_at).toLocaleDateString('de-DE', { year: 'numeric', month: 'short', day: 'numeric' })
    : null
  const lastSeen = device.last_seen_at
    ? new Date(device.last_seen_at).toLocaleDateString('de-DE', { year: 'numeric', month: 'short', day: 'numeric' })
    : null

  const connType = connectionTypeLabel(device.source_type)
  const connBadgeStyle = device.source_type === 'antplus'
    ? 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-400'
    : 'bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-400'

  const pageSize = 50
  const hasMore = (activities?.length ?? 0) >= pageSize
  const lastActivity = allActivities[allActivities.length - 1]

  return (
    <div className="space-y-6">
      {/* Header with back link */}
      <div className="flex items-center gap-3">
        <Link
          to={`/hardware/${backType}`}
          className="text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100 transition-colors"
        >
          ← {backLabel}
        </Link>
      </div>

      {/* Device name + inline edit */}
      <div className="flex items-center gap-3">
        {editingName ? (
          <div className="flex items-center gap-2 flex-1">
            <input
              type="text"
              value={nameInput}
              onChange={e => setNameInput(e.target.value)}
              onKeyDown={e => {
                if (e.key === 'Enter') renameMutation.mutate(nameInput)
                if (e.key === 'Escape') setEditingName(false)
              }}
              className="text-2xl font-semibold bg-transparent border-b-2 border-accent-DEFAULT outline-none text-neutral-900 dark:text-neutral-100 flex-1"
              autoFocus
            />
            <button
              onClick={() => renameMutation.mutate(nameInput)}
              disabled={renameMutation.isPending}
              className="p-1.5 rounded-lg text-emerald-600 hover:bg-emerald-50 dark:hover:bg-emerald-900/20 transition-colors cursor-pointer disabled:opacity-50"
              title="Speichern"
            >
              <Check className="w-5 h-5" />
            </button>
            <button
              onClick={() => setEditingName(false)}
              className="p-1.5 rounded-lg text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
              title="Abbrechen"
            >
              <X className="w-5 h-5" />
            </button>
          </div>
        ) : (
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
              {displayName}
            </h1>
            <button
              onClick={() => {
                setNameInput(device.custom_name ?? '')
                setEditingName(true)
              }}
              className="p-1.5 rounded-lg text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
              title="Name bearbeiten"
            >
              <Pencil className="w-4 h-4" />
            </button>
          </div>
        )}
      </div>

      {/* Info grid */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5">
        <h2 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wider mb-4">
          Geräteinformationen
        </h2>
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
          {device.manufacturer && (
            <div>
              <div className="text-xs text-neutral-500 dark:text-neutral-400">Hersteller</div>
              <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 mt-0.5">
                {device.manufacturer}
              </div>
            </div>
          )}
          {device.product_name && (
            <div>
              <div className="text-xs text-neutral-500 dark:text-neutral-400">Produkt</div>
              <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 mt-0.5">
                {device.product_name}
              </div>
            </div>
          )}
          {device.serial_number != null && (
            <div>
              <div className="text-xs text-neutral-500 dark:text-neutral-400">Seriennummer</div>
              <div className="text-sm font-medium font-mono text-neutral-900 dark:text-neutral-100 mt-0.5">
                {device.serial_number}
              </div>
            </div>
          )}
          <div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400">Verbindung</div>
            <div className="mt-0.5">
              <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${connBadgeStyle}`}>
                {connType}
              </span>
            </div>
          </div>
          {firstSeen && (
            <div>
              <div className="text-xs text-neutral-500 dark:text-neutral-400">Zuerst gesehen</div>
              <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 mt-0.5">
                {firstSeen}
              </div>
            </div>
          )}
          {lastSeen && (
            <div>
              <div className="text-xs text-neutral-500 dark:text-neutral-400">Zuletzt gesehen</div>
              <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 mt-0.5">
                {lastSeen}
              </div>
            </div>
          )}
        </div>

        {/* Sensor type badges */}
        {device.device_types && device.device_types.length > 0 && (
          <div className="mt-4 pt-4 border-t border-neutral-100 dark:border-neutral-800">
            <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-2">Sensortypen</div>
            <div className="flex flex-wrap gap-1.5">
              {device.device_types.map(type_ => (
                <span
                  key={type_}
                  className="text-xs px-1.5 py-0.5 rounded-full bg-blue-50 dark:bg-blue-900/20 text-blue-600 dark:text-blue-400"
                >
                  {deviceTypeBadgeLabel(type_)}
                </span>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* Activity list */}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
            Aktivitäten
            {device.activity_count > 0 && (
              <span className="ml-2 text-sm font-normal text-neutral-500 dark:text-neutral-400">
                ({device.activity_count})
              </span>
            )}
          </h2>
        </div>

        {activitiesLoading && allActivities.length === 0 ? (
          <div className="text-center py-8 text-neutral-400">Lade Aktivitäten...</div>
        ) : allActivities.length > 0 ? (
          <>
            <div className="space-y-2">
              {allActivities.map(activity => (
                <ActivityRow key={activity.id} activity={activity} />
              ))}
            </div>
            {hasMore && (
              <div className="pt-2 text-center">
                <button
                  onClick={() => {
                    if (lastActivity) {
                      setCursor(lastActivity.started_at)
                    }
                  }}
                  disabled={isFetching}
                  className="px-4 py-2 rounded-lg text-sm font-medium border border-neutral-200 dark:border-neutral-800 text-neutral-600 dark:text-neutral-400
                             hover:border-accent-400 dark:hover:border-accent-600 hover:text-neutral-900 dark:hover:text-neutral-100 transition-all cursor-pointer disabled:opacity-50"
                >
                  {isFetching ? 'Lade...' : 'Mehr laden'}
                </button>
              </div>
            )}
          </>
        ) : (
          <div className="text-center py-8 text-neutral-400">
            Keine Aktivitäten für dieses Gerät.
          </div>
        )}
      </div>
    </div>
  )
}
