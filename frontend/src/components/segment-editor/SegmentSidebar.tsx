// frontend/src/components/segment-editor/SegmentSidebar.tsx
import { useMemo, useState, type ReactNode } from 'react'
import { ChevronDown, ChevronRight, Loader2, RefreshCw, Check } from 'lucide-react'
import {
  WINTER_SPORT_WHITELIST,
  WATER_SPORT_WHITELIST,
  BIDIRECTIONAL_DEFAULT,
  type SegType,
  type ClimbType,
} from './constants'
import { Pill } from '../ui/Pill'
import { DifficultySelects } from './DifficultySelects'
import { SurfaceDropdown } from './SurfaceDropdown'
import { ClimbTypePills } from './ClimbTypePills'
import { DifficultyColorPicker } from './DifficultyColorPicker'
import { SportCategoryChips, getVisibleCategories, isCategoryFullyActive, expandCategoryToSportTypes } from './SportCategoryChips'
import { sportsInCategory } from '@/lib/sportTaxonomy'
import { cn } from '@/lib/utils'

export interface SegmentMetadata {
  name: string
  segType: SegType
  sportTypes: string[] | null
  sacScale: string | null
  mtbScale: string | null
  surface: string | null
  difficultyColor: 'blue' | 'red' | 'black' | null
  bidirectional: boolean
  userEditedBidirectional: boolean
  isPublic: boolean
  climbType?: ClimbType
}

export interface LiveInfo {
  distanceKm: number
  gainM: number
  lossM: number
  elevationMaxM?: number | null
}

export interface SegmentSidebarProps {
  mode: 'track' | 'draw' | 'lap'
  meta: SegmentMetadata
  onMetaChange: (patch: Partial<SegmentMetadata>) => void
  info: LiveInfo | null
  allSportTypes: Array<{ key: string; label: string; category: string }>
  onEnrichmentRequest: () => void
  enrichmentState: { loading: boolean; coverage?: number; lastRequestedAt?: number }
  enrichmentSources: Partial<Record<'surface' | 'sac' | 'mtb', { coverage: number } | null>>
  onClearEnrichmentSource: (field: 'surface' | 'sac' | 'mtb') => void
  enrichmentDisabledReason?: string
  enrichmentStatus?: string
}

const SEG_TYPE_LABELS: Record<SegType, string> = {
  standard: 'Standard',
  trail: 'Trail',
  lap: 'Runde',
  wintersport: 'Wintersport',
  wassersport: 'Wassersport',
  lift: 'Lift',
}

export function SegmentSidebar(props: SegmentSidebarProps) {
  const {
    meta,
    onMetaChange,
    info,
    mode,
    allSportTypes,
    onEnrichmentRequest,
    enrichmentState,
    enrichmentDisabledReason,
  } = props

  const [detailOpenCategories, setDetailOpenCategories] = useState<Set<string>>(new Set())

  const toggleDetailOpen = (uiKey: string) => {
    setDetailOpenCategories(prev => {
      const next = new Set(prev)
      if (next.has(uiKey)) next.delete(uiKey)
      else next.add(uiKey)
      return next
    })
  }

  const handleSegTypeChange = (newType: SegType) => {
    const patch: Partial<SegmentMetadata> = { segType: newType }
    if (!meta.userEditedBidirectional) {
      patch.bidirectional = BIDIRECTIONAL_DEFAULT[newType]
    }
    if (newType === 'wintersport' && meta.sportTypes) {
      patch.sportTypes = meta.sportTypes.filter(s => WINTER_SPORT_WHITELIST.includes(s))
      if (patch.sportTypes.length === 0) patch.sportTypes = null
    }
    if (newType === 'wassersport' && meta.sportTypes) {
      patch.sportTypes = meta.sportTypes.filter(s => WATER_SPORT_WHITELIST.includes(s))
      if (patch.sportTypes.length === 0) patch.sportTypes = null
    }
    if (newType === 'wintersport' || newType === 'wassersport' || newType === 'lift') {
      patch.sacScale = null
      patch.mtbScale = null
      patch.surface = null
    }
    onMetaChange(patch)
  }

  const handleBidirectionalToggle = () => {
   const newValue = !meta.bidirectional
   const defaultValue = BIDIRECTIONAL_DEFAULT[meta.segType] ?? true
   const isDefault = newValue === defaultValue
   onMetaChange({
     bidirectional: newValue,
     userEditedBidirectional: !isDefault,
   })
  }
  const toggleSportType = (key: string) => {
    const current = meta.sportTypes
    if (current === null) {
      onMetaChange({ sportTypes: [key] })
      return
    }
    const next = current.includes(key) ? current.filter(s => s !== key) : [...current, key]
    onMetaChange({ sportTypes: next.length === 0 ? null : next })
  }

  const setAll = () => onMetaChange({ sportTypes: null })

  const toggleUiCategory = (uiKey: string) => {
    const whitelist = meta.segType === 'wintersport' ? WINTER_SPORT_WHITELIST : (meta.segType === 'wassersport' ? WATER_SPORT_WHITELIST : undefined)
    const categoryTypes = expandCategoryToSportTypes(uiKey, whitelist)
    const current = meta.sportTypes

    if (isCategoryFullyActive(uiKey, current, whitelist)) {
      const next = current!.filter(t => !categoryTypes.includes(t))
      onMetaChange({ sportTypes: next.length === 0 ? null : next })
    } else {
      const base = current ?? []
      const next = Array.from(new Set([...base, ...categoryTypes]))
      onMetaChange({ sportTypes: next })
    }
  }

  const availableSportTypes = useMemo(() => {
    if (meta.segType === 'wintersport') return allSportTypes.filter(s => WINTER_SPORT_WHITELIST.includes(s.key))
    if (meta.segType === 'wassersport') return allSportTypes.filter(s => WATER_SPORT_WHITELIST.includes(s.key))
    return allSportTypes
  }, [meta.segType, allSportTypes])

  return (
    <aside className="flex h-full w-full flex-col overflow-y-auto border-r border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900">
      {info && (mode === 'track' || (mode === 'draw' && info.distanceKm > 0)) && (
        <div className="mx-3 mt-3 rounded-lg border border-neutral-200 dark:border-neutral-700/60 bg-neutral-50 dark:bg-neutral-800 p-3">
          <div className="grid grid-cols-3 gap-x-2 text-center">
            <MetricCell label="Distanz" value={`${info.distanceKm.toFixed(1)} km`} />
            <MetricCell label="Aufstieg" value={`+${Math.round(info.gainM)} m`} valueClassName="text-red-500" />
            <MetricCell label="Abstieg"  value={`−${Math.round(info.lossM)} m`} valueClassName="text-green-500" />
          </div>
          {info.elevationMaxM != null && (
            <div className="mt-2 border-t border-neutral-200 dark:border-neutral-700/60 pt-2 text-center">
              <MetricCell label="Höchstpunkt" value={`${Math.round(info.elevationMaxM)} m`} />
            </div>
          )}
        </div>
      )}

      <div className="flex flex-col gap-4 mx-3 py-3">
        <Field label="Name" required>
          <input
            type="text"
            value={meta.name}
            onChange={e => onMetaChange({ name: e.target.value })}
            placeholder="Segmentname"
            className={cn(
              'w-full rounded-md border border-neutral-300 dark:border-neutral-700',
              'bg-white dark:bg-neutral-800 px-3 py-1.5',
              'text-sm text-neutral-900 dark:text-neutral-100',
              'placeholder:text-neutral-400 dark:placeholder:text-neutral-500',
              'focus:outline-none focus:border-accent-DEFAULT focus:ring-1 focus:ring-accent-DEFAULT',
            )}
            required
          />
        </Field>

        <Field label="Typ">
          <div className="flex flex-wrap gap-1.5" role="radiogroup" aria-label="Segment-Typ">
            {(['standard', 'trail', 'lap', 'wintersport', 'wassersport', 'lift'] as SegType[]).map(t => (
              <Pill
                key={t}
                role="radio"
                ariaChecked={meta.segType === t}
                active={meta.segType === t}
                onClick={() => handleSegTypeChange(t)}
              >
                {SEG_TYPE_LABELS[t]}
              </Pill>
            ))}
          </div>
        </Field>

        <Field label="Sportarten">
          <SportCategoryChips
            segType={meta.segType}
            activeTypes={meta.sportTypes}
            onToggleCategory={toggleUiCategory}
            onSetAll={setAll}
          />

          {meta.sportTypes !== null && meta.segType !== 'wintersport' && meta.segType !== 'wassersport' && (
            <div className="mt-2 flex flex-col gap-1.5">
              {getVisibleCategories(meta.segType)
                .map(cat => {
                  const isOpen = detailOpenCategories.has(cat.key)
                  const catSports = cat.taxonomyKeys
                    .flatMap(tk => sportsInCategory(tk))
                    .filter(s => availableSportTypes.some(a => a.key === s.sportType))
                  if (catSports.length === 0) return null
                  return (
                    <div key={cat.key}>
                      <button
                        type="button"
                        onClick={() => toggleDetailOpen(cat.key)}
                        className="flex items-center gap-1 mb-1 group"
                      >
                        {isOpen
                          ? <ChevronDown className="h-3 w-3 text-neutral-400 dark:text-neutral-500 group-hover:text-neutral-600 dark:group-hover:text-neutral-300" />
                          : <ChevronRight className="h-3 w-3 text-neutral-400 dark:text-neutral-500 group-hover:text-neutral-600 dark:group-hover:text-neutral-300" />
                        }
                        <span className="text-[10px] font-semibold tracking-wider uppercase text-neutral-400 dark:text-neutral-500 group-hover:text-neutral-600 dark:group-hover:text-neutral-300">
                          {cat.label} — Feinwahl
                        </span>
                      </button>
                      {isOpen && (
                        <div className="flex flex-wrap gap-1 pl-3">
                          {catSports.map(s => {
                            const isActive = meta.sportTypes?.includes(s.sportType) ?? false
                            return (
                              <Pill
                                key={s.sportType}
                                size="sm"
                                role="checkbox"
                                ariaChecked={isActive}
                                active={isActive}
                                onClick={() => toggleSportType(s.sportType)}
                                leftSlot={isActive ? <Check className="h-3 w-3 shrink-0" /> : undefined}
                              >
                                {s.label}
                              </Pill>
                            )
                          })}
                        </div>
                      )}
                    </div>
                  )
                })
              }
            </div>
          )}
          
          {(meta.segType === 'wintersport' || meta.segType === 'wassersport') && meta.sportTypes !== null && (
             <div className="mt-2 flex flex-wrap gap-1">
               {availableSportTypes.map(s => {
                 const isActive = meta.sportTypes?.includes(s.key) ?? false
                 return (
                   <Pill
                     key={s.key}
                     size="sm"
                     role="checkbox"
                     ariaChecked={isActive}
                     active={isActive}
                     onClick={() => toggleSportType(s.key)}
                     leftSlot={isActive ? <Check className="h-3 w-3 shrink-0" /> : undefined}
                   >
                     {s.label}
                   </Pill>
                 )
               })}
             </div>
          )}
        </Field>

        <div>
          <div className="mb-2 flex items-center justify-between">
            <span className="text-[11px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">
              Difficulty &amp; Surface
            </span>
            <button
              type="button"
              onClick={onEnrichmentRequest}
              disabled={!!enrichmentDisabledReason || enrichmentState.loading}
              className={cn(
                'flex items-center gap-1 rounded-md px-2 py-1 text-[11px] font-medium',
                'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-300',
                'border border-neutral-200 dark:border-neutral-700 transition-colors hover:text-accent-DEFAULT disabled:opacity-40',
              )}
            >
              {enrichmentState.loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
            </button>
          </div>
          
          <div className="space-y-4 rounded-lg border border-neutral-200 dark:border-neutral-700 p-3">
             <DifficultySelects 
               segType={meta.segType} 
               sacScale={meta.sacScale} 
               mtbScale={meta.mtbScale} 
               onChange={(f, v) => onMetaChange(f === 'sac' ? { sacScale: v } : { mtbScale: v })}
             />
             <SurfaceDropdown
               segType={meta.segType}
               value={meta.surface}
               onChange={(v) => onMetaChange({ surface: v })}
             />
             <DifficultyColorPicker
               segType={meta.segType}
               value={meta.difficultyColor}
               onChange={(v) => onMetaChange({ difficultyColor: v })}
             />
             {!['trail', 'wintersport'].includes(meta.segType) && (
               <div className="text-xs text-neutral-400 italic">Nur für Trail/Wintersport verfügbar</div>
             )}
          </div>
        </div>

        <Field label="Höhenprofil">
          <ClimbTypePills
            segType={meta.segType}
            value={meta.climbType ?? 'standard'}
            onChange={(v) => onMetaChange({ climbType: v })}
          />
        </Field>

        <Field label="Richtung">
          <ToggleRow
            checked={meta.bidirectional}
            onChange={handleBidirectionalToggle}
            label="In beide Richtungen matchen"
          />
        </Field>

        <Field label="Sichtbarkeit">
          <ToggleRow
            checked={meta.isPublic}
            onChange={e => onMetaChange({ isPublic: e.target.checked })}
            label="Öffentlich sichtbar"
          />
        </Field>
      </div>
    </aside>
  )
}

function MetricCell({ label, value, valueClassName }: { label: string; value: string; valueClassName?: string }) {
  return (
    <div>
      <div className="text-[10px] font-medium tracking-wide text-neutral-500 dark:text-neutral-400 uppercase">{label}</div>
      <div className={cn('mt-0.5 text-sm font-semibold', valueClassName ?? 'text-neutral-900 dark:text-neutral-100')}>{value}</div>
    </div>
  )
}

function Field({ label, children, required }: { label: string; children: ReactNode; required?: boolean }) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center gap-1.5 text-[11px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">
        <span>{label}</span>
        {required && <span className="text-red-500">*</span>}
      </div>
      {children}
    </div>
  )
}

function ToggleRow({ checked, onChange, label }: { checked: boolean; onChange: React.ChangeEventHandler<HTMLInputElement>; label: string }) {
  return (
    <label className="flex cursor-pointer items-center gap-2.5 text-sm text-neutral-700 dark:text-neutral-300 select-none hover:text-neutral-900 dark:hover:text-neutral-100 transition-colors">
      <input type="checkbox" checked={checked} onChange={onChange} className="h-4 w-4 rounded border-neutral-400 dark:border-neutral-600 accent-accent-DEFAULT" />
      {label}
    </label>
  )
}
