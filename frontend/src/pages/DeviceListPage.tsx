import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { fetchDevices } from '@/api/hardware'
import type { Device } from '@/api/hardware'
import { connectionTypeLabel, deviceTypeBadgeLabel } from '@/lib/deviceUtils'

function DeviceCard({ device }: { device: Device }) {
  const name = device.custom_name || device.product_name || device.manufacturer || 'Unbekannt'
  const connType = connectionTypeLabel(device.source_type)
  const connBadgeStyle = device.source_type === 'antplus'
    ? 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-400'
    : 'bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-400'

  const firstSeen = device.first_seen_at
    ? new Date(device.first_seen_at).toLocaleDateString('de-DE', { year: 'numeric', month: 'short', day: 'numeric' })
    : null
  const lastSeen = device.last_seen_at
    ? new Date(device.last_seen_at).toLocaleDateString('de-DE', { year: 'numeric', month: 'short', day: 'numeric' })
    : null

  return (
    <Link
      to={`/hardware/detail/${device.id}`}
      className="group rounded-xl border border-neutral-200 dark:border-neutral-800 p-4 space-y-3
                 hover:border-accent-400 dark:hover:border-accent-600 hover:shadow-sm transition-all block"
    >
      {/* Header */}
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 className="text-base font-medium text-neutral-900 dark:text-neutral-100 truncate group-hover:text-accent-DEFAULT transition-colors">
            {name}
          </h3>
          {device.manufacturer && device.product_name && (
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">{device.manufacturer}</p>
          )}
        </div>
        <span className={`flex-shrink-0 text-xs px-2 py-0.5 rounded-full font-medium ${connBadgeStyle}`}>
          {connType}
        </span>
      </div>

      {/* Sensor type badges */}
      {device.device_types && device.device_types.length > 0 && (
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
      )}

      {/* Activity count */}
      <div className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">
        {device.activity_count}{' '}
        <span className="text-sm font-normal text-neutral-500 dark:text-neutral-400">
          {device.activity_count === 1 ? 'Aktivität' : 'Aktivitäten'}
        </span>
      </div>

      {/* Footer info */}
      <div className="pt-2 border-t border-neutral-100 dark:border-neutral-800 space-y-1">
        {device.serial_number != null && (
          <div className="text-xs text-neutral-400 dark:text-neutral-500">
            S/N: <span className="font-mono">{device.serial_number}</span>
          </div>
        )}
        <div className="flex items-center justify-between text-xs text-neutral-400 dark:text-neutral-500">
          {firstSeen && <span>Zuerst: {firstSeen}</span>}
          {lastSeen && <span>Zuletzt: {lastSeen}</span>}
        </div>
      </div>
    </Link>
  )
}

export default function DeviceListPage() {
  const { type } = useParams<{ type: string }>()
  const effectiveType = type ?? 'recorder'

  const { data: devices, isLoading } = useQuery({
    queryKey: ['devices', effectiveType],
    queryFn: () => fetchDevices(effectiveType),
  })

  const title = effectiveType === 'recorder' ? 'Aufzeichnungsgeräte' : 'Sensoren'

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Link
          to="/gear"
          className="text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100 transition-colors"
        >
          ← Ausrüstung
        </Link>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">{title}</h1>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center h-64 text-neutral-400">Lade {title}...</div>
      ) : devices && devices.length > 0 ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {devices.map(device => (
            <DeviceCard key={device.id} device={device} />
          ))}
        </div>
      ) : (
        <div className="text-center py-16 text-neutral-400">
          Noch keine {title} vorhanden.
        </div>
      )}
    </div>
  )
}
