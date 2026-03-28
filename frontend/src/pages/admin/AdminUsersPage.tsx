import { useState, useCallback } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  getAdminUsers,
  getAuthConfig,
  adminActivateUser,
  adminDeactivateUser,
  adminResetPassword,
  adminCreateUser,
  getAdminUserSessions,
  adminRevokeUserSessions,
  type AdminUser,
  type AdminUserSession,
} from '@/api/auth'
import { getAuthErrorMessage } from '@/pages/auth/errorMessages'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'
import { useDebounce } from '@/lib/useDebounce'
import { useToast } from '@/lib/toast'
import { ModalShell } from '@/components/ui/ModalShell'
import {
  Search,
  Plus,
  MoreHorizontal,
  Shield,
  ShieldOff,
  KeyRound,
  Monitor,
  Ban,
  Loader2,
} from 'lucide-react'

// ── Centralized i18n-ready labels ───────────────────────────────────────────

const STATUS_LABELS: Record<string, string> = {
  active: 'Aktiv',
  inactive: 'Inaktiv',
  deleted: 'Gelöscht',
  all: 'Alle',
}

const REGISTRATION_SOURCE_LABELS: Record<string, string> = {
  admin: 'Admin',
  registration: 'Registrierung',
  invite: 'Einladung',
  oauth_google: 'Google',
  oauth_github: 'GitHub',
}

const PAGE_SIZE = 50

// ── Helpers ─────────────────────────────────────────────────────────────────

function formatTimestamp(iso: string | null): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return d.toLocaleString('de-DE', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function StatusBadge({ user }: { user: AdminUser }) {
  if (user.deleted_at) {
    return (
      <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-red-500/10 text-red-600 dark:text-red-400">
        {STATUS_LABELS.deleted}
      </span>
    )
  }
  if (!user.is_active) {
    return (
      <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-amber-500/10 text-amber-600 dark:text-amber-400">
        {STATUS_LABELS.inactive}
      </span>
    )
  }
  return (
    <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-green-500/10 text-green-600 dark:text-green-400">
      {STATUS_LABELS.active}
    </span>
  )
}

function SourceBadge({ source }: { source: string }) {
  const label = REGISTRATION_SOURCE_LABELS[source] ?? source
  return (
    <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-blue-500/10 text-blue-600 dark:text-blue-400">
      {label}
    </span>
  )
}

// ── Create User Dialog ──────────────────────────────────────────────────────

function CreateUserDialog({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [email, setEmail] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const { addToast } = useToast()
  const { data: authConfig } = useQuery({ queryKey: ['authConfig'], queryFn: getAuthConfig })
  const smtpConfigured = authConfig?.smtp_configured ?? true

  const mutation = useMutation({
    mutationFn: () =>
      adminCreateUser({
        email,
        username,
        password: password || undefined,
      }),
    onSuccess: (data) => {
      if (data.status === 'email_exists') {
        addToast({ type: 'error', title: getAuthErrorMessage('email_exists'), duration: 0 })
        return
      }
      if (password) {
        addToast({ type: 'success', title: 'Benutzer erstellt' })
      } else if (data.invite_sent) {
        addToast({ type: 'success', title: 'Einladungs-Email gesendet' })
      } else {
        addToast({ type: 'success', title: 'Benutzer erstellt' })
      }
      onCreated()
      onClose()
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: getAuthErrorMessage(err.message), duration: 0 })
    },
  })

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-sm">
      <form
        onSubmit={e => {
          e.preventDefault()
          mutation.mutate()
        }}
        className="space-y-4"
      >
        <h2 className="text-lg font-semibold">Benutzer erstellen</h2>

        <div className="space-y-3">
          <div className="flex flex-col gap-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">E-Mail *</label>
            <input
              type="email"
              required
              value={email}
              onChange={e => setEmail(e.target.value)}
              className={AUTH_INPUT_CLASS}
              placeholder="benutzer@example.com"
            />
          </div>

          <div className="flex flex-col gap-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Benutzername *</label>
            <input
              type="text"
              required
              value={username}
              onChange={e => setUsername(e.target.value)}
              className={AUTH_INPUT_CLASS}
              placeholder="benutzername"
            />
          </div>

          <div className="flex flex-col gap-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              {smtpConfigured ? 'Passwort (optional)' : 'Passwort *'}
            </label>
            <input
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              className={AUTH_INPUT_CLASS}
              placeholder={smtpConfigured ? 'Leer = Einladungs-Email wird gesendet' : 'Passwort ist erforderlich (kein SMTP konfiguriert)'}
              required={!smtpConfigured}
              minLength={password ? 10 : undefined}
            />
            {password && password.length > 0 && password.length < 10 && (
              <p className="text-xs text-rose-600 dark:text-rose-400">Mindestens 10 Zeichen erforderlich</p>
            )}
          </div>
        </div>

        <div className="flex justify-end gap-2 pt-2">
          <button
            type="button"
            onClick={onClose}
            disabled={mutation.isPending}
            className="cursor-pointer rounded-lg border border-neutral-300 px-4 py-2 text-sm text-neutral-600 hover:bg-neutral-100 dark:border-neutral-600 dark:text-neutral-400 dark:hover:bg-neutral-800 disabled:cursor-not-allowed disabled:opacity-50"
          >
            Abbrechen
          </button>
          <button
            type="submit"
            disabled={mutation.isPending || !email || !username || (password.length > 0 && password.length < 10)}
            className="flex cursor-pointer items-center gap-2 rounded-lg bg-accent-DEFAULT px-4 py-2 text-sm font-medium text-white hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {mutation.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
            Erstellen
          </button>
        </div>
      </form>
    </ModalShell>
  )
}

// ── Reset Password Dialog ───────────────────────────────────────────────────

function ResetPasswordDialog({
  user,
  onClose,
}: {
  user: AdminUser
  onClose: () => void
}) {
  const [mode, setMode] = useState<'set' | 'email'>('set')
  const [newPassword, setNewPassword] = useState('')
  const { addToast } = useToast()
  const queryClient = useQueryClient()

  const mutation = useMutation({
    mutationFn: () =>
      adminResetPassword(user.id, {
        password: mode === 'set' ? newPassword : undefined,
        send_email: mode === 'email',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      if (mode === 'email') {
        addToast({ type: 'success', title: 'Reset-Email gesendet' })
      } else {
        addToast({ type: 'success', title: 'Passwort gesetzt' })
      }
      onClose()
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: getAuthErrorMessage(err.message), duration: 0 })
    },
  })

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-sm">
      <form
        onSubmit={e => {
          e.preventDefault()
          mutation.mutate()
        }}
        className="space-y-4"
      >
        <h2 className="text-lg font-semibold">Passwort zurücksetzen</h2>
        <p className="text-sm text-neutral-600 dark:text-neutral-400">
          Passwort für <strong>{user.username}</strong> zurücksetzen.
        </p>

        <div className="space-y-2">
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="radio"
              checked={mode === 'set'}
              onChange={() => setMode('set')}
              className="h-4 w-4"
            />
            <span className="text-sm">Passwort setzen</span>
          </label>
          {mode === 'set' && (
            <input
              type="password"
              value={newPassword}
              onChange={e => setNewPassword(e.target.value)}
              placeholder="Neues Passwort (min. 10 Zeichen)"
              className={`${AUTH_INPUT_CLASS} mt-2`}
              required
              minLength={10}
            />
          )}
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="radio"
              checked={mode === 'email'}
              onChange={() => setMode('email')}
              className="h-4 w-4"
            />
            <span className="text-sm">Reset-Email senden</span>
          </label>
        </div>

        <div className="flex justify-end gap-2 pt-2">
          <button
            type="button"
            onClick={onClose}
            disabled={mutation.isPending}
            className="cursor-pointer rounded-lg border border-neutral-300 px-4 py-2 text-sm text-neutral-600 hover:bg-neutral-100 dark:border-neutral-600 dark:text-neutral-400 dark:hover:bg-neutral-800 disabled:cursor-not-allowed disabled:opacity-50"
          >
            Abbrechen
          </button>
          <button
            type="submit"
            disabled={mutation.isPending || (mode === 'set' && newPassword.length < 10)}
            className="flex cursor-pointer items-center gap-2 rounded-lg bg-amber-600 px-4 py-2 text-sm font-medium text-white hover:bg-amber-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {mutation.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
            Zurücksetzen
          </button>
        </div>
      </form>
    </ModalShell>
  )
}

// ── Sessions Dialog ─────────────────────────────────────────────────────────

function SessionsDialog({
  user,
  onClose,
}: {
  user: AdminUser
  onClose: () => void
}) {
  const { addToast } = useToast()
  const queryClient = useQueryClient()

  const { data, isLoading, isError } = useQuery({
    queryKey: ['admin-user-sessions', user.id],
    queryFn: () => getAdminUserSessions(user.id),
  })

  const revokeMutation = useMutation({
    mutationFn: () => adminRevokeUserSessions(user.id),
    onSuccess: () => {
      addToast({ type: 'success', title: 'Alle Sitzungen widerrufen' })
      queryClient.invalidateQueries({ queryKey: ['admin-user-sessions', user.id] })
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: getAuthErrorMessage(err.message), duration: 0 })
    },
  })

  const sessions: AdminUserSession[] = data?.sessions ?? []
  const activeSessions = sessions.filter(s => !s.revoked && new Date(s.expires_at) > new Date())

  function parseUserAgent(ua: string): string {
    if (!ua) return '—'
    // Extract browser name from user agent
    if (ua.includes('Firefox')) return 'Firefox'
    if (ua.includes('Edg/')) return 'Edge'
    if (ua.includes('Chrome')) return 'Chrome'
    if (ua.includes('Safari')) return 'Safari'
    return ua.length > 40 ? ua.slice(0, 40) + '...' : ua
  }

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-xl">
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold">
            Sitzungen von {user.username}
          </h2>
          {activeSessions.length > 0 && (
            <button
              onClick={() => revokeMutation.mutate()}
              disabled={revokeMutation.isPending}
              className="flex cursor-pointer items-center gap-1.5 rounded bg-red-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {revokeMutation.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Ban className="h-3.5 w-3.5" />
              )}
              Alle widerrufen
            </button>
          )}
        </div>

        {isLoading && (
          <div className="flex justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-neutral-400" />
          </div>
        )}

        {isError && (
          <p className="py-4 text-center text-sm text-red-500">
            Fehler beim Laden der Sitzungen.
          </p>
        )}

        {!isLoading && !isError && sessions.length === 0 && (
          <p className="py-4 text-center text-sm text-neutral-500">
            Keine aktiven Sitzungen.
          </p>
        )}

        {!isLoading && !isError && sessions.length > 0 && (
          <div className="overflow-x-auto rounded-lg border border-neutral-200 dark:border-neutral-700">
            <table className="w-full text-sm">
              <thead className="bg-neutral-50 dark:bg-neutral-900">
                <tr>
                  <th className="px-3 py-2 text-left font-medium text-neutral-500">Browser</th>
                  <th className="px-3 py-2 text-left font-medium text-neutral-500">IP</th>
                  <th className="px-3 py-2 text-left font-medium text-neutral-500">Erstellt</th>
                  <th className="px-3 py-2 text-left font-medium text-neutral-500">Ablauf</th>
                  <th className="px-3 py-2 text-left font-medium text-neutral-500">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
                {sessions.map(session => {
                  const expired = new Date(session.expires_at) < new Date()
                  return (
                    <tr key={session.family_id} className="hover:bg-neutral-50 dark:hover:bg-neutral-800/50">
                      <td className="px-3 py-2 text-neutral-800 dark:text-neutral-200" title={session.user_agent}>
                        {parseUserAgent(session.user_agent)}
                      </td>
                      <td className="whitespace-nowrap px-3 py-2 font-mono text-xs text-neutral-600 dark:text-neutral-400">
                        {session.ip_address || '—'}
                      </td>
                      <td className="whitespace-nowrap px-3 py-2 text-xs text-neutral-600 dark:text-neutral-400">
                        {formatTimestamp(session.created_at)}
                      </td>
                      <td className="whitespace-nowrap px-3 py-2 text-xs text-neutral-600 dark:text-neutral-400">
                        {formatTimestamp(session.expires_at)}
                      </td>
                      <td className="px-3 py-2">
                        {session.revoked ? (
                          <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-red-500/10 text-red-600 dark:text-red-400">
                            Widerrufen
                          </span>
                        ) : expired ? (
                          <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-neutral-500/10 text-neutral-600 dark:text-neutral-400">
                            Abgelaufen
                          </span>
                        ) : (
                          <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-green-500/10 text-green-600 dark:text-green-400">
                            Aktiv
                          </span>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}

        <div className="flex justify-end">
          <button
            onClick={onClose}
            className="cursor-pointer rounded-lg border border-neutral-300 px-4 py-2 text-sm text-neutral-600 hover:bg-neutral-100 dark:border-neutral-600 dark:text-neutral-400 dark:hover:bg-neutral-800"
          >
            Schließen
          </button>
        </div>
      </div>
    </ModalShell>
  )
}

// ── Confirmation Dialog ─────────────────────────────────────────────────────

function ConfirmDialog({
  title,
  message,
  confirmLabel,
  confirmClass,
  isPending,
  onConfirm,
  onClose,
}: {
  title: string
  message: string
  confirmLabel: string
  confirmClass?: string
  isPending: boolean
  onConfirm: () => void
  onClose: () => void
}) {
  return (
    <ModalShell onClose={onClose} maxWidth="max-w-sm">
      <div className="space-y-4">
        <h2 className="text-lg font-semibold">{title}</h2>
        <p className="text-sm text-neutral-600 dark:text-neutral-400">{message}</p>
        <div className="flex justify-end gap-2 pt-2">
          <button
            onClick={onClose}
            disabled={isPending}
            className="cursor-pointer rounded-lg border border-neutral-300 px-4 py-2 text-sm text-neutral-600 hover:bg-neutral-100 dark:border-neutral-600 dark:text-neutral-400 dark:hover:bg-neutral-800 disabled:cursor-not-allowed disabled:opacity-50"
          >
            Abbrechen
          </button>
          <button
            onClick={onConfirm}
            disabled={isPending}
            className={`flex cursor-pointer items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white disabled:opacity-50 disabled:cursor-not-allowed ${confirmClass ?? 'bg-red-600 hover:bg-red-700'}`}
          >
            {isPending && <Loader2 className="h-4 w-4 animate-spin" />}
            {confirmLabel}
          </button>
        </div>
      </div>
    </ModalShell>
  )
}

// ── User Actions Menu ───────────────────────────────────────────────────────

function UserActionsMenu({
  user,
  onAction,
}: {
  user: AdminUser
  onAction: (action: 'activate' | 'deactivate' | 'reset-password' | 'sessions' | 'revoke-sessions') => void
}) {
  const [open, setOpen] = useState(false)

  const handleAction = useCallback(
    (action: 'activate' | 'deactivate' | 'reset-password' | 'sessions' | 'revoke-sessions') => {
      setOpen(false)
      onAction(action)
    },
    [onAction],
  )

  const isDeleted = user.deleted_at != null

  return (
    <div className="relative">
      <button
        onClick={() => setOpen(prev => !prev)}
        className="cursor-pointer rounded p-1.5 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-neutral-800 dark:hover:text-neutral-300"
        title="Aktionen"
      >
        <MoreHorizontal className="h-4 w-4" />
      </button>

      {open && (
        <>
          {/* Click-outside overlay */}
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div className="absolute right-0 top-full z-50 mt-1 w-52 rounded-lg border border-neutral-200 bg-white py-1 shadow-lg dark:border-neutral-700 dark:bg-neutral-900">
            {!isDeleted && (
              <>
                {user.is_active ? (
                  <button
                    onClick={() => handleAction('deactivate')}
                    className="flex w-full cursor-pointer items-center gap-2 px-3 py-2 text-left text-sm text-neutral-700 hover:bg-neutral-50 dark:text-neutral-300 dark:hover:bg-neutral-800"
                  >
                    <ShieldOff className="h-4 w-4 text-amber-500" />
                    Deaktivieren
                  </button>
                ) : (
                  <button
                    onClick={() => handleAction('activate')}
                    className="flex w-full cursor-pointer items-center gap-2 px-3 py-2 text-left text-sm text-neutral-700 hover:bg-neutral-50 dark:text-neutral-300 dark:hover:bg-neutral-800"
                  >
                    <Shield className="h-4 w-4 text-green-500" />
                    Aktivieren
                  </button>
                )}
                <button
                  onClick={() => handleAction('reset-password')}
                  className="flex w-full cursor-pointer items-center gap-2 px-3 py-2 text-left text-sm text-neutral-700 hover:bg-neutral-50 dark:text-neutral-300 dark:hover:bg-neutral-800"
                >
                  <KeyRound className="h-4 w-4 text-amber-500" />
                  Passwort zurücksetzen
                </button>
              </>
            )}
            <button
              onClick={() => handleAction('sessions')}
              className="flex w-full cursor-pointer items-center gap-2 px-3 py-2 text-left text-sm text-neutral-700 hover:bg-neutral-50 dark:text-neutral-300 dark:hover:bg-neutral-800"
            >
              <Monitor className="h-4 w-4 text-blue-500" />
              Sitzungen anzeigen
            </button>
            {!isDeleted && (
              <button
                onClick={() => handleAction('revoke-sessions')}
                className="flex w-full cursor-pointer items-center gap-2 px-3 py-2 text-left text-sm text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-950/30"
              >
                <Ban className="h-4 w-4" />
                Alle Sitzungen widerrufen
              </button>
            )}
          </div>
        </>
      )}
    </div>
  )
}

// ── Main Page ───────────────────────────────────────────────────────────────

export default function AdminUsersPage() {
  const [searchInput, setSearchInput] = useState('')
  const debouncedSearch = useDebounce(searchInput, 300)
  const [statusFilter, setStatusFilter] = useState('active')
  const [offset, setOffset] = useState(0)

  // Dialogs
  const [showCreateDialog, setShowCreateDialog] = useState(false)
  const [resetPasswordUser, setResetPasswordUser] = useState<AdminUser | null>(null)
  const [sessionsUser, setSessionsUser] = useState<AdminUser | null>(null)
  const [confirmAction, setConfirmAction] = useState<{
    type: 'activate' | 'deactivate' | 'revoke-sessions'
    user: AdminUser
  } | null>(null)

  const queryClient = useQueryClient()
  const { addToast } = useToast()

  const { data, isLoading, isError } = useQuery({
    queryKey: ['admin-users', debouncedSearch, statusFilter, offset],
    queryFn: () =>
      getAdminUsers({
        search: debouncedSearch || undefined,
        status: statusFilter,
        limit: PAGE_SIZE,
        offset,
      }),
  })

  const users: AdminUser[] = data?.users ?? []
  const total: number = data?.total ?? 0
  const currentPage = Math.floor(offset / PAGE_SIZE) + 1
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  // Reset offset when filters change
  const handleSearchChange = useCallback((value: string) => {
    setSearchInput(value)
    setOffset(0)
  }, [])

  const handleStatusChange = useCallback((value: string) => {
    setStatusFilter(value)
    setOffset(0)
  }, [])

  // ── Mutations for confirm actions ──

  const activateMutation = useMutation({
    mutationFn: (id: string) => adminActivateUser(id),
    onSuccess: () => {
      addToast({ type: 'success', title: 'Benutzer aktiviert' })
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      setConfirmAction(null)
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: getAuthErrorMessage(err.message), duration: 0 })
    },
  })

  const deactivateMutation = useMutation({
    mutationFn: (id: string) => adminDeactivateUser(id),
    onSuccess: () => {
      addToast({ type: 'success', title: 'Benutzer deaktiviert' })
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      setConfirmAction(null)
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: getAuthErrorMessage(err.message), duration: 0 })
    },
  })

  const revokeSessionsMutation = useMutation({
    mutationFn: (id: string) => adminRevokeUserSessions(id),
    onSuccess: () => {
      addToast({ type: 'success', title: 'Alle Sitzungen widerrufen' })
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      setConfirmAction(null)
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: getAuthErrorMessage(err.message), duration: 0 })
    },
  })

  function handleUserAction(
    user: AdminUser,
    action: 'activate' | 'deactivate' | 'reset-password' | 'sessions' | 'revoke-sessions',
  ) {
    switch (action) {
      case 'activate':
        setConfirmAction({ type: 'activate', user })
        break
      case 'deactivate':
        setConfirmAction({ type: 'deactivate', user })
        break
      case 'reset-password':
        setResetPasswordUser(user)
        break
      case 'sessions':
        setSessionsUser(user)
        break
      case 'revoke-sessions':
        setConfirmAction({ type: 'revoke-sessions', user })
        break
    }
  }

  function handleConfirm() {
    if (!confirmAction) return
    const { type, user } = confirmAction
    switch (type) {
      case 'activate':
        activateMutation.mutate(user.id)
        break
      case 'deactivate':
        deactivateMutation.mutate(user.id)
        break
      case 'revoke-sessions':
        revokeSessionsMutation.mutate(user.id)
        break
    }
  }

  const confirmPending =
    activateMutation.isPending || deactivateMutation.isPending || revokeSessionsMutation.isPending

  const CONFIRM_CONFIG: Record<string, { title: string; message: string; label: string; cls?: string }> = {
    activate: {
      title: 'Benutzer aktivieren',
      message: `Soll "${confirmAction?.user.username}" aktiviert werden? Der Benutzer kann sich danach wieder anmelden.`,
      label: 'Aktivieren',
      cls: 'bg-green-600 hover:bg-green-700',
    },
    deactivate: {
      title: 'Benutzer deaktivieren',
      message: `Soll "${confirmAction?.user.username}" deaktiviert werden? Der Benutzer kann sich danach nicht mehr anmelden.`,
      label: 'Deaktivieren',
    },
    'revoke-sessions': {
      title: 'Alle Sitzungen widerrufen',
      message: `Alle aktiven Sitzungen von "${confirmAction?.user.username}" werden sofort beendet. Der Benutzer muss sich neu anmelden.`,
      label: 'Widerrufen',
    },
  }

  return (
    <div className="p-6 space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Benutzerverwaltung</h1>
        <button
          onClick={() => setShowCreateDialog(true)}
          className="flex cursor-pointer items-center gap-2 rounded-lg bg-accent-DEFAULT px-4 py-2 text-sm font-medium text-white hover:bg-accent-600 transition-colors"
        >
          <Plus className="h-4 w-4" />
          Benutzer erstellen
        </button>
      </div>

      {/* Filter bar */}
      <div className="flex flex-wrap items-end gap-3 rounded-lg border border-neutral-200 bg-neutral-50 p-4 dark:border-neutral-700 dark:bg-neutral-900">
        {/* Search */}
        <div className="flex flex-col gap-1 flex-1 min-w-[200px]">
          <label className="text-xs font-medium text-neutral-500">Suche</label>
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-neutral-400" />
            <input
              type="text"
              value={searchInput}
              onChange={e => handleSearchChange(e.target.value)}
              placeholder="Name, E-Mail oder ID..."
              className="h-9 w-full rounded border border-neutral-300 bg-white pl-9 pr-3 text-sm dark:border-neutral-600 dark:bg-neutral-800"
            />
          </div>
        </div>

        {/* Status filter */}
        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-neutral-500">Status</label>
          <select
            value={statusFilter}
            onChange={e => handleStatusChange(e.target.value)}
            className="h-9 rounded border border-neutral-300 bg-white px-2 text-sm dark:border-neutral-600 dark:bg-neutral-800"
          >
            <option value="active">{STATUS_LABELS.active}</option>
            <option value="inactive">{STATUS_LABELS.inactive}</option>
            <option value="deleted">{STATUS_LABELS.deleted}</option>
            <option value="all">{STATUS_LABELS.all}</option>
          </select>
        </div>
      </div>

      {/* Table */}
      <div className="overflow-x-auto rounded-lg border border-neutral-200 dark:border-neutral-700">
        <table className="w-full text-sm">
          <thead className="bg-neutral-50 dark:bg-neutral-900">
            <tr>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">Benutzer</th>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">E-Mail</th>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">Status</th>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">Quelle</th>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">Letzter Login</th>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">Erstellt</th>
              <th className="px-4 py-3 text-right font-medium text-neutral-500">Aktionen</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
            {isLoading && (
              <tr>
                <td colSpan={7} className="px-4 py-8 text-center text-neutral-500">
                  <div className="flex items-center justify-center gap-2">
                    <Loader2 className="h-4 w-4 animate-spin" />
                    Lade Benutzer...
                  </div>
                </td>
              </tr>
            )}
            {isError && (
              <tr>
                <td colSpan={7} className="px-4 py-8 text-center text-red-500">
                  Fehler beim Laden der Benutzer.
                </td>
              </tr>
            )}
            {!isLoading && !isError && users.length === 0 && (
              <tr>
                <td colSpan={7} className="px-4 py-8 text-center text-neutral-500">
                  Keine Benutzer gefunden.
                </td>
              </tr>
            )}
            {users.map(user => {
              const isDeleted = user.deleted_at != null
              return (
                <tr
                  key={user.id}
                  className={`hover:bg-neutral-50 dark:hover:bg-neutral-800/50 ${isDeleted ? 'opacity-60' : ''}`}
                >
                  {/* Username + Admin badge */}
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2">
                      <span className={`font-medium text-neutral-800 dark:text-neutral-200 ${isDeleted ? 'line-through' : ''}`}>
                        {user.username}
                      </span>
                      {user.is_admin && (
                        <span className="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wide bg-violet-500/10 text-violet-600 dark:text-violet-400">
                          Admin
                        </span>
                      )}
                      {!user.email_verified && !isDeleted && (
                        <span
                          className="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium bg-amber-500/10 text-amber-600 dark:text-amber-400"
                          title="E-Mail nicht verifiziert"
                        >
                          Unverifiziert
                        </span>
                      )}
                    </div>
                    {user.display_name && user.display_name !== user.username && (
                      <div className={`text-xs text-neutral-500 ${isDeleted ? 'line-through' : ''}`}>
                        {user.display_name}
                      </div>
                    )}
                  </td>

                  {/* Email */}
                  <td className={`px-4 py-3 text-neutral-600 dark:text-neutral-400 ${isDeleted ? 'line-through' : ''}`}>
                    {user.email}
                  </td>

                  {/* Status */}
                  <td className="px-4 py-3">
                    <StatusBadge user={user} />
                  </td>

                  {/* Registration source */}
                  <td className="px-4 py-3">
                    <SourceBadge source={user.registration_source} />
                  </td>

                  {/* Last login */}
                  <td className="whitespace-nowrap px-4 py-3 text-xs text-neutral-600 dark:text-neutral-400">
                    {formatTimestamp(user.last_login_at)}
                  </td>

                  {/* Created + deleted info */}
                  <td className="whitespace-nowrap px-4 py-3 text-xs text-neutral-600 dark:text-neutral-400">
                    {formatTimestamp(user.created_at)}
                    {isDeleted && (
                      <div className="text-red-500 mt-0.5">
                        Gelöscht: {formatTimestamp(user.deleted_at)}
                      </div>
                    )}
                  </td>

                  {/* Actions */}
                  <td className="px-4 py-3 text-right">
                    <UserActionsMenu
                      user={user}
                      onAction={action => handleUserAction(user, action)}
                    />
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      <div className="flex items-center justify-between text-sm text-neutral-600 dark:text-neutral-400">
        <span>
          {total > 0
            ? `${offset + 1}–${Math.min(offset + PAGE_SIZE, total)} von ${total} Benutzern`
            : 'Keine Benutzer'}
        </span>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
            disabled={offset === 0}
            className="cursor-pointer rounded-lg border border-neutral-300 px-3 py-1 hover:bg-neutral-100 disabled:cursor-not-allowed disabled:opacity-40 dark:border-neutral-600 dark:hover:bg-neutral-800"
          >
            Zurück
          </button>
          <span>
            Seite {currentPage} / {totalPages}
          </span>
          <button
            onClick={() => setOffset(offset + PAGE_SIZE)}
            disabled={offset + PAGE_SIZE >= total}
            className="cursor-pointer rounded-lg border border-neutral-300 px-3 py-1 hover:bg-neutral-100 disabled:cursor-not-allowed disabled:opacity-40 dark:border-neutral-600 dark:hover:bg-neutral-800"
          >
            Weiter
          </button>
        </div>
      </div>

      {/* ── Dialogs ── */}

      {showCreateDialog && (
        <CreateUserDialog
          onClose={() => setShowCreateDialog(false)}
          onCreated={() => queryClient.invalidateQueries({ queryKey: ['admin-users'] })}
        />
      )}

      {resetPasswordUser && (
        <ResetPasswordDialog
          user={resetPasswordUser}
          onClose={() => setResetPasswordUser(null)}
        />
      )}

      {sessionsUser && (
        <SessionsDialog
          user={sessionsUser}
          onClose={() => setSessionsUser(null)}
        />
      )}

      {confirmAction && (
        <ConfirmDialog
          title={CONFIRM_CONFIG[confirmAction.type].title}
          message={CONFIRM_CONFIG[confirmAction.type].message}
          confirmLabel={CONFIRM_CONFIG[confirmAction.type].label}
          confirmClass={CONFIRM_CONFIG[confirmAction.type].cls}
          isPending={confirmPending}
          onConfirm={handleConfirm}
          onClose={() => setConfirmAction(null)}
        />
      )}
    </div>
  )
}
