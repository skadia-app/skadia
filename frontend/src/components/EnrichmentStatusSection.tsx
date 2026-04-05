import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchEnrichmentStatus, fetchEnrichmentSettings, type EnrichmentModuleStatus } from '@/api/enrichment'
import { apiClient } from '@/api/client'
import { useToast } from '@/lib/toast'
import type { EnrichmentSettings } from '@/api/types'

// Maps module names (Go backend) to the corresponding API field in EnrichmentSettings.
// A module is globally disabled when the field is explicitly false (default: true/absent = enabled).
const MODULE_API_FIELD: Partial<Record<string, keyof EnrichmentSettings>> = {
  summits:     'enable_summit_detection',
  forests:     'enable_forest_detection',
  water:       'enable_water_detection',
  settlements: 'enable_settlement_detection',
  borders:     'enable_border_detection',
  stops:       'enable_stop_detection',
  lifts:       'enable_lift_detection',
  surfaces:    'enable_surface_detection',
  ascents:     'enable_ascent_detection',
  intervals:   'enable_interval_detection',
  embeddings:  'enable_embeddings',
}

function buildGloballyDisabled(settings: EnrichmentSettings | undefined): Set<string> {
  if (!settings) return new Set()
  const disabled = new Set<string>()
  for (const [moduleName, field] of Object.entries(MODULE_API_FIELD)) {
    if (field && settings[field] === false) {
      disabled.add(moduleName)
    }
  }
  return disabled
}

interface Props {
  activityId: string
  enrichmentStatus: string
  hiddenModules: Set<string>
  onToggleModule: (name: string) => void
}

const MODULE_META: Record<string, { label: string; icon: string }> = {
  summits:     { label: 'Gipfel & Pässe', icon: 'M3 17l4-8 4 4 4-4 4 8H3z' },
  lifts:       { label: 'Lifte',          icon: 'M4 4v16h2V4H4zm14 0v16h2V4h-2zM8 8l4-4 4 4M12 4v12' },
  settlements: { label: 'Siedlungen',     icon: 'M3 21h18M5 21V7l7-4 7 4v14M9 21v-6h6v6' },
  borders:     { label: 'Grenzen',        icon: 'M4 4v16M4 4h4v4H4M4 12h4v4H4M12 4h4v4h-4M12 12h4v4h-4M20 4v16' },
  water:       { label: 'Gewässer',       icon: 'M2 12c2-3 4-3 6 0s4 3 6 0 4-3 6 0M2 17c2-3 4-3 6 0s4 3 6 0 4-3 6 0' },
  forests:     { label: 'Wälder',         icon: 'M12 2L7 10h3l-4 6h4l-3 4h10l-3-4h4l-4-6h3L12 2z' },
  surfaces:    { label: 'Oberflächen',    icon: 'M2 20h20M4 20V10l4-4 4 6 4-8 4 6v10' },
  stops:       { label: 'Stopps',         icon: 'M12 2a7 7 0 00-7 7c0 5 7 13 7 13s7-8 7-13a7 7 0 00-7-7zm0 10a3 3 0 110-6 3 3 0 010 6z' },
  intervals:   { label: 'Intervalle',     icon: 'M4 18V6M8 18v-8M12 18V4M16 18v-6M20 18v-10' },
  ascents:     { label: 'Anstiege',       icon: 'M3 17l5-10 4 6 4-8 5 12' },
  embeddings:  { label: 'Embeddings',     icon: 'M12 2a10 10 0 100 20 10 10 0 000-20zm0 4a3 3 0 110 6 3 3 0 010-6z' },
}

export default function EnrichmentStatusSection({ activityId, enrichmentStatus, hiddenModules, onToggleModule }: Props) {
  const [collapsed, setCollapsed] = useState(true)
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  const { data: enrichmentConfig } = useQuery({
    queryKey: ['enrichment-settings'],
    queryFn: fetchEnrichmentSettings,
    staleTime: 60_000, // settings rarely change — avoid repeated fetches per activity
  })
  const globallyDisabled = buildGloballyDisabled(enrichmentConfig)

  const { data, isLoading } = useQuery({
    queryKey: ['enrichment-status', activityId],
    queryFn: () => fetchEnrichmentStatus(activityId),
    refetchInterval: enrichmentStatus === 'processing' ? 5000 : false,
  })

  const recomputeMutation = useMutation({
    mutationFn: (_moduleName?: string) =>
      apiClient(`/activities/${activityId}/recompute`, {
        method: 'POST',
        body: { scope: ['geo'] },
      }),
    onSuccess: (_data, _moduleName) => {
      queryClient.invalidateQueries({ queryKey: ['enrichment-status', activityId] })
      queryClient.invalidateQueries({ queryKey: ['activity', activityId] })
      queryClient.invalidateQueries({ queryKey: ['geo-tags', activityId] })
      queryClient.invalidateQueries({ queryKey: ['stops', activityId] })
      queryClient.invalidateQueries({ queryKey: ['surfaces', activityId] })
      queryClient.invalidateQueries({ queryKey: ['climbs', activityId] })
      addToast({ type: 'success', title: 'Geo-Enrichment gestartet' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler beim Starten', duration: 0 })
    },
  })

  const handleTileClick = (module: EnrichmentModuleStatus) => {
    if (globallyDisabled.has(module.name)) return // can't activate globally disabled modules
    if (hiddenModules.has(module.name)) {
      // Unhide: remove from hidden set (data already exists in DB)
      onToggleModule(module.name)
    } else if (module.status === 'completed') {
      // Hide from view
      onToggleModule(module.name)
    } else {
      // no_data or error: trigger recompute
      recomputeMutation.mutate(module.name)
    }
  }

  if (isLoading || !data) return null

  const modules = data.modules
  const errorCount = modules.filter(m => m.status === 'error').length
  const completedCount = modules.filter(m => m.status === 'completed' && !hiddenModules.has(m.name)).length

  return (
    <div>
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="flex items-center gap-2 mb-3 cursor-pointer group"
      >
        <svg
          className={`w-4 h-4 text-neutral-400 transition-transform ${collapsed ? '' : 'rotate-90'}`}
          fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 group-hover:text-accent-DEFAULT transition-colors">
          Enrichment-Module
        </h2>
        <span className="text-xs text-neutral-500 dark:text-neutral-400 ml-1">
          {completedCount}/{modules.length}
          {errorCount > 0 && (
            <span className="text-red-500 ml-1">({errorCount} Fehler)</span>
          )}
        </span>
      </button>

      {!collapsed && (
        <div className="space-y-3">
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-2">
            {modules.map(m => (
              <ModuleTile
                key={m.name}
                module={m}
                hidden={hiddenModules.has(m.name)}
                globallyDisabled={globallyDisabled.has(m.name)}
                computing={recomputeMutation.isPending}
                onClick={() => handleTileClick(m)}
              />
            ))}
          </div>

          <p className="text-[11px] text-neutral-400 dark:text-neutral-600">
            Aktive Module werden in der Detailansicht angezeigt. Inaktive Module können durch Klick neu berechnet werden.{' '}
            {globallyDisabled.size > 0 && (
              <Link to="/settings?tab=analysen" className="text-accent-DEFAULT hover:underline">
                {globallyDisabled.size === 1 ? '1 Modul ist' : `${globallyDisabled.size} Module sind`} global deaktiviert.
              </Link>
            )}
          </p>
        </div>
      )}
    </div>
  )
}

function ModuleTile({
  module,
  hidden,
  globallyDisabled,
  computing,
  onClick,
}: {
  module: EnrichmentModuleStatus
  hidden: boolean
  globallyDisabled: boolean
  computing: boolean
  onClick: () => void
}) {
  const meta = MODULE_META[module.name] ?? { label: module.name, icon: '' }

  const isCompleted = module.status === 'completed'
  const isError = module.status === 'error'
  const isPending = module.status === 'pending'

  let bg: string
  let ring: string
  let dotColor: string
  let titleColor: string
  let tooltip: string

  if (globallyDisabled) {
    bg = 'bg-neutral-50/50 dark:bg-neutral-900/10'
    ring = 'ring-neutral-100 dark:ring-neutral-800/30'
    dotColor = 'bg-neutral-200 dark:bg-neutral-700'
    titleColor = 'text-neutral-350 dark:text-neutral-600 opacity-50'
    tooltip = 'Global deaktiviert — unter Einstellungen → Analysen aktivieren'
  } else if (hidden) {
    bg = 'bg-neutral-50 dark:bg-neutral-900/20'
    ring = 'ring-neutral-150 dark:ring-neutral-800/50'
    dotColor = 'bg-neutral-200 dark:bg-neutral-700'
    titleColor = 'text-neutral-400 dark:text-neutral-600 line-through'
    tooltip = 'Ausgeblendet — klicken zum Einblenden'
  } else if (isCompleted) {
    bg = 'bg-emerald-50 dark:bg-emerald-950/30'
    ring = 'ring-emerald-200 dark:ring-emerald-800'
    dotColor = 'bg-emerald-500'
    titleColor = 'text-neutral-700 dark:text-neutral-300'
    tooltip = 'Daten vorhanden — klicken zum Ausblenden'
  } else if (isError) {
    bg = 'bg-red-50 dark:bg-red-950/30'
    ring = 'ring-red-200 dark:ring-red-800'
    dotColor = 'bg-red-500'
    titleColor = 'text-neutral-700 dark:text-neutral-300'
    tooltip = module.error ?? 'Fehler — klicken zum Neu berechnen'
  } else if (isPending) {
    bg = 'bg-amber-50 dark:bg-amber-950/30'
    ring = 'ring-amber-200 dark:ring-amber-800'
    dotColor = 'bg-amber-500 animate-pulse'
    titleColor = 'text-neutral-700 dark:text-neutral-300'
    tooltip = 'Ausstehend'
  } else {
    bg = 'bg-neutral-50 dark:bg-neutral-900/30'
    ring = 'ring-neutral-200 dark:ring-neutral-800'
    dotColor = 'bg-neutral-300 dark:bg-neutral-600'
    titleColor = 'text-neutral-500 dark:text-neutral-500'
    tooltip = 'Keine Daten — klicken zum Berechnen'
  }

  const canClick = !computing && !isPending && !globallyDisabled
  const cursorClass = canClick
    ? 'cursor-pointer hover:brightness-95 dark:hover:brightness-110 active:scale-[0.98]'
    : 'cursor-default'

  return (
    <button
      type="button"
      disabled={!canClick}
      onClick={canClick ? onClick : undefined}
      title={tooltip}
      className={`relative flex items-center gap-2 px-3 py-2 rounded-lg ring-1 text-left transition-all ${bg} ${ring} ${cursorClass} disabled:cursor-default`}
    >
      <span className={`absolute top-1.5 right-1.5 w-2 h-2 rounded-full shrink-0 ${dotColor}`} />

      <svg
        className={`w-4 h-4 shrink-0 text-neutral-500 dark:text-neutral-400 ${globallyDisabled ? 'opacity-40' : ''}`}
        fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}
      >
        <path strokeLinecap="round" strokeLinejoin="round" d={meta.icon} />
      </svg>

      <div className="min-w-0 flex-1">
        <div className={`text-xs font-medium truncate transition-colors ${titleColor}`}>
          {meta.label}
        </div>
        {!hidden && !globallyDisabled && isCompleted && module.count > 0 && (
          <div className="text-[10px] text-neutral-500 dark:text-neutral-500">
            {module.count} Treffer
          </div>
        )}
        {!hidden && !globallyDisabled && isError && (
          <div className="text-[10px] text-red-500 truncate" title={module.error}>
            {module.error}
          </div>
        )}
        {globallyDisabled && (
          <div className="text-[10px] text-neutral-400 dark:text-neutral-600">
            global aus
          </div>
        )}
      </div>
    </button>
  )
}
