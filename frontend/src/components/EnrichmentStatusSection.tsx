import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchEnrichmentStatus, type EnrichmentModuleStatus } from '@/api/enrichment'
import { apiClient } from '@/api/client'
import { useToast } from '@/lib/toast'

interface Props {
  activityId: string
  enrichmentStatus: string
}

const MODULE_META: Record<string, { label: string; icon: string }> = {
  peaks:       { label: 'Gipfel & Pässe', icon: 'M3 17l4-8 4 4 4-4 4 8H3z' },
  lifts:       { label: 'Lifte',          icon: 'M4 4v16h2V4H4zm14 0v16h2V4h-2zM8 8l4-4 4 4M12 4v12' },
  settlements: { label: 'Siedlungen',     icon: 'M3 21h18M5 21V7l7-4 7 4v14M9 21v-6h6v6' },
  commute:     { label: 'Pendeln',        icon: 'M8 6v6l4-3-4-3zM16 6v6l-4-3 4-3zM3 18h18' },
  borders:     { label: 'Grenzen',        icon: 'M4 4v16M4 4h4v4H4M4 12h4v4H4M12 4h4v4h-4M12 12h4v4h-4M20 4v16' },
  water:       { label: 'Gewässer',       icon: 'M2 12c2-3 4-3 6 0s4 3 6 0 4-3 6 0M2 17c2-3 4-3 6 0s4 3 6 0 4-3 6 0' },
  forests:     { label: 'Wälder',         icon: 'M12 2L7 10h3l-4 6h4l-3 4h10l-3-4h4l-4-6h3L12 2z' },
  surfaces:    { label: 'Oberflächen',    icon: 'M2 20h20M4 20V10l4-4 4 6 4-8 4 6v10' },
  stops:       { label: 'Stopps',         icon: 'M12 2a7 7 0 00-7 7c0 5 7 13 7 13s7-8 7-13a7 7 0 00-7-7zm0 10a3 3 0 110-6 3 3 0 010 6z' },
  intervals:   { label: 'Intervalle',     icon: 'M4 18V6M8 18v-8M12 18V4M16 18v-6M20 18v-10' },
  ascents:     { label: 'Anstiege',       icon: 'M3 17l5-10 4 6 4-8 5 12' },
  embeddings:  { label: 'Embeddings',     icon: 'M12 2a10 10 0 100 20 10 10 0 000-20zm0 4a3 3 0 110 6 3 3 0 010-6z' },
}

const STATUS_STYLES: Record<string, { bg: string; ring: string; dot: string; label: string }> = {
  completed: {
    bg: 'bg-emerald-50 dark:bg-emerald-950/30',
    ring: 'ring-emerald-200 dark:ring-emerald-800',
    dot: 'bg-emerald-500',
    label: 'Daten vorhanden',
  },
  error: {
    bg: 'bg-red-50 dark:bg-red-950/30',
    ring: 'ring-red-200 dark:ring-red-800',
    dot: 'bg-red-500',
    label: 'Fehler',
  },
  no_data: {
    bg: 'bg-neutral-50 dark:bg-neutral-900/30',
    ring: 'ring-neutral-200 dark:ring-neutral-800',
    dot: 'bg-neutral-400',
    label: 'Keine Daten',
  },
  pending: {
    bg: 'bg-amber-50 dark:bg-amber-950/30',
    ring: 'ring-amber-200 dark:ring-amber-800',
    dot: 'bg-amber-500 animate-pulse',
    label: 'Ausstehend',
  },
}

export default function EnrichmentStatusSection({ activityId, enrichmentStatus }: Props) {
  const [collapsed, setCollapsed] = useState(true)
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  const { data, isLoading } = useQuery({
    queryKey: ['enrichment-status', activityId],
    queryFn: () => fetchEnrichmentStatus(activityId),
    enabled: enrichmentStatus !== 'pending',
    refetchInterval: enrichmentStatus === 'processing' ? 5000 : false,
  })

  const recomputeMutation = useMutation({
    mutationFn: () =>
      apiClient(`/activities/${activityId}/recompute`, {
        method: 'POST',
        body: { scope: ['geo'] },
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['enrichment-status', activityId] })
      queryClient.invalidateQueries({ queryKey: ['activity', activityId] })
      addToast({ type: 'success', title: 'Geo-Enrichment gestartet' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler beim Starten', duration: 0 })
    },
  })

  if (enrichmentStatus === 'pending') return null
  if (isLoading || !data) return null

  const modules = data.modules
  const errorCount = modules.filter(m => m.status === 'error').length
  const completedCount = modules.filter(m => m.status === 'completed').length

  return (
    <div>
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="flex items-center gap-2 mb-3 group"
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
              <ModuleTile key={m.name} module={m} />
            ))}
          </div>

          {errorCount > 0 && (
            <button
              onClick={() => recomputeMutation.mutate()}
              disabled={recomputeMutation.isPending}
              className="text-sm px-3 py-1.5 rounded-md bg-accent-DEFAULT/10 text-accent-DEFAULT
                         hover:bg-accent-DEFAULT/20 transition-colors disabled:opacity-50"
            >
              {recomputeMutation.isPending ? 'Wird gestartet...' : 'Geo-Enrichment erneut ausführen'}
            </button>
          )}
        </div>
      )}
    </div>
  )
}

function ModuleTile({ module }: { module: EnrichmentModuleStatus }) {
  const meta = MODULE_META[module.name] ?? { label: module.name, icon: '' }
  const style = STATUS_STYLES[module.status] ?? STATUS_STYLES.no_data

  return (
    <div
      className={`relative flex items-center gap-2 px-3 py-2 rounded-lg ring-1 ${style.bg} ${style.ring}`}
      title={module.status === 'error' ? module.error : style.label}
    >
      {/* Status dot */}
      <span className={`absolute top-1.5 right-1.5 w-2 h-2 rounded-full ${style.dot}`} />

      {/* Module icon */}
      <svg className="w-4 h-4 shrink-0 text-neutral-500 dark:text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
        <path strokeLinecap="round" strokeLinejoin="round" d={meta.icon} />
      </svg>

      <div className="min-w-0 flex-1">
        <div className="text-xs font-medium text-neutral-700 dark:text-neutral-300 truncate">
          {meta.label}
        </div>
        {module.status === 'completed' && module.count > 0 && (
          <div className="text-[10px] text-neutral-500 dark:text-neutral-500">
            {module.count} Treffer
          </div>
        )}
        {module.status === 'error' && (
          <div className="text-[10px] text-red-500 truncate" title={module.error}>
            {module.error}
          </div>
        )}
      </div>
    </div>
  )
}
