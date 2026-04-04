import { Link } from 'react-router-dom'
import type { Sensor } from '@/api/types'

interface SensorListProps {
  sensors: Sensor[]
}

function sensorSourceLabel(sourceType: string): string {
  switch (sourceType) {
    case 'antplus': return 'ANT+'
    case 'bluetooth': return 'BT'
    case 'app': return ''
    default: return sourceType
  }
}

export function SensorList({ sensors }: SensorListProps) {
  return (
    <div>
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-3">Sensoren</h2>
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
        {sensors.map(sensor => {
          const isInvalidManufacturer = !sensor.manufacturer || sensor.manufacturer.includes('ManufacturerInvalid')
          const title = sensor.product_name || sensor.primary_type_label || sensor.primary_type || 'Unbekannt'
          const cardContent = (
            <>
              {/* Header: product name + connection type badge */}
              <div className="flex items-center gap-2">
                <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">
                  {title}
                </span>
                {sensorSourceLabel(sensor.source_type) && (
                  <span className="shrink-0 text-xs px-1.5 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400">
                    {sensorSourceLabel(sensor.source_type)}
                  </span>
                )}
              </div>

              {/* Manufacturer (when separate from product name and valid) */}
              {!isInvalidManufacturer && sensor.manufacturer && sensor.product_name && (
                <div className="text-xs text-neutral-500 dark:text-neutral-400">{sensor.manufacturer}</div>
              )}

              {/* Sensor type badges — use labels when available */}
              <div className="flex flex-wrap gap-1.5">
                {sensor.all_types.map((type_, idx) => (
                  <span
                    key={type_}
                    className="text-xs px-1.5 py-0.5 rounded-full bg-blue-50 dark:bg-blue-900/20 text-blue-600 dark:text-blue-400"
                  >
                    {(sensor.all_type_labels && sensor.all_type_labels[idx]) || type_}
                  </span>
                ))}
              </div>

              {/* IDs: serial number and ANT+ device number */}
              {(sensor.serial_number != null || sensor.ant_device_number != null || sensor.serial_text) && (
                <div className="text-xs text-neutral-400 dark:text-neutral-500 space-y-0.5 pt-0.5 border-t border-neutral-100 dark:border-neutral-800">
                  {sensor.serial_number != null && (
                    <div>S/N: <span className="font-mono">{sensor.serial_number}</span></div>
                  )}
                  {sensor.serial_text && (
                    <div>S/N: <span className="font-mono">{sensor.serial_text}</span></div>
                  )}
                  {sensor.ant_device_number != null && (
                    <div>ANT+: <span className="font-mono">{sensor.ant_device_number}</span></div>
                  )}
                </div>
              )}

              {/* Battery status */}
              {sensor.battery_status && sensor.battery_status !== 'unknown' && (
                <div className="text-xs text-neutral-400">Batterie: {sensor.battery_status}</div>
              )}
            </>
          )

          const cardClass = "rounded-lg border border-neutral-200 dark:border-neutral-800 p-3 space-y-1.5"

          return sensor.device_id ? (
            <Link
              key={sensor.id}
              to={`/hardware/detail/${sensor.device_id}`}
              className={`${cardClass} block hover:border-accent-400 transition-colors`}
            >
              {cardContent}
            </Link>
          ) : (
            <div key={sensor.id} className={cardClass}>
              {cardContent}
            </div>
          )
        })}
      </div>
    </div>
  )
}
