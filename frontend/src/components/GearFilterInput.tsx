import { useState, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchGear } from '@/api/gear'
import { fetchDevices } from '@/api/hardware'
import type { Gear } from '@/api/types'
import type { Device } from '@/api/hardware'

export type GearFilterState = 'include' | 'exclude'

export interface GearFilter {
  id: string
  name: string
  type: string
  state: GearFilterState
}

export interface DeviceFilter {
  id: string
  name: string
  deviceType: 'recorder' | 'sensor'
  state: GearFilterState
}

interface Props {
  value: GearFilter[]
  onChange: (filters: GearFilter[]) => void
  deviceValue?: DeviceFilter[]
  onDeviceChange?: (filters: DeviceFilter[]) => void
  placeholder?: string
  /** Hide chips and hint — for composite layouts that render chips externally */
  compact?: boolean
}

export function gearTypeIcon(type: string): string {
  const icons: Record<string, string> = {
    bike: '🚲',
    shoes: '👟',
    goggles: '🥽',
    ski: '⛷',
    paddle: '🛶',
    other: '🎒',
  }
  return icons[type] ?? '🎒'
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

function deviceLabel(d: Device): string {
  return d.custom_name || d.product_name || d.manufacturer || d.id
}

function deviceKindLabel(d: Device): string {
  return d.type === 'recorder' ? 'Gerät' : 'Sensor'
}

export default function GearFilterInput({
  value,
  onChange,
  deviceValue = [],
  onDeviceChange,
  placeholder = 'Ausrüstung / Gerät suchen...',
  compact,
}: Props) {
  const [search, setSearch] = useState('')
  const [listOpen, setListOpen] = useState(false)
  const listRef = useRef<HTMLDivElement>(null)

  const { data: allGear = [] } = useQuery({
    queryKey: ['gear'],
    queryFn: () => fetchGear(),
  })

  const { data: allDevices = [] } = useQuery({
    queryKey: ['devices-all'],
    queryFn: () => fetchDevices(),
    enabled: !!onDeviceChange,
  })

  // Only show active (non-retired) gear
  const activeGear = allGear.filter((g: Gear) => !g.is_retired)

  const filterMap = new Map(value.map((f) => [f.id, f]))
  const deviceFilterMap = new Map(deviceValue.map((f) => [f.id, f]))

  const searchLower = search.toLowerCase().trim()
  const filteredGear = activeGear.filter(
    (g: Gear) => g.name.toLowerCase().includes(searchLower),
  )
  const filteredDevices = onDeviceChange
    ? allDevices.filter((d: Device) => deviceLabel(d).toLowerCase().includes(searchLower))
    : []

  function cycleGear(gear: Gear) {
    const current = filterMap.get(gear.id)
    if (!current) {
      onChange([...value, { id: gear.id, name: gear.name, type: gear.type, state: 'include' }])
    } else if (current.state === 'include') {
      onChange(value.map((f) => f.id === gear.id ? { ...f, state: 'exclude' as GearFilterState } : f))
    } else {
      onChange(value.filter((f) => f.id !== gear.id))
    }
  }

  function cycleDevice(device: Device) {
    if (!onDeviceChange) return
    const current = deviceFilterMap.get(device.id)
    if (!current) {
      onDeviceChange([...deviceValue, {
        id: device.id,
        name: deviceLabel(device),
        deviceType: device.type,
        state: 'include',
      }])
    } else if (current.state === 'include') {
      onDeviceChange(deviceValue.map((f) => f.id === device.id ? { ...f, state: 'exclude' as GearFilterState } : f))
    } else {
      onDeviceChange(deviceValue.filter((f) => f.id !== device.id))
    }
  }

  function stateStyle(state: GearFilterState | undefined) {
    if (state === 'include') return 'bg-green-50 dark:bg-green-900/30 border-green-400 dark:border-green-700'
    if (state === 'exclude') return 'bg-red-50 dark:bg-red-900/30 border-red-400 dark:border-red-700'
    return 'border-transparent hover:bg-neutral-50 dark:hover:bg-neutral-800'
  }

  function stateIndicator(state: GearFilterState | undefined) {
    if (state === 'include') return <span className="ml-auto text-green-600 dark:text-green-400 text-xs">&#10003;</span>
    if (state === 'exclude') return <span className="ml-auto text-red-500 dark:text-red-400 text-xs">&#10005;</span>
    return null
  }

  const hasActiveGear = value.length > 0
  const hasActiveDevices = deviceValue.length > 0
  const hasActive = hasActiveGear || hasActiveDevices

  function handleOpen() {
    setListOpen(true)
    requestAnimationFrame(() => {
      listRef.current?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
    })
  }

  const showList = listOpen && (filteredGear.length > 0 || filteredDevices.length > 0)
  const showEmpty = listOpen && searchLower !== '' && filteredGear.length === 0 && filteredDevices.length === 0

  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">
          {onDeviceChange ? 'Ausrüstung & Geräte' : 'Ausrüstung'}
        </span>
        {hasActive && !compact && (
          <button
            type="button"
            onClick={() => { onChange([]); onDeviceChange?.([]) }}
            className="text-xs text-accent-DEFAULT hover:text-accent-600 cursor-pointer"
          >
            Zurücksetzen
          </button>
        )}
      </div>

      {/* Search input */}
      <div className="relative">
        <input
          type="text"
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
            handleOpen()
          }}
          onClick={handleOpen}
          onFocus={handleOpen}
          placeholder={placeholder}
          className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent placeholder-neutral-400 dark:placeholder-neutral-500"
        />
        {search && (
          <button
            type="button"
            onClick={() => { setSearch(''); setListOpen(false) }}
            className="absolute right-2 top-1/2 -translate-y-1/2 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200"
          >
            &#10005;
          </button>
        )}
      </div>

      {/* Combined list (inline, not absolute) */}
      {showList && (
        <div ref={listRef} className="mt-1.5 space-y-0.5 max-h-52 overflow-y-auto">
          {/* Gear section */}
          {filteredGear.length > 0 && (
            <>
              {onDeviceChange && filteredDevices.length > 0 && (
                <div className="px-2 pt-1 pb-0.5">
                  <span className="text-[10px] font-semibold uppercase tracking-wide text-neutral-400 dark:text-neutral-500">
                    Ausrüstung
                  </span>
                </div>
              )}
              {filteredGear.map((gear: Gear) => {
                const state = filterMap.get(gear.id)?.state
                return (
                  <button
                    key={gear.id}
                    type="button"
                    onClick={() => cycleGear(gear)}
                    className={`w-full flex items-center gap-2 px-2.5 py-1.5 text-sm rounded-md border transition-colors cursor-pointer text-left ${stateStyle(state)}`}
                  >
                    <span className="flex-shrink-0" aria-hidden="true">
                      {gearTypeIcon(gear.type)}
                    </span>
                    <span className={`text-neutral-800 dark:text-neutral-200 ${state === 'exclude' ? 'line-through' : ''}`}>
                      {gear.name}
                    </span>
                    {!state && (
                      <span className="ml-auto text-xs text-neutral-400 dark:text-neutral-500">
                        {gearTypeLabel(gear.type)}
                      </span>
                    )}
                    {stateIndicator(state)}
                  </button>
                )
              })}
            </>
          )}

          {/* Device section */}
          {onDeviceChange && filteredDevices.length > 0 && (
            <>
              {filteredGear.length > 0 && (
                <div className="px-2 pt-2 pb-0.5">
                  <span className="text-[10px] font-semibold uppercase tracking-wide text-neutral-400 dark:text-neutral-500">
                    Geräte & Sensoren
                  </span>
                </div>
              )}
              {filteredGear.length === 0 && (
                <div className="px-2 pt-1 pb-0.5">
                  <span className="text-[10px] font-semibold uppercase tracking-wide text-neutral-400 dark:text-neutral-500">
                    Geräte & Sensoren
                  </span>
                </div>
              )}
              {filteredDevices.map((device: Device) => {
                const state = deviceFilterMap.get(device.id)?.state
                return (
                  <button
                    key={device.id}
                    type="button"
                    onClick={() => cycleDevice(device)}
                    className={`w-full flex items-center gap-2 px-2.5 py-1.5 text-sm rounded-md border transition-colors cursor-pointer text-left ${stateStyle(state)}`}
                  >
                    <span className="flex-shrink-0 text-neutral-400" aria-hidden="true">
                      {device.type === 'recorder' ? '📡' : '📶'}
                    </span>
                    <span className={`text-neutral-800 dark:text-neutral-200 ${state === 'exclude' ? 'line-through' : ''}`}>
                      {deviceLabel(device)}
                    </span>
                    {!state && (
                      <span className="ml-auto text-xs text-neutral-400 dark:text-neutral-500">
                        {deviceKindLabel(device)}
                      </span>
                    )}
                    {stateIndicator(state)}
                  </button>
                )
              })}
            </>
          )}
        </div>
      )}

      {showEmpty && (
        <div className="mt-1.5 px-3 py-2 text-sm text-neutral-400 dark:text-neutral-500">
          Keine Ergebnisse gefunden
        </div>
      )}

      {hasActive && !compact && (
        <p className="text-[10px] text-neutral-400 dark:text-neutral-500 mt-1.5">
          Klick: einschließen &rarr; ausschließen &rarr; entfernen
        </p>
      )}
    </div>
  )
}
