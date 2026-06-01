import { useState, useRef } from 'react'
import { useClickOutside } from '@/hooks/useClickOutside'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Bell, CheckCircle2, XCircle, Settings, AlertTriangle, Wrench, MessageCircle, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { apiClient } from '@/api/client'
import { i18n } from '@/i18n/i18n'
import { localeForLanguage, isSupportedLanguage } from '@/i18n/languages'
import { getJobNotificationTitle } from '@/lib/jobLabels'

export interface NotificationItem {
  id: string
  category: string
  title: string
  body?: string | null
  read: boolean
  created_at: string
  data?: {
    key?: string
    message_key?: string
    params?: Record<string, unknown>
    message_params?: Record<string, unknown>
  } | null
}

export function localizeNotification(notification: NotificationItem): { title: string; body: string | null } {
  const data = notification.data
  const key = data?.key ?? data?.message_key
  const params = data?.params ?? data?.message_params ?? {}

  if (key === 'notifications.jobs.completed' || key === 'notifications.jobs.failed') {
    const jobType = typeof params.jobType === 'string' ? params.jobType : typeof params.job_type === 'string' ? params.job_type : 'unknown'
    const entityName = typeof params.entityName === 'string' ? params.entityName : typeof params.entity_name === 'string' ? params.entity_name : undefined
    const status = key.endsWith('.failed') ? 'failed' : 'completed'
    return {
      title: getJobNotificationTitle({ job_type: jobType, entity_name: entityName, status }),
      body: null,
    }
  }

  if (key) {
    const titleKey = `${key}.title`
    const bodyKey = `${key}.body`
    if (i18n.exists(titleKey)) {
      const title = i18n.t(titleKey, params)
      const body = i18n.exists(bodyKey) ? i18n.t(bodyKey, params) : ''
      return { title, body: body || null }
    }
  }

  return { title: notification.title, body: notification.body ?? null }
}

function NotificationIcon({ category }: { category: string }) {
  switch (category) {
    case 'job':
      return <CheckCircle2 className="w-4 h-4 text-green-500 shrink-0 mt-0.5" />
    case 'job_error':
      return <XCircle className="w-4 h-4 text-red-500 shrink-0 mt-0.5" />
    case 'enrichment':
      return <Settings className="w-4 h-4 text-neutral-400 shrink-0 mt-0.5" />
    case 'anomaly':
      return <AlertTriangle className="w-4 h-4 text-yellow-500 shrink-0 mt-0.5" />
    case 'gear':
      return <Wrench className="w-4 h-4 text-neutral-400 shrink-0 mt-0.5" />
    case 'chat':
      return <MessageCircle className="w-4 h-4 text-neutral-400 shrink-0 mt-0.5" />
    default:
      return <Bell className="w-4 h-4 text-neutral-400 shrink-0 mt-0.5" />
  }
}

export default function NotificationCenter() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [notifications, setNotifications] = useState<NotificationItem[]>([])
  const ref = useRef<HTMLDivElement>(null)
  const queryClient = useQueryClient()

  const UNREAD_KEY = ['notifications', 'unread-count'] as const

  const { data: unreadData } = useQuery({
    queryKey: UNREAD_KEY,
    queryFn: () => apiClient<{ unread_count: number }>('/notifications/unread-count'),
    // SSE delivers unread count every 5s via setQueryData. This 120s poll is a
    // safety net for edge cases (suspended tabs, SSE failure, leader crash).
    refetchInterval: 120_000,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
    staleTime: 30_000,
  })

  const unreadCount = unreadData?.unread_count ?? 0

  // Close on outside click.
  useClickOutside(ref, () => setOpen(false), open)

  async function fetchNotifications() {
    try {
      const data = await apiClient<{ items: NotificationItem[]; unread_count: number }>('/notifications?limit=30')
      setNotifications(data.items ?? [])
      queryClient.setQueryData(UNREAD_KEY, { unread_count: data.unread_count })
    } catch {
      // Ignore transient notification refresh failures.
    }
  }

  async function markAllRead() {
    try {
      await apiClient('/notifications/read-all', { method: 'POST' })
      queryClient.setQueryData(UNREAD_KEY, { unread_count: 0 })
      setNotifications(prev => prev.map(n => ({ ...n, read: true })))
    } catch {
      // Ignore transient notification refresh failures.
    }
  }

  // Delete a single notification with an optimistic update.
  async function deleteNotification(id: string) {
    setNotifications(prev => prev.filter(n => n.id !== id))
    const wasUnread = notifications.find(n => n.id === id)?.read === false
    if (wasUnread) {
      queryClient.setQueryData(UNREAD_KEY, (old: { unread_count: number } | undefined) => ({
        unread_count: Math.max(0, (old?.unread_count ?? 1) - 1),
      }))
    }
    try {
      await apiClient(`/notifications/${id}`, { method: 'DELETE' })
    } catch {
      // Reload the list if the optimistic delete failed.
      fetchNotifications()
    }
  }

  // Delete all notifications with an optimistic update.
  async function deleteAll() {
    setNotifications([])
    queryClient.setQueryData(UNREAD_KEY, { unread_count: 0 })
    try {
      await apiClient('/notifications', { method: 'DELETE' })
    } catch {
      // Reload the list if the optimistic delete failed.
      fetchNotifications()
    }
  }

  const language = isSupportedLanguage(i18n.language) ? i18n.language : 'de'

  const handleOpen = () => {
    if (!open) fetchNotifications()
    setOpen(o => !o)
  }

  return (
    <div ref={ref} className="relative">
      <button
        onClick={handleOpen}
        className="cursor-pointer relative p-2 rounded-lg text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
        aria-label={t('notifications.panelTitle')}
      >
        <Bell className="w-4 h-4" />
        {unreadCount > 0 && (
          <span className="absolute -top-0.5 -right-0.5 w-4 h-4 bg-red-500 text-white text-[10px] font-bold rounded-full flex items-center justify-center">
            {unreadCount > 9 ? '9+' : unreadCount}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-2 w-80 bg-white dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-800 rounded-xl shadow-lg z-50 overflow-hidden">
          <div className="flex items-center justify-between px-3 py-2 border-b border-neutral-200 dark:border-neutral-800">
            <span className="text-sm font-medium">{t('notifications.panelTitle')}</span>
            <div className="flex items-center gap-2">
              {unreadCount > 0 && (
                <button
                  onClick={markAllRead}
                  className="cursor-pointer text-xs text-accent-DEFAULT hover:underline"
                >
                  {t('notifications.markAllRead')}
                </button>
              )}
              {notifications.length > 0 && (
                <button
                  onClick={deleteAll}
                  className="cursor-pointer text-xs text-neutral-400 hover:text-red-500 dark:hover:text-red-400 transition-colors"
                  title={t('notifications.deleteAll')}
                >
                  {t('notifications.deleteAll')}
                </button>
              )}
            </div>
          </div>
          <div className="max-h-80 overflow-y-auto">
            {notifications.length === 0 ? (
              <p className="text-sm text-neutral-400 text-center py-8">{t('notifications.empty')}</p>
            ) : (
              notifications.map(n => {
                const localized = localizeNotification(n)
                return (
                  <div
                    key={n.id}
                    className={cn(
                      'group px-3 py-2 border-b border-neutral-100 dark:border-neutral-800 last:border-0',
                      !n.read && 'bg-accent-100 dark:bg-accent-DEFAULT/10',
                    )}
                  >
                    <div className="flex items-start gap-2">
                      <NotificationIcon category={n.category} />
                      <div className="flex-1 min-w-0">
                        <p className="text-sm font-medium truncate">{localized.title}</p>
                        {localized.body && (
                          <p className="text-xs text-neutral-500 dark:text-neutral-400 line-clamp-2">{localized.body}</p>
                        )}
                        <p className="text-xs text-neutral-400 mt-0.5">
                          {new Date(n.created_at).toLocaleString(localeForLanguage(language), { dateStyle: 'short', timeStyle: 'short' })}
                        </p>
                      </div>
                      <button
                        onClick={() => deleteNotification(n.id)}
                        className="cursor-pointer opacity-0 group-hover:opacity-100 shrink-0 p-0.5 rounded text-neutral-400 hover:text-red-500 dark:hover:text-red-400 transition-all"
                        aria-label={t('notifications.deleteOne')}
                        title={t('notifications.deleteOneShort')}
                      >
                        <X className="w-3 h-3" />
                      </button>
                    </div>
                  </div>
                )
              })
            )}
          </div>
        </div>
      )}
    </div>
  )
}
