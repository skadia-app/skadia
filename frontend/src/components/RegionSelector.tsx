import { useState } from 'react'

interface RegionItem {
  id: string
  label: string
  continent?: string
}

interface RegionSelectorProps {
  regions: RegionItem[]
  selected: string[]
  onChange: (ids: string[]) => void
  showContinentTabs?: boolean
  hint?: string
}

export default function RegionSelector({ regions, selected, onChange, showContinentTabs, hint }: RegionSelectorProps) {
  const continents = showContinentTabs
    ? ([...new Set(regions.map(r => r.continent).filter(Boolean))] as string[])
    : []

  const [activeContinent, setActiveContinent] = useState(continents[0] ?? '')

  const visibleRegions = showContinentTabs && activeContinent
    ? regions.filter(r => r.continent === activeContinent)
    : regions

  const allSelected = visibleRegions.length > 0 && visibleRegions.every(r => selected.includes(r.id))

  function toggleAll() {
    if (allSelected) {
      onChange(selected.filter(id => !visibleRegions.some(r => r.id === id)))
    } else {
      onChange([...new Set([...selected, ...visibleRegions.map(r => r.id)])])
    }
  }

  function toggle(id: string) {
    onChange(selected.includes(id) ? selected.filter(s => s !== id) : [...selected, id])
  }

  return (
    <div>
      {showContinentTabs && continents.length > 1 && (
        <div className="flex flex-wrap gap-1 mb-2">
          {continents.map(c => (
            <button
              key={c}
              onClick={() => setActiveContinent(c)}
              className={`px-3 py-1 rounded-full text-xs font-medium transition-colors ${
                activeContinent === c
                  ? 'bg-accent-DEFAULT text-white'
                  : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
              }`}
            >
              {c}
            </button>
          ))}
        </div>
      )}
      <div className="flex flex-wrap gap-1.5 mb-1">
        <button
          onClick={toggleAll}
          className={`px-2.5 py-1 rounded-lg text-xs font-medium border transition-colors ${
            allSelected
              ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
              : 'bg-white dark:bg-neutral-800 border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 hover:border-accent-DEFAULT'
          }`}
        >
          Alle
        </button>
        {visibleRegions.map(r => (
          <button
            key={r.id}
            onClick={() => toggle(r.id)}
            className={`px-2.5 py-1 rounded-lg text-xs font-medium border transition-colors ${
              selected.includes(r.id)
                ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
                : 'bg-white dark:bg-neutral-800 border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:border-accent-DEFAULT'
            }`}
          >
            {r.label}
          </button>
        ))}
      </div>
      {hint && <p className="text-[11px] text-neutral-500 dark:text-neutral-500 italic">{hint}</p>}
    </div>
  )
}
