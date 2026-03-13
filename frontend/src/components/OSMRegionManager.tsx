import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Trash2, Database, XCircle, ChevronDown } from 'lucide-react'
import { useState } from 'react'
import {
  fetchOSMRegionStatus,
  fetchOSMDiskUsage,
  deleteOSMRegion,
  cancelOSMImport,
} from '@/api/osm'
import { useToast } from '@/lib/toast'

function formatBytes(bytes: number | null): string {
  if (!bytes) return '—'
  if (bytes > 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`
  return `${(bytes / 1024 / 1024).toFixed(0)} MB`
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('de-AT', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
  })
}

export function OSMRegionManager() {
  const qc = useQueryClient()
  const { addToast } = useToast()
  const [diskExpanded, setDiskExpanded] = useState(false)

  const { data: regionData } = useQuery({
    queryKey: ['osm-regions-status'],
    queryFn: fetchOSMRegionStatus,
    refetchInterval: (query) =>
      query.state.data?.regions?.some((r) => r.status === 'importing') ? 5000 : false,
  })

  const { data: diskData } = useQuery({
    queryKey: ['osm-disk-usage'],
    queryFn: fetchOSMDiskUsage,
    refetchInterval: 30000,
  })

  const deleteMutation = useMutation({
    mutationFn: deleteOSMRegion,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['osm-regions-status'] })
      addToast({ type: 'success', title: 'Region entfernt' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler beim Entfernen der Region', duration: 0 })
    },
  })

  const cancelMutation = useMutation({
    mutationFn: cancelOSMImport,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['osm-regions-status'] })
      addToast({ type: 'info', title: 'Import abgebrochen' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler beim Abbrechen des Imports', duration: 0 })
    },
  })

  const regions = regionData?.regions ?? []

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
          Importierte Regionen
        </h3>
        {diskData && (
          <button
            onClick={() => setDiskExpanded(!diskExpanded)}
            className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
          >
            <Database className="w-3 h-3" />
            {formatBytes(diskData.total_bytes)} OSM-Daten
            <ChevronDown
              className={`w-3 h-3 transition-transform ${diskExpanded ? 'rotate-180' : ''}`}
            />
          </button>
        )}
      </div>

      {diskExpanded && diskData && (
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-3 text-xs space-y-1 bg-neutral-50 dark:bg-neutral-800/30">
          {diskData.tables.map((t) => (
            <div key={t.tablename} className="flex justify-between">
              <span className="text-neutral-500 dark:text-neutral-400 font-mono">{t.tablename}</span>
              <span className="text-neutral-700 dark:text-neutral-300">{formatBytes(t.total_bytes)}</span>
            </div>
          ))}
          <div className="flex justify-between border-t border-neutral-200 dark:border-neutral-700 pt-1 mt-1 font-medium">
            <span className="text-neutral-600 dark:text-neutral-300">Gesamt</span>
            <span className="text-neutral-800 dark:text-neutral-200">{formatBytes(diskData.total_bytes)}</span>
          </div>
        </div>
      )}

      {regions.length === 0 ? (
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          Noch keine OSM-Regionen importiert.
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-200 dark:border-neutral-700 text-neutral-500 dark:text-neutral-400 text-xs">
                <th className="text-left pb-2 pr-3 font-medium">Region</th>
                <th className="text-left pb-2 pr-3 font-medium">Kontinent</th>
                <th className="text-left pb-2 pr-3 font-medium">Importiert</th>
                <th className="text-left pb-2 pr-3 font-medium">Kategorien</th>
                <th className="text-left pb-2 pr-3 font-medium">Größe</th>
                <th className="text-left pb-2 pr-3 font-medium">Status</th>
                <th className="text-right pb-2 font-medium">Aktionen</th>
              </tr>
            </thead>
            <tbody>
              {regions.map((reg) => (
                <tr
                  key={reg.region_id}
                  className="border-b border-neutral-100 dark:border-neutral-800 last:border-0"
                >
                  <td className="py-2 pr-3 font-medium text-neutral-900 dark:text-neutral-100">
                    {reg.label}
                  </td>
                  <td className="py-2 pr-3 text-neutral-500 dark:text-neutral-400">
                    {reg.continent}
                  </td>
                  <td className="py-2 pr-3 text-neutral-500 dark:text-neutral-400">
                    {formatDate(reg.imported_at)}
                  </td>
                  <td className="py-2 pr-3 text-neutral-500 dark:text-neutral-400 text-xs">
                    {reg.categories.join(', ')}
                  </td>
                  <td className="py-2 pr-3 text-neutral-500 dark:text-neutral-400">
                    {formatBytes(reg.pbf_size_bytes)}
                  </td>
                  <td className="py-2 pr-3">
                    <span
                      className={`text-xs px-2 py-0.5 rounded-full font-medium ${
                        reg.status === 'imported'
                          ? 'bg-green-500/10 text-green-600 dark:text-green-400'
                          : reg.status === 'importing'
                            ? 'bg-yellow-500/10 text-yellow-600 dark:text-yellow-400'
                            : 'bg-red-500/10 text-red-600 dark:text-red-400'
                      }`}
                    >
                      {reg.status === 'imported'
                        ? 'Importiert'
                        : reg.status === 'importing'
                          ? 'Wird importiert…'
                          : 'Fehler'}
                    </span>
                  </td>
                  <td className="py-2 text-right">
                    <div className="flex justify-end gap-1">
                      {reg.status === 'importing' && (
                        <button
                          onClick={() => cancelMutation.mutate(reg.region_id)}
                          disabled={cancelMutation.isPending}
                          className="p-1 text-neutral-400 hover:text-red-500 dark:hover:text-red-400 transition-colors disabled:opacity-40"
                          title="Import abbrechen"
                        >
                          <XCircle className="w-4 h-4" />
                        </button>
                      )}
                      <button
                        onClick={() => {
                          if (confirm(`Region "${reg.label}" aus Tracking entfernen?\n\nDie importierten Daten werden aus der Datenbank gelöscht.`)) {
                            deleteMutation.mutate(reg.region_id)
                          }
                        }}
                        disabled={deleteMutation.isPending}
                        className="p-1 text-neutral-400 hover:text-red-500 dark:hover:text-red-400 transition-colors disabled:opacity-40"
                        title="Aus Tracking entfernen"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
