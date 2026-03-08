import { useState, useEffect, useRef } from 'react'
import { Bell } from 'lucide-react'
import { cn } from '@/lib/utils'
import { apiFetch } from '@/lib/auth'

interface Notification {
  id: string
  category: string
  title: string
  body?: string | null
  read: boolean
  created_at: string
}

const CATEGORY_ICONS: Record<string, string> = {
  enrichment: '⚙️',
  anomaly: '⚠️',
  gear: '🔧',
  chat: '💬',
}

export default function NotificationCenter() {
  const [open, setOpen] = useState(false)
  const [notifications, setNotifications] = useState<Notification[]>([])
  const [unreadCount, setUnreadCount] = useState(0)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    fetchUnreadCount()
    const interval = setInterval(fetchUnreadCount, 30_000)
    return () => clearInterval(interval)
  }, [])

  // Close on click outside
  useEffect(() => {
    if (!open) return
    function handle(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handle)
    return () => document.removeEventListener('mousedown', handle)
  }, [open])

  async function fetchUnreadCount() {
    try {
      const res = await apiFetch('/api/v1/notifications/unread-count')
      if (!res.ok) return
      const data = await res.json() as { unread_count: number }
      setUnreadCount(data.unread_count)
    } catch {
      // silently ignore
    }
  }

  async function fetchNotifications() {
    try {
      const res = await apiFetch('/api/v1/notifications?limit=30')
      if (!res.ok) return
      const data = await res.json() as { items: Notification[]; unread_count: number }
      setNotifications(data.items ?? [])
      setUnreadCount(data.unread_count)
    } catch {
      // silently ignore
    }
  }

  async function markAllRead() {
    try {
      await apiFetch('/api/v1/notifications/read-all', { method: 'POST' })
      setUnreadCount(0)
      setNotifications(prev => prev.map(n => ({ ...n, read: true })))
    } catch {
      // silently ignore
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
        className="relative p-2 rounded-lg text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
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
            {unreadCount > 0 && (
              <button
                onClick={markAllRead}
                className="text-xs text-accent-DEFAULT hover:underline"
              >
                Alle gelesen
              </button>
            )}
          </div>
          <div className="max-h-80 overflow-y-auto">
            {notifications.length === 0 ? (
              <p className="text-sm text-neutral-400 text-center py-8">Keine Benachrichtigungen</p>
            ) : (
              notifications.map(n => (
                <div
                  key={n.id}
                  className={cn(
                    'px-3 py-2 border-b border-neutral-100 dark:border-neutral-800 last:border-0',
                    !n.read && 'bg-accent-50 dark:bg-accent-950/20',
                  )}
                >
                  <div className="flex items-start gap-2">
                    <span className="text-sm mt-0.5">{CATEGORY_ICONS[n.category] ?? '🔔'}</span>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium truncate">{n.title}</p>
                      {n.body && (
                        <p className="text-xs text-neutral-500 dark:text-neutral-400 line-clamp-2">{n.body}</p>
                      )}
                      <p className="text-xs text-neutral-400 mt-0.5">
                        {new Date(n.created_at).toLocaleString('de-DE', { dateStyle: 'short', timeStyle: 'short' })}
                      </p>
                    </div>
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
