import { useState, useRef } from 'react'
import { useClickOutside } from '@/hooks/useClickOutside'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Bell, CheckCircle2, XCircle, Settings, AlertTriangle, Wrench, MessageCircle, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { apiClient } from '@/api/client'

interface Notification {
  id: string
  category: string
  title: string
  body?: string | null
  read: boolean
  created_at: string
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
  const [open, setOpen] = useState(false)
  const [notifications, setNotifications] = useState<Notification[]>([])
  const ref = useRef<HTMLDivElement>(null)
  const queryClient = useQueryClient()

  const UNREAD_KEY = ['notifications', 'unread-count'] as const

  const { data: unreadData } = useQuery({
    queryKey: UNREAD_KEY,
    queryFn: () => apiClient<{ unread_count: number }>('/notifications/unread-count'),
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  })

  const unreadCount = unreadData?.unread_count ?? 0

  // Schließen beim Klick außerhalb des Panels
  useClickOutside(ref, () => setOpen(false), open)

  async function fetchNotifications() {
    try {
      const data = await apiClient<{ items: Notification[]; unread_count: number }>('/notifications?limit=30')
      setNotifications(data.items ?? [])
      queryClient.setQueryData(UNREAD_KEY, { unread_count: data.unread_count })
    } catch {
      // Fehler stillschweigend ignorieren
    }
  }

  async function markAllRead() {
    try {
      await apiClient('/notifications/read-all', { method: 'POST' })
      queryClient.setQueryData(UNREAD_KEY, { unread_count: 0 })
      setNotifications(prev => prev.map(n => ({ ...n, read: true })))
    } catch {
      // Fehler stillschweigend ignorieren
    }
  }

  // Einzelne Benachrichtigung löschen — optimistisches Update
  async function deleteNotification(id: string) {
    // Sofort aus lokalem State entfernen
    setNotifications(prev => prev.filter(n => n.id !== id))
    // Unread-Count anpassen: war die gelöschte Notiz ungelesen?
    const wasUnread = notifications.find(n => n.id === id)?.read === false
    if (wasUnread) {
      queryClient.setQueryData(UNREAD_KEY, (old: { unread_count: number } | undefined) => ({
        unread_count: Math.max(0, (old?.unread_count ?? 1) - 1),
      }))
    }
    try {
      await apiClient(`/notifications/${id}`, { method: 'DELETE' })
    } catch {
      // Bei Fehler Liste neu laden
      fetchNotifications()
    }
  }

  // Alle Benachrichtigungen löschen
  async function deleteAll() {
    // Optimistisches Update: sofort leeren
    setNotifications([])
    queryClient.setQueryData(UNREAD_KEY, { unread_count: 0 })
    try {
      await apiClient('/notifications', { method: 'DELETE' })
    } catch {
      // Bei Fehler Liste neu laden
      fetchNotifications()
    }
  }

  const handleOpen = () => {
    if (!open) fetchNotifications()
    setOpen(o => !o)
  }

  return (
    <div ref={ref} className="relative">
      <button
        onClick={handleOpen}
        className="cursor-pointer relative p-2 rounded-lg text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
        aria-label="Benachrichtigungen"
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
            <span className="text-sm font-medium">Benachrichtigungen</span>
            <div className="flex items-center gap-2">
              {/* "Alle gelesen" nur anzeigen wenn es ungelesene gibt */}
              {unreadCount > 0 && (
                <button
                  onClick={markAllRead}
                  className="cursor-pointer text-xs text-accent-DEFAULT hover:underline"
                >
                  Alle gelesen
                </button>
              )}
              {/* "Alle löschen" nur anzeigen wenn es Benachrichtigungen gibt */}
              {notifications.length > 0 && (
                <button
                  onClick={deleteAll}
                  className="cursor-pointer text-xs text-neutral-400 hover:text-red-500 dark:hover:text-red-400 transition-colors"
                  title="Alle löschen"
                >
                  Alle löschen
                </button>
              )}
            </div>
          </div>
          <div className="max-h-80 overflow-y-auto">
            {notifications.length === 0 ? (
              <p className="text-sm text-neutral-400 text-center py-8">Keine Benachrichtigungen</p>
            ) : (
              notifications.map(n => (
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
                      <p className="text-sm font-medium truncate">{n.title}</p>
                      {n.body && (
                        <p className="text-xs text-neutral-500 dark:text-neutral-400 line-clamp-2">{n.body}</p>
                      )}
                      <p className="text-xs text-neutral-400 mt-0.5">
                        {new Date(n.created_at).toLocaleString('de-DE', { dateStyle: 'short', timeStyle: 'short' })}
                      </p>
                    </div>
                    {/* X-Button: nur bei Hover sichtbar */}
                    <button
                      onClick={() => deleteNotification(n.id)}
                      className="cursor-pointer opacity-0 group-hover:opacity-100 shrink-0 p-0.5 rounded text-neutral-400 hover:text-red-500 dark:hover:text-red-400 transition-all"
                      aria-label="Benachrichtigung löschen"
                      title="Löschen"
                    >
                      <X className="w-3 h-3" />
                    </button>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  )
}
