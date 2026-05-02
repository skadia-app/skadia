import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchSportTypeStats } from '@/api/stats'
import { useGlobalSportSettings } from '@/lib/globalSportSettings'
import { filterToExistingSports, getAllSportKeys, getOptionKey, type SportOption } from '@/lib/sportOptions'
import { sportFilterOptionsForPreset, type SportFilterPreset } from '@/lib/sportFilterPresets'

export interface SportFilterValue {
  sportTypes: string[] | null
  predicates?: Array<{ sport: string; sub_type?: string }>
  modifiers?: {
    is_electric?: boolean
    is_indoor?: boolean
    is_virtual?: boolean
    is_commute?: boolean
  }
}

type ModifierKey = keyof NonNullable<SportFilterValue['modifiers']>

interface SportFilterProps {
  preset: SportFilterPreset
  value: SportFilterValue
  onChange: (value: SportFilterValue) => void
  showModifiers?: boolean
  modifierKeys?: ModifierKey[]
  className?: string
}

const MODIFIER_LABELS: Record<ModifierKey, string> = {
  is_electric: 'E-Bike',
  is_indoor: 'Indoor',
  is_virtual: 'Virtuell',
  is_commute: 'Pendeln',
}

export function SportFilter({
  preset,
  value,
  onChange,
  showModifiers = true,
  modifierKeys = ['is_electric', 'is_indoor', 'is_virtual', 'is_commute'],
  className,
}: SportFilterProps) {
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set())
  const { hideUnused } = useGlobalSportSettings()
  const { data: statsData } = useQuery({
    queryKey: ['sport-type-stats'],
    queryFn: fetchSportTypeStats,
    enabled: hideUnused,
  })
  const baseOptions = useMemo(() => sportFilterOptionsForPreset(preset), [preset])
  const existingSportKeys = useMemo(() => new Set((statsData ?? []).map((s) => s.sub_type ? `${s.sport_type}:${s.sub_type}` : s.sport_type)), [statsData])
  const options = useMemo(() => {
    if (hideUnused && existingSportKeys.size > 0) {
      return filterToExistingSports(baseOptions, existingSportKeys)
    }
    return baseOptions
  }, [baseOptions, existingSportKeys, hideUnused])
  const allKeys = useMemo(() => getAllSportKeys(options), [options])
  const selected = useMemo(() => new Set(value.sportTypes ?? allKeys), [allKeys, value.sportTypes])
  const allSelected = allKeys.length > 0 && allKeys.every((key) => selected.has(key))

  function emit(next: Set<string>) {
    // null = "all selected, no filter applied" (default state)
    // []   = "explicitly none selected" (after Alle abwählen)
    // [..] = "specific subset selected"
    // Existing consumers (ActivityFilters, MapFilters, SportOnlyFilterBar)
    // collapse null and [] to the same Set on input, so this distinction
    // only matters for callers that treat empty as a real selection.
    let sportTypes: string[] | null
    if (allKeys.length > 0 && allKeys.every((key) => next.has(key))) {
      sportTypes = null
    } else if (next.size === 0) {
      sportTypes = []
    } else {
      sportTypes = [...next]
    }
    onChange({ ...value, sportTypes })
  }

  function toggleSport(key: string) {
    const next = new Set(selected)
    if (next.has(key)) next.delete(key)
    else next.add(key)
    emit(next)
  }

  function toggleParent(parent: SportOption) {
    if (!parent.children) return
    const childKeys = parent.children.map(getOptionKey)
    const allChildrenSelected = childKeys.every((key) => selected.has(key))
    const next = new Set(selected)
    if (allChildrenSelected) childKeys.forEach((key) => next.delete(key))
    else childKeys.forEach((key) => next.add(key))
    emit(next)
  }

  function toggleAll() {
    emit(allSelected ? new Set<string>() : new Set(allKeys))
  }

  function setModifier(key: keyof NonNullable<SportFilterValue['modifiers']>, next: boolean | undefined) {
    const modifiers = { ...(value.modifiers ?? {}) }
    if (next === undefined) delete modifiers[key]
    else modifiers[key] = next
    onChange({ ...value, modifiers })
  }

  return (
    <div className={className}>
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Sportart</span>
        <button
          type="button"
          onClick={toggleAll}
          className="text-xs text-accent-DEFAULT hover:text-accent-600 cursor-pointer"
        >
          {allSelected ? 'Alle abwählen' : 'Alle auswählen'}
        </button>
      </div>
      <div className="space-y-1.5">
        {options.map((opt) => (
          <div key={opt.label}>
            {opt.children && opt.groupKey ? (
              <div>
                <div className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={opt.children.every((child) => selected.has(getOptionKey(child)))}
                    ref={(el) => {
                      if (!el) return
                      const checked = opt.children!.filter((child) => selected.has(getOptionKey(child))).length
                      el.indeterminate = checked > 0 && checked < opt.children!.length
                    }}
                    onChange={() => toggleParent(opt)}
                    className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT focus:ring-1"
                  />
                  <button
                    type="button"
                    onClick={() => setExpandedGroups((prev) => {
                      const next = new Set(prev)
                      if (next.has(opt.groupKey!)) next.delete(opt.groupKey!)
                      else next.add(opt.groupKey!)
                      return next
                    })}
                    className="flex items-center gap-1 text-sm text-neutral-700 dark:text-neutral-300 hover:text-neutral-900 dark:hover:text-neutral-100 cursor-pointer"
                  >
                    <svg
                      className={`w-3 h-3 text-neutral-400 transition-transform ${expandedGroups.has(opt.groupKey!) ? '' : '-rotate-90'}`}
                      viewBox="0 0 20 20"
                      fill="currentColor"
                    >
                      <path fillRule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clipRule="evenodd" />
                    </svg>
                    {opt.label}
                  </button>
                </div>
                {expandedGroups.has(opt.groupKey) && (
                  <div className="ml-6 mt-1 space-y-1">
                    {opt.children.map((child) => {
                      const key = getOptionKey(child)
                      return (
                        <label key={key} className="flex items-center gap-2 cursor-pointer group">
                          <input
                            type="checkbox"
                            checked={selected.has(key)}
                            onChange={() => toggleSport(key)}
                            className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT focus:ring-1"
                          />
                          <span className="text-sm text-neutral-600 dark:text-neutral-400 group-hover:text-neutral-900 dark:group-hover:text-neutral-100">
                            {child.label}
                          </span>
                        </label>
                      )
                    })}
                  </div>
                )}
              </div>
            ) : (
              <label className="flex items-center gap-2 cursor-pointer group">
                <input
                  type="checkbox"
                  checked={selected.has(opt.value)}
                  onChange={() => toggleSport(opt.value)}
                  className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT focus:ring-1"
                />
                <span className="text-sm text-neutral-700 dark:text-neutral-300 group-hover:text-neutral-900 dark:group-hover:text-neutral-100">
                  {opt.label}
                </span>
              </label>
            )}
          </div>
        ))}
      </div>
      {showModifiers && (
        <div className="mt-3 grid grid-cols-2 gap-2">
          {modifierKeys.map((key) => (
            <ModifierSelect
              key={key}
              label={MODIFIER_LABELS[key]}
              value={value.modifiers?.[key]}
              onChange={(v) => setModifier(key, v)}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function ModifierSelect({ label, value, onChange }: { label: string; value?: boolean; onChange: (value: boolean | undefined) => void }) {
  return (
    <label className="text-xs text-neutral-500 dark:text-neutral-400">
      <span className="block mb-1">{label}</span>
      <select
        value={value === undefined ? '' : String(value)}
        onChange={(e) => onChange(e.target.value === '' ? undefined : e.target.value === 'true')}
        className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-2 py-1 text-xs text-neutral-800 dark:text-neutral-200"
      >
        <option value="">Alle</option>
        <option value="true">Nur</option>
        <option value="false">Ohne</option>
      </select>
    </label>
  )
}
