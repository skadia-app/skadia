import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import type { Sensor } from '@/api/types'
import { batteryStatusLabel, connectionTypeLabel } from '@/lib/deviceUtils'

interface SensorListProps {
  sensors: Sensor[]
}

export function SensorList({ sensors }: SensorListProps) {
  const { t } = useTranslation()

  return (
    <div>
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-3">{t('activityTables.sensors.title')}</h2>
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
        {sensors.map(sensor => {
          const isInvalidManufacturer = !sensor.manufacturer || sensor.manufacturer.includes('ManufacturerInvalid')
          const title = sensor.product_name || sensor.primary_type_label || sensor.primary_type || t('common.unknown')
          const sourceLabel = connectionTypeLabel(sensor.source_type)
          const cardContent = (
            <>
              {/* Header: product name + connection type badge */}
              <div className="flex items-center gap-2">
                <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">
                  {title}
                </span>
                {sourceLabel && (
                  <span className="shrink-0 text-xs px-1.5 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400">
                    {sourceLabel}
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
                <div className="text-xs text-neutral-400">{t('activityTables.sensors.battery')}: {batteryStatusLabel(sensor.battery_status, t)}</div>
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
