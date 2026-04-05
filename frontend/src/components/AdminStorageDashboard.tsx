import { useState, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '@/api/client'
import { useToast } from '@/lib/toast'
import { fmtBytes } from '@/lib/format'
import { FILE_TYPE_LABELS } from '@/api/shadowfs'
import {
  HardDrive,
  Cloud,
  AlertTriangle,
  RefreshCw,
  ChevronUp,
  ChevronDown,
  ChevronsUpDown,
  Archive,
  Layers,
} from 'lucide-react'

// ── i18n-ready labels ────────────────────────────────────────────────────────

const LABELS = {
  title: 'Speicher-Übersicht',
  backend: 'Backend',
  basePath: 'Basispfad',
  keepVersions: 'Versionen behalten',
  keepVersionsValues: {
    '-1': 'Alle behalten',
    '0': 'Löschen',
    '1': 'Letzte behalten',
  } satisfies Record<string, string>,
  syncEnabled: 'Sync aktiv',
  yes: 'Ja',
  no: 'Nein',
  backendLocal: 'Lokal',
  backendS3: 'S3',
  usersTable: {
    heading: 'Speicherverbrauch pro Benutzer',
    libraryDir: 'Verzeichnis',
    diskUsage: 'Aktiv',
    fileCount: 'Dateien',
    archiveSize: 'Archiv',
  },
  typeCounts: {
    heading: 'Dateien nach Typ',
    noData: 'Keine Daten',
  },
  archiveWarning: 'Archiv > 1 GB',
  actions: {
    heading: 'Verwaltungsaktionen',
    reconcile: 'Abgleich starten',
    reconcileDesc: 'Prüft Konsistenz zwischen Dateisystem und user_files Registry',
    reconcileStarted: 'Abgleich gestartet',
    reconcileRunning: 'Abgleich läuft bereits',
    reconcileFailed: 'Abgleich fehlgeschlagen',
    backfillMeta: 'Backup-Metadaten erstellen',
    backfillMetaDesc: 'Erstellt .meta Backup-Dateien für alle Aktivitäten und Foto-Positionen (für Restore ohne Datenbank).',
    backfillMetaStarted: 'Backup-Metadaten werden erstellt',
    backfillMetaRunning: 'Backfill läuft bereits',
    backfillMetaFailed: 'Backfill fehlgeschlagen',
    sortBy: 'Sortieren nach',
  },
  loading: 'Lade Speicher-Statistiken...',
  error: 'Statistiken konnten nicht geladen werden.',
  refresh: 'Aktualisieren',
  noUsers: 'Keine Benutzer gefunden',
  pendingLabel: 'Läuft...',
}

// ── Types ────────────────────────────────────────────────────────────────────

interface StorageUserRow {
  user_id: string
  library_dir: string
  disk_usage_bytes: number
  file_count: number
  archive_size_bytes: number
}

interface StorageDashboardData {
  backend: string
  base_path: string
  keep_versions: number
  sync_enabled: boolean
  users: StorageUserRow[]
  type_counts: Record<string, number>
}

interface ActionResponse {
  status: string
  job_id?: string
}

type SortKey = 'library_dir' | 'disk_usage_bytes' | 'file_count' | 'archive_size_bytes'
type SortDir = 'asc' | 'desc'

// ── Helpers ──────────────────────────────────────────────────────────────────

const GB = 1024 * 1024 * 1024

// ── Sub-components ───────────────────────────────────────────────────────────

function BackendBadge({ backend }: { backend: string }) {
  const isS3 = backend === 's3'
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-sm font-medium ${
        isS3
          ? 'bg-blue-500/10 text-blue-600 dark:text-blue-400'
          : 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
      }`}
    >
      {isS3 ? <Cloud className="w-3.5 h-3.5" /> : <HardDrive className="w-3.5 h-3.5" />}
      {isS3 ? LABELS.backendS3 : LABELS.backendLocal}
    </span>
  )
}

function SortIcon({ col, sortKey, sortDir }: { col: SortKey; sortKey: SortKey; sortDir: SortDir }) {
  if (col !== sortKey) return <ChevronsUpDown className="w-3.5 h-3.5 text-neutral-400" />
  return sortDir === 'asc'
    ? <ChevronUp className="w-3.5 h-3.5 text-accent-DEFAULT" />
    : <ChevronDown className="w-3.5 h-3.5 text-accent-DEFAULT" />
}

function TypeCountCard({ type, count, maxCount }: { type: string; count: number; maxCount: number }) {
  const pct = maxCount > 0 ? (count / maxCount) * 100 : 0
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center justify-between text-sm">
        <span className="text-neutral-700 dark:text-neutral-300">
          {FILE_TYPE_LABELS[type] ?? type}
        </span>
        <span className="tabular-nums font-medium text-neutral-900 dark:text-neutral-100">
          {/* TODO(i18n): replace 'de-DE' with user locale */}
          {count.toLocaleString('de-DE')}
        </span>
      </div>
      <div className="h-2 rounded-full bg-neutral-100 dark:bg-neutral-700 overflow-hidden">
        <div
          className="h-full rounded-full bg-accent-DEFAULT transition-all duration-300"
          style={{ width: `${pct}%` }}
          role="progressbar"
          aria-valuenow={count}
          aria-valuemin={0}
          aria-valuemax={maxCount}
          aria-label={FILE_TYPE_LABELS[type] ?? type}
        />
      </div>
    </div>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

export default function AdminStorageDashboard() {
  const { addToast } = useToast()
  const queryClient = useQueryClient()
  const [sortKey, setSortKey] = useState<SortKey>('disk_usage_bytes')
  const [sortDir, setSortDir] = useState<SortDir>('desc')

  const { data, isLoading, isError, refetch, isFetching } = useQuery<StorageDashboardData>({
    queryKey: ['admin-storage-dashboard'],
    queryFn: () => apiClient<StorageDashboardData>('/admin/storage'),
    staleTime: 30_000,
  })

  const reconcileMut = useMutation<ActionResponse, Error>({
    mutationFn: () => apiClient<ActionResponse>('/admin/storage/reconcile', { method: 'POST' }),
    onSuccess: (res) => {
      if (res.status === 'already_running') {
        addToast({ type: 'warning', title: LABELS.actions.reconcileRunning })
        return
      }
      queryClient.invalidateQueries({ queryKey: ['admin-storage-dashboard'] })
      addToast({ type: 'success', title: LABELS.actions.reconcileStarted, message: `Job: ${res.job_id ?? '—'}` })
    },
    onError: (err) => {
      addToast({ type: 'error', title: LABELS.actions.reconcileFailed, message: err.message, duration: 0 })
    },
  })

  const backfillMetaMut = useMutation<ActionResponse, Error>({
    mutationFn: () => apiClient<ActionResponse>('/admin/storage/backfill-meta', { method: 'POST' }),
    onSuccess: (res) => {
      if (res.status === 'already_running') {
        addToast({ type: 'warning', title: LABELS.actions.backfillMetaRunning })
        return
      }
      addToast({ type: 'success', title: LABELS.actions.backfillMetaStarted, message: `Job: ${res.job_id ?? '—'}` })
    },
    onError: (err) => {
      addToast({ type: 'error', title: LABELS.actions.backfillMetaFailed, message: err.message, duration: 0 })
    },
  })

  const anyJobRunning = reconcileMut.isPending || backfillMetaMut.isPending

  function toggleSort(col: SortKey) {
    if (sortKey === col) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortKey(col)
      setSortDir('desc')
    }
  }

  // Sort users
  const sortedUsers = useMemo(() => {
    if (!data) return []
    return [...data.users].sort((a, b) => {
      const valA = sortKey === 'library_dir' ? a.library_dir : (a[sortKey] as number)
      const valB = sortKey === 'library_dir' ? b.library_dir : (b[sortKey] as number)
      if (typeof valA === 'string' && typeof valB === 'string') {
        return sortDir === 'asc' ? valA.localeCompare(valB) : valB.localeCompare(valA)
      }
      return sortDir === 'asc' ? (valA as number) - (valB as number) : (valB as number) - (valA as number)
    })
  }, [data, sortKey, sortDir])

  // Max count for bar charts
  const { typeEntries, maxCount } = useMemo(() => {
    const entries = data ? Object.entries(data.type_counts).sort((a, b) => b[1] - a[1]) : []
    return { typeEntries: entries, maxCount: entries.length > 0 ? Math.max(...entries.map(([, c]) => c)) : 1 }
  }, [data])

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-neutral-400 py-8">
        <RefreshCw className="w-4 h-4 animate-spin" />
        {LABELS.loading}
      </div>
    )
  }

  if (isError || !data) {
    return (
      <div className="rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 px-4 py-3 text-sm text-red-600 dark:text-red-400">
        {LABELS.error}
      </div>
    )
  }

  const keepVersionsLabel =
    LABELS.keepVersionsValues[String(data.keep_versions)] ?? String(data.keep_versions)

  return (
    <div className="space-y-6">

      {/* Header */}
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">
          {LABELS.title}
        </h2>
        <button
          onClick={() => refetch()}
          disabled={isFetching}
          className="flex items-center gap-1.5 text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 transition-colors cursor-pointer"
          aria-label={LABELS.refresh}
        >
          <RefreshCw className={`w-3.5 h-3.5 ${isFetching ? 'animate-spin' : ''}`} />
          {LABELS.refresh}
        </button>
      </div>

      {/* Config strip */}
      <div className="flex flex-wrap gap-4 rounded-xl border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-800/40 px-4 py-3 text-sm">
        <div className="flex items-center gap-2">
          <span className="text-neutral-500 dark:text-neutral-400">{LABELS.backend}:</span>
          <BackendBadge backend={data.backend} />
        </div>
        {data.base_path && (
          <div className="flex items-center gap-2">
            <span className="text-neutral-500 dark:text-neutral-400">{LABELS.basePath}:</span>
            <code className="font-mono text-xs bg-neutral-100 dark:bg-neutral-700 px-2 py-0.5 rounded text-neutral-800 dark:text-neutral-200">
              {data.base_path}
            </code>
          </div>
        )}
        <div className="flex items-center gap-2">
          <span className="text-neutral-500 dark:text-neutral-400">{LABELS.keepVersions}:</span>
          <span className="text-neutral-800 dark:text-neutral-200">{keepVersionsLabel}</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-neutral-500 dark:text-neutral-400">{LABELS.syncEnabled}:</span>
          <span className={data.sync_enabled ? 'text-emerald-600 dark:text-emerald-400' : 'text-neutral-500'}>
            {data.sync_enabled ? LABELS.yes : LABELS.no}
          </span>
        </div>
      </div>

      {/* Two-column grid: users table + type counts */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">

        {/* Users table */}
        <section className="lg:col-span-2 rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
          <div className="px-4 py-3 border-b border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-800/40">
            <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
              {LABELS.usersTable.heading}
            </h3>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm" aria-describedby="storage-users-caption">
              <caption id="storage-users-caption" className="sr-only">
                {LABELS.usersTable.heading}
              </caption>
              <thead>
                <tr className="text-left text-xs text-neutral-500 dark:text-neutral-400 border-b border-neutral-100 dark:border-neutral-800">
                  <th
                    scope="col"
                    className="px-4 py-2 font-medium"
                    aria-sort={sortKey === 'library_dir' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}
                  >
                    <button
                      className="flex items-center gap-1 cursor-pointer hover:text-neutral-700 dark:hover:text-neutral-200"
                      onClick={() => toggleSort('library_dir')}
                      aria-label={`${LABELS.actions.sortBy} ${LABELS.usersTable.libraryDir}`}
                    >
                      {LABELS.usersTable.libraryDir}
                      <SortIcon col="library_dir" sortKey={sortKey} sortDir={sortDir} />
                    </button>
                  </th>
                  <th
                    scope="col"
                    className="px-4 py-2 font-medium text-right"
                    aria-sort={sortKey === 'disk_usage_bytes' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}
                  >
                    <button
                      className="flex items-center gap-1 cursor-pointer hover:text-neutral-700 dark:hover:text-neutral-200 ml-auto"
                      onClick={() => toggleSort('disk_usage_bytes')}
                      aria-label={`${LABELS.actions.sortBy} ${LABELS.usersTable.diskUsage}`}
                    >
                      {LABELS.usersTable.diskUsage}
                      <SortIcon col="disk_usage_bytes" sortKey={sortKey} sortDir={sortDir} />
                    </button>
                  </th>
                  <th
                    scope="col"
                    className="px-4 py-2 font-medium text-right"
                    aria-sort={sortKey === 'file_count' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}
                  >
                    <button
                      className="flex items-center gap-1 cursor-pointer hover:text-neutral-700 dark:hover:text-neutral-200 ml-auto"
                      onClick={() => toggleSort('file_count')}
                      aria-label={`${LABELS.actions.sortBy} ${LABELS.usersTable.fileCount}`}
                    >
                      {LABELS.usersTable.fileCount}
                      <SortIcon col="file_count" sortKey={sortKey} sortDir={sortDir} />
                    </button>
                  </th>
                  <th
                    scope="col"
                    className="px-4 py-2 font-medium text-right"
                    aria-sort={sortKey === 'archive_size_bytes' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}
                  >
                    <button
                      className="flex items-center gap-1 cursor-pointer hover:text-neutral-700 dark:hover:text-neutral-200 ml-auto"
                      onClick={() => toggleSort('archive_size_bytes')}
                      aria-label={`${LABELS.actions.sortBy} ${LABELS.usersTable.archiveSize}`}
                    >
                      <Archive className="w-3 h-3" />
                      {LABELS.usersTable.archiveSize}
                      <SortIcon col="archive_size_bytes" sortKey={sortKey} sortDir={sortDir} />
                    </button>
                  </th>
                </tr>
              </thead>
              <tbody>
                {sortedUsers.map((user) => {
                  const overLimit = user.archive_size_bytes > GB
                  return (
                    <tr
                      key={user.user_id}
                      className="border-b border-neutral-100 dark:border-neutral-800 last:border-0 hover:bg-neutral-50 dark:hover:bg-neutral-800/30 transition-colors"
                    >
                      <td className="px-4 py-2.5 font-mono text-xs text-neutral-700 dark:text-neutral-300 max-w-[200px] truncate">
                        {user.library_dir}
                      </td>
                      <td className="px-4 py-2.5 text-right tabular-nums text-neutral-700 dark:text-neutral-300">
                        {fmtBytes(user.disk_usage_bytes)}
                      </td>
                      <td className="px-4 py-2.5 text-right tabular-nums text-neutral-700 dark:text-neutral-300">
                        {/* TODO(i18n): replace 'de-DE' with user locale */}
                        {user.file_count.toLocaleString('de-DE')}
                      </td>
                      <td className="px-4 py-2.5 text-right">
                        <span className={`inline-flex items-center gap-1 tabular-nums ${
                          overLimit
                            ? 'text-amber-600 dark:text-amber-400 font-medium'
                            : 'text-neutral-700 dark:text-neutral-300'
                        }`}>
                          {overLimit && <AlertTriangle className="w-3.5 h-3.5 shrink-0" role="img" aria-label={LABELS.archiveWarning} />}
                          {fmtBytes(user.archive_size_bytes)}
                        </span>
                      </td>
                    </tr>
                  )
                })}
                {sortedUsers.length === 0 && (
                  <tr>
                    <td colSpan={4} className="px-4 py-6 text-center text-sm text-neutral-400">
                      {LABELS.noUsers}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </section>

        {/* Type counts */}
        <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
          <div className="px-4 py-3 border-b border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-800/40 flex items-center gap-2">
            <Layers className="w-4 h-4 text-neutral-500" />
            <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
              {LABELS.typeCounts.heading}
            </h3>
          </div>
          <div className="px-4 py-4 space-y-3">
            {typeEntries.length === 0 ? (
              <p className="text-sm text-neutral-400">{LABELS.typeCounts.noData}</p>
            ) : (
              typeEntries.map(([type, count]) => (
                <TypeCountCard key={type} type={type} count={count} maxCount={maxCount} />
              ))
            )}
          </div>
        </section>

      </div>

      {/* Actions */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-4">
        <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
          {LABELS.actions.heading}
        </h3>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">

          <div className="rounded-lg border border-neutral-100 dark:border-neutral-700 p-4 space-y-3">
            <div>
              <p className="text-sm font-medium text-neutral-800 dark:text-neutral-200">
                {LABELS.actions.reconcile}
              </p>
              <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
                {LABELS.actions.reconcileDesc}
              </p>
            </div>
            <button
              onClick={() => reconcileMut.mutate()}
              disabled={anyJobRunning}
              className="w-full px-3 py-1.5 rounded-lg text-sm font-medium border border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors disabled:opacity-50 cursor-pointer disabled:cursor-not-allowed"
            >
              {reconcileMut.isPending ? LABELS.pendingLabel : LABELS.actions.reconcile}
            </button>
          </div>

          <div className="rounded-lg border border-neutral-100 dark:border-neutral-700 p-4 space-y-3">
            <div>
              <p className="text-sm font-medium text-neutral-800 dark:text-neutral-200">
                {LABELS.actions.backfillMeta}
              </p>
              <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
                {LABELS.actions.backfillMetaDesc}
              </p>
            </div>
            <button
              onClick={() => backfillMetaMut.mutate()}
              disabled={anyJobRunning}
              className="w-full px-3 py-1.5 rounded-lg text-sm font-medium border border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors disabled:opacity-50 cursor-pointer disabled:cursor-not-allowed"
            >
              {backfillMetaMut.isPending ? LABELS.pendingLabel : LABELS.actions.backfillMeta}
            </button>
          </div>

        </div>
      </section>

    </div>
  )
}
