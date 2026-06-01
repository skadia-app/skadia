import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { getAuditLog, type AuditLogEntry } from '@/api/auth'

const EVENT_TYPES = [
  'login_success',
  'login_failure',
  'register',
  'password_change',
  'password_reset',
  'session_revoke',
  'admin_activate_user',
  'admin_deactivate_user',
  'admin_revoke_sessions',
  'admin_create_user',
  'admin_reset_password',
  'admin_restore_user',
  'account_deletion_requested',
  'account_deletion_cancelled',
  'account_purged',
  'email_changed',
  'profile_update',
  'data_export_requested',
  'email_change_requested',
  'admin_toggle_hotfolder',
  'admin_promote_user',
  'admin_demote_user',
  'admin_delete_user',
  'admin_settings_update',
  'bootstrap_admin_created',
] as const

const EVENT_COLORS: Record<string, string> = {
  login_success: 'bg-green-500/10 text-green-600 dark:text-green-400',
  login_failure: 'bg-red-500/10 text-red-600 dark:text-red-400',
  register: 'bg-blue-500/10 text-blue-600 dark:text-blue-400',
  password_change: 'bg-amber-500/10 text-amber-600 dark:text-amber-400',
  password_reset: 'bg-amber-500/10 text-amber-600 dark:text-amber-400',
  session_revoke: 'bg-orange-500/10 text-orange-600 dark:text-orange-400',
  admin_activate_user: 'bg-green-500/10 text-green-600 dark:text-green-400',
  admin_deactivate_user: 'bg-red-500/10 text-red-600 dark:text-red-400',
  admin_revoke_sessions: 'bg-orange-500/10 text-orange-600 dark:text-orange-400',
  admin_create_user: 'bg-blue-500/10 text-blue-600 dark:text-blue-400',
  admin_reset_password: 'bg-amber-500/10 text-amber-600 dark:text-amber-400',
  admin_restore_user: 'bg-green-500/10 text-green-600 dark:text-green-400',
  account_deletion_requested: 'bg-red-500/10 text-red-600 dark:text-red-400',
  account_deletion_cancelled: 'bg-green-500/10 text-green-600 dark:text-green-400',
  account_purged: 'bg-red-500/10 text-red-600 dark:text-red-400',
  email_changed: 'bg-blue-500/10 text-blue-600 dark:text-blue-400',
  profile_update: 'bg-blue-500/10 text-blue-600 dark:text-blue-400',
  data_export_requested: 'bg-blue-500/10 text-blue-600 dark:text-blue-400',
  admin_toggle_hotfolder: 'bg-amber-500/10 text-amber-600 dark:text-amber-400',
  admin_promote_user: 'bg-purple-500/10 text-purple-600 dark:text-purple-400',
  admin_demote_user: 'bg-orange-500/10 text-orange-600 dark:text-orange-400',
  admin_delete_user: 'bg-red-500/10 text-red-600 dark:text-red-400',
  admin_settings_update: 'bg-blue-500/10 text-blue-600 dark:text-blue-400',
  bootstrap_admin_created: 'bg-purple-500/10 text-purple-600 dark:text-purple-400',
}

const PAGE_SIZE = 50

function formatTimestamp(iso: string, locale: string): string {
  const d = new Date(iso)
  return d.toLocaleString(locale, {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function EventBadge({ eventType, t }: { eventType: string; t: TFunction }) {
  const color = EVENT_COLORS[eventType] ?? 'bg-neutral-500/10 text-neutral-600 dark:text-neutral-400'
  const label = EVENT_TYPES.includes(eventType as (typeof EVENT_TYPES)[number])
    ? t(`adminAuditLog.events.${eventType}`)
    : t('adminAuditLog.events.unknown')

  return (
    <span className={`inline-flex items-center rounded px-2 py-0.5 text-xs font-medium ${color}`}>
      {label}
    </span>
  )
}

function DetailsCell({ details }: { details: Record<string, unknown> | null }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)

  if (!details || Object.keys(details).length === 0) {
    return <span className="text-neutral-400">—</span>
  }

  const json = JSON.stringify(details, null, 2)
  const preview = JSON.stringify(details)

  return (
    <div>
      {expanded ? (
        <div className="relative">
          <pre className="max-w-xs overflow-auto rounded bg-neutral-100 p-2 text-xs dark:bg-neutral-800">
            {json}
          </pre>
          <button
            onClick={() => setExpanded(false)}
            className="mt-1 text-xs text-blue-600 hover:underline dark:text-blue-400 cursor-pointer"
          >
            {t('adminAuditLog.collapse')}
          </button>
        </div>
      ) : (
        <button
          onClick={() => setExpanded(true)}
          className="max-w-[180px] truncate text-left text-xs text-neutral-500 hover:text-neutral-800 hover:underline dark:hover:text-neutral-200 cursor-pointer"
          title={preview}
        >
          {preview}
        </button>
      )}
    </div>
  )
}

export default function AdminAuditLogPage() {
  const { t, i18n } = useTranslation()
  const [filterEventType, setFilterEventType] = useState('')
  const [filterUser, setFilterUser] = useState('')
  const [filterFrom, setFilterFrom] = useState('')
  const [filterTo, setFilterTo] = useState('')
  const [offset, setOffset] = useState(0)
  const [autoRefresh, setAutoRefresh] = useState(false)

  const { data, isLoading, isError } = useQuery({
    queryKey: ['audit-log', filterEventType, filterUser, filterFrom, filterTo, offset],
    queryFn: () =>
      getAuditLog({
        event_type: filterEventType || undefined,
        user_id: filterUser || undefined,
        from: filterFrom || undefined,
        to: filterTo || undefined,
        limit: PAGE_SIZE,
        offset,
      }),
    refetchInterval: autoRefresh ? 30_000 : false,
  })

  const entries: AuditLogEntry[] = data?.entries ?? []
  const total: number = data?.total ?? 0
  const currentPage = Math.floor(offset / PAGE_SIZE) + 1
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  function applyFilters() {
    setOffset(0)
  }

  function resetFilters() {
    setFilterEventType('')
    setFilterUser('')
    setFilterFrom('')
    setFilterTo('')
    setOffset(0)
  }

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t('adminAuditLog.title')}</h1>
        <label className="flex items-center gap-2 text-sm text-neutral-600 dark:text-neutral-400 cursor-pointer select-none">
          <input
            type="checkbox"
            checked={autoRefresh}
            onChange={e => setAutoRefresh(e.target.checked)}
            className="h-4 w-4 rounded border-neutral-300"
          />
          {t('adminAuditLog.autoRefresh')}
        </label>
      </div>

      {/* Filter bar */}
      <div className="flex flex-wrap gap-3 rounded-lg border border-neutral-200 bg-neutral-50 p-4 dark:border-neutral-700 dark:bg-neutral-900">
        {/* Event type dropdown */}
        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-neutral-500">{t('adminAuditLog.filters.eventType')}</label>
          <select
            value={filterEventType}
            onChange={e => setFilterEventType(e.target.value)}
            className="h-9 rounded border border-neutral-300 bg-white px-2 text-sm dark:border-neutral-600 dark:bg-neutral-800"
          >
            <option value="">{t('adminAuditLog.filters.allTypes')}</option>
            {EVENT_TYPES.map((key) => (
              <option key={key} value={key}>
                {t(`adminAuditLog.events.${key}`)}
              </option>
            ))}
          </select>
        </div>

        {/* User search */}
        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-neutral-500">{t('adminAuditLog.filters.userId')}</label>
          <input
            type="text"
            value={filterUser}
            onChange={e => setFilterUser(e.target.value)}
            placeholder={t('adminAuditLog.filters.userIdPlaceholder')}
            className="h-9 w-52 rounded border border-neutral-300 bg-white px-2 text-sm dark:border-neutral-600 dark:bg-neutral-800"
          />
        </div>

        {/* Date range */}
        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-neutral-500">{t('adminAuditLog.filters.from')}</label>
          <input
            type="datetime-local"
            value={filterFrom}
            onChange={e => setFilterFrom(e.target.value)}
            className="h-9 rounded border border-neutral-300 bg-white px-2 text-sm dark:border-neutral-600 dark:bg-neutral-800"
          />
        </div>

        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-neutral-500">{t('adminAuditLog.filters.to')}</label>
          <input
            type="datetime-local"
            value={filterTo}
            onChange={e => setFilterTo(e.target.value)}
            className="h-9 rounded border border-neutral-300 bg-white px-2 text-sm dark:border-neutral-600 dark:bg-neutral-800"
          />
        </div>

        {/* Buttons */}
        <div className="flex items-end gap-2">
          <button
            onClick={applyFilters}
            className="h-9 cursor-pointer rounded bg-accent-DEFAULT px-4 text-sm font-medium text-white hover:bg-accent-600"
          >
            {t('adminAuditLog.filters.apply')}
          </button>
          <button
            onClick={resetFilters}
            className="h-9 cursor-pointer rounded border border-neutral-300 px-4 text-sm text-neutral-600 hover:bg-neutral-100 dark:border-neutral-600 dark:text-neutral-400 dark:hover:bg-neutral-800"
          >
            {t('common.reset')}
          </button>
        </div>
      </div>

      {/* Table */}
      <div className="overflow-x-auto rounded-lg border border-neutral-200 dark:border-neutral-700">
        <table className="w-full text-sm">
          <thead className="bg-neutral-50 dark:bg-neutral-900">
            <tr>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">{t('adminAuditLog.table.timestamp')}</th>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">{t('adminAuditLog.table.event')}</th>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">{t('adminAuditLog.table.user')}</th>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">{t('adminAuditLog.table.ipAddress')}</th>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">{t('adminAuditLog.table.details')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
            {isLoading && (
              <tr>
                <td colSpan={5} className="px-4 py-8 text-center text-neutral-500">
                  {t('adminAuditLog.loading')}
                </td>
              </tr>
            )}
            {isError && (
              <tr>
                <td colSpan={5} className="px-4 py-8 text-center text-red-500">
                  {t('adminAuditLog.loadError')}
                </td>
              </tr>
            )}
            {!isLoading && !isError && entries.length === 0 && (
              <tr>
                <td colSpan={5} className="px-4 py-8 text-center text-neutral-500">
                  {t('adminAuditLog.empty')}
                </td>
              </tr>
            )}
            {entries.map(entry => (
              <tr key={entry.id} className="hover:bg-neutral-50 dark:hover:bg-neutral-800/50">
                <td className="whitespace-nowrap px-4 py-3 font-mono text-xs text-neutral-600 dark:text-neutral-400">
                  {formatTimestamp(entry.created_at, i18n.language)}
                </td>
                <td className="px-4 py-3">
                  <EventBadge eventType={entry.event_type} t={t} />
                </td>
                <td className="px-4 py-3 text-neutral-800 dark:text-neutral-200">
                  {entry.username ?? <span className="text-neutral-400">—</span>}
                </td>
                <td className="whitespace-nowrap px-4 py-3 font-mono text-xs text-neutral-600 dark:text-neutral-400">
                  {entry.ip_address ?? <span className="text-neutral-400">—</span>}
                </td>
                <td className="px-4 py-3">
                  <DetailsCell details={entry.details} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      <div className="flex items-center justify-between text-sm text-neutral-600 dark:text-neutral-400">
        <span>
          {total > 0
            ? t('adminAuditLog.pagination.range', {
              start: offset + 1,
              end: Math.min(offset + PAGE_SIZE, total),
              total,
            })
            : t('adminAuditLog.pagination.empty')}
        </span>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
            disabled={offset === 0}
            className="rounded border border-neutral-300 px-3 py-1 hover:bg-neutral-100 cursor-pointer disabled:cursor-not-allowed disabled:opacity-40 dark:border-neutral-600 dark:hover:bg-neutral-800"
          >
            {t('common.back')}
          </button>
          <span>
            {t('adminAuditLog.pagination.page', { currentPage, totalPages })}
          </span>
          <button
            onClick={() => setOffset(offset + PAGE_SIZE)}
            disabled={offset + PAGE_SIZE >= total}
            className="rounded border border-neutral-300 px-3 py-1 hover:bg-neutral-100 cursor-pointer disabled:cursor-not-allowed disabled:opacity-40 dark:border-neutral-600 dark:hover:bg-neutral-800"
          >
            {t('common.next')}
          </button>
        </div>
      </div>
    </div>
  )
}
