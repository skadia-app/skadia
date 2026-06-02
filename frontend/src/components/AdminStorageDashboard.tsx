import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { AdminStorageLimitsSection } from '@/pages/settings/AdminStorageLimitsSection'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, apiClient } from '@/api/client'
import { useToast } from '@/lib/toast'
import { fmtBytes } from '@/lib/format'
import { isSupportedLanguage, localeForLanguage } from '@/i18n/languages'
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

// ── Types ────────────────────────────────────────────────────────────────────

interface StorageUserRow {
  user_id: string
  username: string
  email: string
  library_dir: string
  disk_usage_bytes: number
  file_count: number
  total_files: number
  archive_size_bytes: number
  deleted_at?: string | null
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

type SortKey = 'library_dir' | 'disk_usage_bytes' | 'total_files' | 'archive_size_bytes'
type SortDir = 'asc' | 'desc'

// ── Helpers ──────────────────────────────────────────────────────────────────

const GB = 1024 * 1024 * 1024

function structuredErrorCode(err: unknown): string | undefined {
  if (err instanceof ApiError) {
    if (typeof err.json?.error_code === 'string') return err.json.error_code
    if (typeof err.json?.error === 'string') return err.json.error
  }
  if (err instanceof Error && !err.message.startsWith('API error ')) return err.message
  return undefined
}

// ── Sub-components ───────────────────────────────────────────────────────────

function BackendBadge({ backend, label }: { backend: string; label: string }) {
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
      {label}
    </span>
  )
}

function SortIcon({ col, sortKey, sortDir }: { col: SortKey; sortKey: SortKey; sortDir: SortDir }) {
  if (col !== sortKey) return <ChevronsUpDown className="w-3.5 h-3.5 text-neutral-400" />
  return sortDir === 'asc'
    ? <ChevronUp className="w-3.5 h-3.5 text-accent-DEFAULT" />
    : <ChevronDown className="w-3.5 h-3.5 text-accent-DEFAULT" />
}

function TypeCountCard({
  count,
  maxCount,
  label,
  locale,
}: {
  count: number
  maxCount: number
  label: string
  locale: string
}) {
  const pct = maxCount > 0 ? (count / maxCount) * 100 : 0
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center justify-between text-sm">
        <span className="text-neutral-700 dark:text-neutral-300">
          {label}
        </span>
        <span className="tabular-nums font-medium text-neutral-900 dark:text-neutral-100">
          {count.toLocaleString(locale)}
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
          aria-label={label}
        />
      </div>
    </div>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

export default function AdminStorageDashboard() {
  const { t, i18n } = useTranslation()
  const { addToast } = useToast()
  const queryClient = useQueryClient()
  const [sortKey, setSortKey] = useState<SortKey>('disk_usage_bytes')
  const [sortDir, setSortDir] = useState<SortDir>('desc')
  const activeLanguage = isSupportedLanguage(i18n.language) ? i18n.language : 'de'
  const activeLocale = localeForLanguage(activeLanguage)

  const { data, isLoading, isError, refetch, isFetching } = useQuery<StorageDashboardData>({
    queryKey: ['admin-storage-dashboard'],
    queryFn: () => apiClient<StorageDashboardData>('/admin/storage'),
    staleTime: 30_000,
  })

  const reconcileMut = useMutation<ActionResponse, Error>({
    mutationFn: () => apiClient<ActionResponse>('/admin/storage/reconcile', { method: 'POST' }),
    onSuccess: (res) => {
      if (res.status === 'already_running') {
        addToast({ type: 'warning', title: t('admin.storage.actions.reconcileRunning') })
        return
      }
      queryClient.invalidateQueries({ queryKey: ['admin-storage-dashboard'] })
      addToast({
        type: 'success',
        title: t('admin.storage.actions.reconcileStarted'),
        message: t('admin.storage.actions.jobMessage', { jobId: res.job_id ?? '—' }),
      })
    },
    onError: (err) => {
      if (structuredErrorCode(err) === 'already_running') {
        addToast({ type: 'warning', title: t('admin.storage.actions.reconcileRunning') })
        return
      }
      addToast({
        type: 'error',
        title: t('admin.storage.actions.reconcileFailed'),
        duration: 0,
      })
    },
  })

  const backfillMetaMut = useMutation<ActionResponse, Error>({
    mutationFn: () => apiClient<ActionResponse>('/admin/storage/backfill-meta', { method: 'POST' }),
    onSuccess: (res) => {
      if (res.status === 'already_running') {
        addToast({ type: 'warning', title: t('admin.storage.actions.backfillMetaRunning') })
        return
      }
      addToast({
        type: 'success',
        title: t('admin.storage.actions.backfillMetaStarted'),
        message: t('admin.storage.actions.jobMessage', { jobId: res.job_id ?? '—' }),
      })
    },
    onError: (err) => {
      if (structuredErrorCode(err) === 'already_running') {
        addToast({ type: 'warning', title: t('admin.storage.actions.backfillMetaRunning') })
        return
      }
      addToast({
        type: 'error',
        title: t('admin.storage.actions.backfillMetaFailed'),
        duration: 0,
      })
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

  function fileTypeLabel(type: string): string {
    const key = `admin.storage.fileTypes.${type}`
    return i18n.exists(key) ? t(key) : t('admin.storage.fileTypes.unknown')
  }

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-neutral-400 py-8">
        <RefreshCw className="w-4 h-4 animate-spin" />
        {t('admin.storage.loading')}
      </div>
    )
  }

  if (isError || !data) {
    return (
      <div className="rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 px-4 py-3 text-sm text-red-600 dark:text-red-400">
        {t('admin.storage.error')}
      </div>
    )
  }

  const keepVersionsLabel =
    data.keep_versions === -1
      ? t('admin.storage.keepVersionsValues.all')
      : data.keep_versions === 0
        ? t('admin.storage.keepVersionsValues.delete')
        : data.keep_versions === 1
          ? t('admin.storage.keepVersionsValues.latest')
          : String(data.keep_versions)

  return (
    <div className="space-y-6">

      {/* Header */}
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">
          {t('admin.storage.title')}
        </h2>
        <button
          onClick={() => refetch()}
          disabled={isFetching}
          className="flex items-center gap-1.5 text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 transition-colors cursor-pointer"
          aria-label={t('admin.storage.refresh')}
        >
          <RefreshCw className={`w-3.5 h-3.5 ${isFetching ? 'animate-spin' : ''}`} />
          {t('admin.storage.refresh')}
        </button>
      </div>

      {/* Config strip */}
      <div className="flex flex-wrap gap-4 rounded-xl border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-800/40 px-4 py-3 text-sm">
        <div className="flex items-center gap-2">
          <span className="text-neutral-500 dark:text-neutral-400">{t('admin.storage.backend')}:</span>
          <BackendBadge
            backend={data.backend}
            label={data.backend === 's3' ? t('admin.storage.backendS3') : t('admin.storage.backendLocal')}
          />
        </div>
        {data.base_path && (
          <div className="flex items-center gap-2">
            <span className="text-neutral-500 dark:text-neutral-400">{t('admin.storage.basePath')}:</span>
            <code className="font-mono text-xs bg-neutral-100 dark:bg-neutral-700 px-2 py-0.5 rounded text-neutral-800 dark:text-neutral-200">
              {data.base_path}
            </code>
          </div>
        )}
        <div className="flex items-center gap-2">
          <span className="text-neutral-500 dark:text-neutral-400">{t('admin.storage.keepVersions')}:</span>
          <span className="text-neutral-800 dark:text-neutral-200">{keepVersionsLabel}</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-neutral-500 dark:text-neutral-400">{t('admin.storage.syncEnabled')}:</span>
          <span className={data.sync_enabled ? 'text-emerald-600 dark:text-emerald-400' : 'text-neutral-500'}>
            {data.sync_enabled ? t('admin.storage.yes') : t('admin.storage.no')}
          </span>
        </div>
      </div>

      {/* Two-column grid: users table + type counts */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">

        {/* Users table */}
        <section className="lg:col-span-2 rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
          <div className="px-4 py-3 border-b border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-800/40">
            <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
              {t('admin.storage.usersTable.heading')}
            </h3>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm" aria-describedby="storage-users-caption">
              <caption id="storage-users-caption" className="sr-only">
                {t('admin.storage.usersTable.heading')}
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
                      aria-label={t('admin.storage.actions.sortByColumn', { column: t('admin.storage.usersTable.user') })}
                    >
                      {t('admin.storage.usersTable.user')}
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
                      aria-label={t('admin.storage.actions.sortByColumn', { column: t('admin.storage.usersTable.diskUsage') })}
                    >
                      {t('admin.storage.usersTable.diskUsage')}
                      <SortIcon col="disk_usage_bytes" sortKey={sortKey} sortDir={sortDir} />
                    </button>
                  </th>
                  <th
                    scope="col"
                    className="px-4 py-2 font-medium text-right"
                    aria-sort={sortKey === 'total_files' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}
                  >
                    <button
                      className="flex items-center gap-1 cursor-pointer hover:text-neutral-700 dark:hover:text-neutral-200 ml-auto"
                      onClick={() => toggleSort('total_files')}
                      aria-label={t('admin.storage.actions.sortByColumn', { column: t('admin.storage.usersTable.fileCount') })}
                    >
                      {t('admin.storage.usersTable.fileCount')}
                      <SortIcon col="total_files" sortKey={sortKey} sortDir={sortDir} />
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
                      aria-label={t('admin.storage.actions.sortByColumn', { column: t('admin.storage.usersTable.archiveSize') })}
                    >
                      <Archive className="w-3 h-3" />
                      {t('admin.storage.usersTable.archiveSize')}
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
                      <td className="px-4 py-2.5 max-w-[240px]">
                        <div className="flex flex-col gap-0.5 min-w-0">
                          <span className="flex items-center gap-1.5 flex-wrap">
                            <span className="font-medium text-sm text-neutral-900 dark:text-neutral-100 truncate">
                              {user.username}
                            </span>
                            {user.deleted_at && (
                              <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-red-500/10 text-red-600 dark:text-red-400">
                                {t('admin.storage.deletedBadge')}
                              </span>
                            )}
                          </span>
                          {user.email && (
                            <span className="text-xs text-neutral-500 dark:text-neutral-400 truncate">
                              {user.email}
                            </span>
                          )}
                          <code className="font-mono text-xs text-neutral-400 dark:text-neutral-500 truncate">
                            {user.library_dir}
                          </code>
                        </div>
                      </td>
                      <td className="px-4 py-2.5 text-right tabular-nums text-neutral-700 dark:text-neutral-300">
                        {fmtBytes(user.disk_usage_bytes)}
                      </td>
                      <td className="px-4 py-2.5 text-right tabular-nums text-neutral-700 dark:text-neutral-300">
                        {user.total_files.toLocaleString(activeLocale)}
                      </td>
                      <td className="px-4 py-2.5 text-right">
                        <span className={`inline-flex items-center gap-1 tabular-nums ${
                          overLimit
                            ? 'text-amber-600 dark:text-amber-400 font-medium'
                            : 'text-neutral-700 dark:text-neutral-300'
                        }`}>
                          {overLimit && <AlertTriangle className="w-3.5 h-3.5 shrink-0" role="img" aria-label={t('admin.storage.archiveWarning')} />}
                          {fmtBytes(user.archive_size_bytes)}
                        </span>
                      </td>
                    </tr>
                  )
                })}
                {sortedUsers.length === 0 && (
                  <tr>
                    <td colSpan={4} className="px-4 py-6 text-center text-sm text-neutral-400">
                      {t('admin.storage.noUsers')}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </section>

        {/* Type counts + Limits */}
        <div className="flex flex-col gap-6">
          <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
            <div className="px-4 py-3 border-b border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-800/40 flex items-center gap-2">
              <Layers className="w-4 h-4 text-neutral-500" />
              <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
                {t('admin.storage.typeCounts.heading')}
              </h3>
            </div>
            <div className="px-4 py-4 space-y-3">
              {typeEntries.length === 0 ? (
                <p className="text-sm text-neutral-400">{t('admin.storage.typeCounts.noData')}</p>
              ) : (
                typeEntries.map(([type, count]) => (
                  <TypeCountCard
                    key={type}
                    count={count}
                    maxCount={maxCount}
                    label={fileTypeLabel(type)}
                    locale={activeLocale}
                  />
                ))
              )}
            </div>
          </section>

          <AdminStorageLimitsSection />
        </div>

      </div>

      {/* Actions */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-4">
        <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
          {t('admin.storage.actions.heading')}
        </h3>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">

          <div className="rounded-lg border border-neutral-100 dark:border-neutral-700 p-4 space-y-3">
            <div>
              <p className="text-sm font-medium text-neutral-800 dark:text-neutral-200">
                {t('admin.storage.actions.reconcile')}
              </p>
              <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
                {t('admin.storage.actions.reconcileDesc')}
              </p>
            </div>
            <button
              onClick={() => reconcileMut.mutate()}
              disabled={anyJobRunning}
              className="w-full px-3 py-1.5 rounded-lg text-sm font-medium border border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors disabled:opacity-50 cursor-pointer disabled:cursor-not-allowed"
            >
              {reconcileMut.isPending ? t('admin.storage.pendingLabel') : t('admin.storage.actions.reconcile')}
            </button>
          </div>

          <div className="rounded-lg border border-neutral-100 dark:border-neutral-700 p-4 space-y-3">
            <div>
              <p className="text-sm font-medium text-neutral-800 dark:text-neutral-200">
                {t('admin.storage.actions.backfillMeta')}
              </p>
              <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
                {t('admin.storage.actions.backfillMetaDesc')}
              </p>
            </div>
            <button
              onClick={() => backfillMetaMut.mutate()}
              disabled={anyJobRunning}
              className="w-full px-3 py-1.5 rounded-lg text-sm font-medium border border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors disabled:opacity-50 cursor-pointer disabled:cursor-not-allowed"
            >
              {backfillMetaMut.isPending ? t('admin.storage.pendingLabel') : t('admin.storage.actions.backfillMeta')}
            </button>
          </div>

        </div>
      </section>

    </div>
  )
}
