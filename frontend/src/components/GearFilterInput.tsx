import { useState, useRef, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchGear } from '@/api/gear'
import type { Gear } from '@/api/types'

export type GearFilterState = 'include' | 'exclude'

export interface GearFilter {
  id: string
  name: string
  type: string
  state: GearFilterState
}

interface Props {
  value: GearFilter[]
  onChange: (filters: GearFilter[]) => void
  placeholder?: string
}

function gearTypeIcon(type: string): string {
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

export default function GearFilterInput({ value, onChange, placeholder = 'Ausrüstung suchen...' }: Props) {
  const [search, setSearch] = useState('')
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  const { data: allGear = [] } = useQuery({
    queryKey: ['gear'],
    queryFn: () => fetchGear(),
  })

  // Only show active (non-retired) gear
  const activeGear = allGear.filter((g: Gear) => !g.is_retired)

  // Close dropdown on outside click
  useEffect(() => {
    if (!open) return
    function handleClick(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  const selectedIds = new Set(value.map((f) => f.id))

  const filtered = activeGear.filter(
    (g: Gear) =>
      !selectedIds.has(g.id) &&
      g.name.toLowerCase().includes(search.toLowerCase().trim()),
  )

  function addGear(gear: Gear) {
    onChange([...value, { id: gear.id, name: gear.name, type: gear.type, state: 'include' }])
    setSearch('')
    // Keep open so another item can be selected immediately;
    // the selected gear disappears from the list automatically.
    setOpen(true)
  }

  function cycleChip(id: string) {
    const chip = value.find((f) => f.id === id)
    if (!chip) return
    if (chip.state === 'include') {
      // include → exclude
      onChange(value.map((f) => f.id === id ? { ...f, state: 'exclude' as GearFilterState } : f))
    } else {
      // exclude → remove
      onChange(value.filter((f) => f.id !== id))
    }
  }

  return (
    <div ref={containerRef} className="relative">
      {/* Selected chips */}
      {value.length > 0 && (
        <div className="mb-2">
          <div className="flex flex-wrap gap-1.5 mb-1">
          {value.map((chip) => (
            <button
              key={chip.id}
              type="button"
              onClick={() => cycleChip(chip.id)}
              className={[
                'inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium border transition-all',
                chip.state === 'include'
                  ? 'bg-green-100 dark:bg-green-900/40 border-green-400 dark:border-green-600 text-green-800 dark:text-green-200'
                  : 'bg-red-100 dark:bg-red-900/40 border-red-400 dark:border-red-600 text-red-700 dark:text-red-300 line-through',
              ].join(' ')}
            >
              <span className="flex-shrink-0" aria-hidden="true">
                {gearTypeIcon(chip.type)}
              </span>
              {chip.name}
              <span className="ml-0.5 opacity-60 text-[10px]">
                {chip.state === 'include' ? '✓' : '✕'}
              </span>
            </button>
          ))}
          </div>
          <p className="text-[10px] text-neutral-400 dark:text-neutral-500">
            Klick: einschließen → ausschließen → entfernen
          </p>
        </div>
      )}

      {/* Search input */}
      <div className="relative">
        <input
          type="text"
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
            setOpen(true)
          }}
          onFocus={() => setOpen(true)}
          placeholder={placeholder}
          className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent placeholder-neutral-400 dark:placeholder-neutral-500"
        />
        {search && (
          <button
            type="button"
            onClick={() => { setSearch(''); setOpen(false) }}
            className="absolute right-2 top-1/2 -translate-y-1/2 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200"
          >
            ✕
          </button>
        )}
      </div>

      {/* Dropdown */}
      {open && filtered.length > 0 && (
        <div className="absolute z-50 mt-1 w-full bg-white dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-700 rounded-lg shadow-lg max-h-48 overflow-y-auto">
          {filtered.map((gear: Gear) => (
            <button
              key={gear.id}
              type="button"
              onMouseDown={(e) => {
                e.preventDefault() // prevent blur before click
                addGear(gear)
              }}
              className="w-full flex items-center gap-2 px-3 py-2 text-sm text-neutral-800 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors text-left"
            >
              <span className="flex-shrink-0" aria-hidden="true">
                {gearTypeIcon(gear.type)}
              </span>
              <span>{gear.name}</span>
              <span className="ml-auto text-xs text-neutral-400 dark:text-neutral-500">
                {gearTypeLabel(gear.type)}
              </span>
            </button>
          ))}
        </div>
      )}

      {open && search.trim() !== '' && filtered.length === 0 && (
        <div className="absolute z-50 mt-1 w-full bg-white dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-700 rounded-lg shadow-lg px-3 py-2 text-sm text-neutral-400 dark:text-neutral-500">
          Keine Ausrüstung gefunden
        </div>
      )}
    </div>
  )
}
