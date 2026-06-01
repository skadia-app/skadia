import { useState, useCallback } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  getAdminUsers,
  getAuthConfig,
  adminActivateUser,
  adminDeactivateUser,
  adminResetPassword,
  adminCreateUser,
  getAdminUserSessions,
  adminRevokeUserSessions,
  adminSetUserAdmin,
  adminDeleteUser,
  adminSetUserHotfolder,
  getMe,
  type AdminUser,
  type AdminUserSession,
} from '@/api/auth'
import { getAdminSettings } from '@/api/adminSettings'
import {
  getAdminUserLibraryDir,
  adminSetUserLibraryDir,
  adminRenameUser,
  API_STATUS,
} from '@/api/shadowfs'
import { getAuthErrorMessage } from '@/pages/auth/errorMessages'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'
import { fmtDateTime, parseUserAgent } from '@/lib/format'
import { useDebounce } from '@/lib/useDebounce'
import { useToast } from '@/lib/toast'
import { i18n } from '@/i18n/i18n'
import { ModalShell } from '@/components/ui/ModalShell'
import ConfirmModal from '@/components/ConfirmModal'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu'
import {
  Search,
  Plus,
  MoreHorizontal,
  Shield,
  ShieldOff,
  ShieldCheck,
  ShieldX,
  KeyRound,
  Monitor,
  Ban,
  Loader2,
  FolderOpen,
  FolderX,
  FolderEdit,
  Trash2,
  RefreshCw,
} from 'lucide-react'

const PAGE_SIZE = 50

// ── Helpers ─────────────────────────────────────────────────────────────────

const fmtTs = (iso: string | null): string => (iso ? fmtDateTime(iso) : '—')
const REGISTRATION_SOURCE_KEYS = new Set(['admin', 'registration', 'invite', 'oauth_google', 'oauth_github'])

function getAdminErrorMessage(code: string, t: TFunction): string {
  const key = `adminUsers.errors.${code}`
  if (i18n.exists(key)) return t(key)
  return getAuthErrorMessage(code)
}

function StatusBadge({ user }: { user: AdminUser }) {
  const { t } = useTranslation()
  if (user.deleted_at) {
    return (
      <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-red-500/10 text-red-600 dark:text-red-400">
        {t('adminUsers.status.deleted')}
      </span>
    )
  }
  if (!user.is_active && user.pending_approval) {
    return (
      <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-blue-500/10 text-blue-600 dark:text-blue-400">
        {t('adminUsers.status.pending')}
      </span>
    )
  }
  if (!user.is_active) {
    return (
      <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-amber-500/10 text-amber-600 dark:text-amber-400">
        {t('adminUsers.status.inactive')}
      </span>
    )
  }
  return (
    <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-green-500/10 text-green-600 dark:text-green-400">
      {t('adminUsers.status.active')}
    </span>
  )
}

function SourceBadge({ source }: { source: string }) {
  const { t } = useTranslation()
  const label = REGISTRATION_SOURCE_KEYS.has(source)
    ? t(`adminUsers.registrationSources.${source}`)
    : t('adminUsers.registrationSources.unknown')
  return (
    <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-blue-500/10 text-blue-600 dark:text-blue-400">
      {label}
    </span>
  )
}

// ── Create User Dialog ──────────────────────────────────────────────────────

function CreateUserDialog({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const { t } = useTranslation()
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
        addToast({ type: 'success', title: t('adminUsers.create.toasts.created') })
      } else if (data.invite_sent) {
        addToast({ type: 'success', title: t('adminUsers.create.toasts.inviteSent') })
      } else {
        addToast({ type: 'success', title: t('adminUsers.create.toasts.created') })
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
        <h2 className="text-lg font-semibold">{t('adminUsers.create.title')}</h2>

        <div className="space-y-3">
          <div className="flex flex-col gap-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('adminUsers.create.email')} *</label>
            <input
              type="email"
              required
              value={email}
              onChange={e => setEmail(e.target.value)}
              className={AUTH_INPUT_CLASS}
              placeholder={t('adminUsers.create.emailPlaceholder')}
            />
          </div>

          <div className="flex flex-col gap-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('adminUsers.create.username')} *</label>
            <input
              type="text"
              required
              value={username}
              onChange={e => setUsername(e.target.value)}
              className={AUTH_INPUT_CLASS}
              placeholder={t('adminUsers.create.usernamePlaceholder')}
            />
          </div>

          <div className="flex flex-col gap-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              {smtpConfigured ? t('adminUsers.create.passwordOptional') : t('adminUsers.create.passwordRequired')}
            </label>
            <input
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              className={AUTH_INPUT_CLASS}
              placeholder={smtpConfigured ? t('adminUsers.create.passwordInvitePlaceholder') : t('adminUsers.create.passwordRequiredPlaceholder')}
              required={!smtpConfigured}
              minLength={password ? 10 : undefined}
            />
            {password && password.length > 0 && password.length < 10 && (
              <p className="text-xs text-rose-600 dark:text-rose-400">{t('adminUsers.passwordMinLength')}</p>
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
            {t('common.cancel')}
          </button>
          <button
            type="submit"
            disabled={mutation.isPending || !email || !username || (password.length > 0 && password.length < 10)}
            className="flex cursor-pointer items-center gap-2 rounded-lg bg-accent-DEFAULT px-4 py-2 text-sm font-medium text-white hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {mutation.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
            {t('adminUsers.create.submit')}
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
  const { t } = useTranslation()
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
        addToast({ type: 'success', title: t('adminUsers.resetPassword.toasts.emailSent') })
      } else {
        addToast({ type: 'success', title: t('adminUsers.resetPassword.toasts.passwordSet') })
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
        <h2 className="text-lg font-semibold">{t('adminUsers.resetPassword.title')}</h2>
        <p className="text-sm text-neutral-600 dark:text-neutral-400">
          {t('adminUsers.resetPassword.description', { username: user.username })}
        </p>

        <div className="space-y-2">
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="radio"
              checked={mode === 'set'}
              onChange={() => setMode('set')}
              className="h-4 w-4"
            />
            <span className="text-sm">{t('adminUsers.resetPassword.setPassword')}</span>
          </label>
          {mode === 'set' && (
            <input
              type="password"
              value={newPassword}
              onChange={e => setNewPassword(e.target.value)}
              placeholder={t('adminUsers.resetPassword.newPasswordPlaceholder')}
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
            <span className="text-sm">{t('adminUsers.resetPassword.sendEmail')}</span>
          </label>
        </div>

        <div className="flex justify-end gap-2 pt-2">
          <button
            type="button"
            onClick={onClose}
            disabled={mutation.isPending}
            className="cursor-pointer rounded-lg border border-neutral-300 px-4 py-2 text-sm text-neutral-600 hover:bg-neutral-100 dark:border-neutral-600 dark:text-neutral-400 dark:hover:bg-neutral-800 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {t('common.cancel')}
          </button>
          <button
            type="submit"
            disabled={mutation.isPending || (mode === 'set' && newPassword.length < 10)}
            className="flex cursor-pointer items-center gap-2 rounded-lg bg-amber-600 px-4 py-2 text-sm font-medium text-white hover:bg-amber-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {mutation.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
            {t('adminUsers.resetPassword.submit')}
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
  const { t } = useTranslation()
  const { addToast } = useToast()
  const queryClient = useQueryClient()

  const { data, isLoading, isError } = useQuery({
    queryKey: ['admin-user-sessions', user.id],
    queryFn: () => getAdminUserSessions(user.id),
  })

  const revokeMutation = useMutation({
    mutationFn: () => adminRevokeUserSessions(user.id),
    onSuccess: () => {
      addToast({ type: 'success', title: t('adminUsers.sessions.toasts.revoked') })
      queryClient.invalidateQueries({ queryKey: ['admin-user-sessions', user.id] })
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: getAuthErrorMessage(err.message), duration: 0 })
    },
  })

  const sessions: AdminUserSession[] = data?.sessions ?? []
  const activeSessions = sessions.filter(s => !s.revoked && new Date(s.expires_at) > new Date())

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-xl">
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold">
            {t('adminUsers.sessions.title', { username: user.username })}
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
              {t('adminUsers.sessions.revokeAll')}
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
            {t('adminUsers.sessions.loadError')}
          </p>
        )}

        {!isLoading && !isError && sessions.length === 0 && (
          <p className="py-4 text-center text-sm text-neutral-500">
            {t('adminUsers.sessions.empty')}
          </p>
        )}

        {!isLoading && !isError && sessions.length > 0 && (
          <div className="overflow-x-auto rounded-lg border border-neutral-200 dark:border-neutral-700">
            <table className="w-full text-sm">
              <thead className="bg-neutral-50 dark:bg-neutral-900">
                <tr>
                  <th className="px-3 py-2 text-left font-medium text-neutral-500">{t('adminUsers.sessions.browser')}</th>
                  <th className="px-3 py-2 text-left font-medium text-neutral-500">IP</th>
                  <th className="px-3 py-2 text-left font-medium text-neutral-500">{t('adminUsers.table.created')}</th>
                  <th className="px-3 py-2 text-left font-medium text-neutral-500">{t('adminUsers.sessions.expires')}</th>
                  <th className="px-3 py-2 text-left font-medium text-neutral-500">{t('adminUsers.table.status')}</th>
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
                        {fmtTs(session.created_at)}
                      </td>
                      <td className="whitespace-nowrap px-3 py-2 text-xs text-neutral-600 dark:text-neutral-400">
                        {fmtTs(session.expires_at)}
                      </td>
                      <td className="px-3 py-2">
                        {session.revoked ? (
                          <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-red-500/10 text-red-600 dark:text-red-400">
                            {t('adminUsers.sessions.revoked')}
                          </span>
                        ) : expired ? (
                          <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-neutral-500/10 text-neutral-600 dark:text-neutral-400">
                            {t('adminUsers.sessions.expired')}
                          </span>
                        ) : (
                          <span className="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-green-500/10 text-green-600 dark:text-green-400">
                            {t('common.active')}
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
            {t('common.close')}
          </button>
        </div>
      </div>
    </ModalShell>
  )
}

// ── Delete User Dialog (two-step confirmation) ──────────────────────────────

function DeleteUserDialog({
  user,
  onClose,
  onDeleted,
}: {
  user: AdminUser
  onClose: () => void
  onDeleted: () => void
}) {
  const { t } = useTranslation()
  const [step, setStep] = useState<1 | 2>(1)
  const [confirmInput, setConfirmInput] = useState('')
  const { addToast } = useToast()

  const mutation = useMutation({
    mutationFn: () => adminDeleteUser(user.id, user.username),
    onSuccess: () => {
      addToast({ type: 'success', title: t('adminUsers.delete.toasts.deleted', { username: user.username }) })
      onDeleted()
      onClose()
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: getAdminErrorMessage(err.message, t), duration: 0 })
    },
  })

  const canConfirm = confirmInput.trim().toLowerCase() === user.username.toLowerCase() && !mutation.isPending

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-sm">
      <div className="space-y-4">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-red-100 dark:bg-red-900/30">
            <Trash2 className="h-5 w-5 text-red-600 dark:text-red-400" />
          </div>
          <h2 className="text-lg font-semibold">{t('adminUsers.delete.title')}</h2>
        </div>

        {step === 1 && (
          <>
            <div className="rounded-lg border border-red-200 bg-red-50 p-3 dark:border-red-800/40 dark:bg-red-900/10">
              <p className="text-sm font-medium text-red-700 dark:text-red-400">
                {t('adminUsers.delete.warningTitle')}
              </p>
              <ul className="mt-2 space-y-1 text-sm text-red-600 dark:text-red-300">
                <li>{t('adminUsers.delete.warningUser', { username: user.username })}</li>
                <li>{t('adminUsers.delete.warningSessions')}</li>
                <li>{t('adminUsers.delete.warningData')}</li>
              </ul>
            </div>
            <div className="flex justify-end gap-2 pt-1">
              <button
                onClick={onClose}
                className="cursor-pointer rounded-lg border border-neutral-300 px-4 py-2 text-sm text-neutral-600 hover:bg-neutral-100 dark:border-neutral-600 dark:text-neutral-400 dark:hover:bg-neutral-800"
              >
                {t('common.cancel')}
              </button>
              <button
                onClick={() => setStep(2)}
                className="cursor-pointer rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700"
              >
                {t('common.next')}
              </button>
            </div>
          </>
        )}

        {step === 2 && (
          <>
            <p className="text-sm text-neutral-600 dark:text-neutral-400">
              {t('adminUsers.delete.confirmPrompt', { username: user.username })}
            </p>
            <input
              type="text"
              value={confirmInput}
              onChange={e => setConfirmInput(e.target.value)}
              className={AUTH_INPUT_CLASS}
              placeholder={user.username}
              autoFocus
              onKeyDown={e => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                }
              }}
            />
            <div className="flex justify-end gap-2 pt-1">
              <button
                onClick={() => { setStep(1); setConfirmInput('') }}
                disabled={mutation.isPending}
                className="cursor-pointer rounded-lg border border-neutral-300 px-4 py-2 text-sm text-neutral-600 hover:bg-neutral-100 dark:border-neutral-600 dark:text-neutral-400 dark:hover:bg-neutral-800 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {t('common.back')}
              </button>
              <button
                onClick={() => mutation.mutate()}
                disabled={!canConfirm}
                className="flex cursor-pointer items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {mutation.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
                {t('adminUsers.delete.confirm')}
              </button>
            </div>
          </>
        )}
      </div>
    </ModalShell>
  )
}

// ── Library-Dir Labels (admin context) ─────────────────────────────────────

const FILE_TYPES_FOR_RENAME = ['activity', 'media', 'document', 'health', 'route'] as const

// ── UserLibraryDirDialog ────────────────────────────────────────────────────

function UserLibraryDirDialog({
  user,
  onClose,
}: {
  user: AdminUser
  onClose: () => void
}) {
  const { t } = useTranslation()
  const { addToast } = useToast()
  const queryClient = useQueryClient()

  const { data, isLoading, isError } = useQuery({
    queryKey: ['admin-user-library-dir', user.id],
    queryFn: () => getAdminUserLibraryDir(user.id),
  })

  const [inputValue, setInputValue] = useState<string | null>(null)
  const current = inputValue ?? data?.library_dir_name ?? ''

  const setDirMutation = useMutation({
    mutationFn: (name: string | null) => adminSetUserLibraryDir(user.id, name),
    onSuccess: (res) => {
      if (res.status === API_STATUS.libraryDirNameTaken) {
        addToast({ type: 'warning', title: t('libraryLabels.labels.toastLibraryDirTaken'), duration: 0 })
        return
      }
      if (res.status === API_STATUS.libraryDirNameClashesUUID) {
        addToast({ type: 'warning', title: t('libraryLabels.labels.toastLibraryDirClashesUUID'), duration: 0 })
        return
      }
      queryClient.invalidateQueries({ queryKey: ['admin-user-library-dir', user.id] })
      setInputValue(null)
      addToast({ type: 'success', title: t('libraryLabels.labels.toastSaved') })
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: t('common.error'), message: err.message, duration: 0 })
    },
  })

  const renameMutation = useMutation({
    mutationFn: (fileType: string) => adminRenameUser(user.id, fileType),
    onSuccess: () => {
      addToast({ type: 'success', title: t('libraryLabels.labels.toastRenameStarted') })
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: t('common.error'), message: err.message, duration: 0 })
    },
  })

  function handleSave() {
    const trimmed = current.trim()
    setDirMutation.mutate(trimmed || null)
  }

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-md">
      <div className="space-y-5">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-blue-500/10">
            <FolderEdit className="h-5 w-5 text-blue-600 dark:text-blue-400" />
          </div>
          <div>
            <h2 className="text-lg font-semibold">{t('libraryLabels.labels.editLibraryDirMenuItem')}</h2>
            <p className="text-xs text-neutral-500 dark:text-neutral-400">{user.username}</p>
          </div>
        </div>

        {isLoading && (
          <div className="flex justify-center py-4">
            <Loader2 className="h-5 w-5 animate-spin text-neutral-400" />
          </div>
        )}

        {isError && (
          <p className="py-2 text-sm text-red-500">{t('libraryLabels.labels.loadLibraryDirErrorTitle')}</p>
        )}

        {!isLoading && !isError && data && (
          <>
            {/* Library-Dir-Name edit */}
            <div className="space-y-2">
              <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
                {t('libraryLabels.labels.libraryDirPrefix')}
              </label>
              <div className="flex gap-2">
                <input
                  type="text"
                  value={current}
                  onChange={e => setInputValue(e.target.value)}
                  placeholder={user.id}
                  className={`${AUTH_INPUT_CLASS} flex-1`}
                  disabled={setDirMutation.isPending}
                />
                <button
                  onClick={handleSave}
                  disabled={setDirMutation.isPending}
                  className="flex shrink-0 cursor-pointer items-center gap-1.5 rounded-lg bg-accent-DEFAULT px-3 py-2 text-sm font-medium text-white hover:bg-accent-600 disabled:cursor-not-allowed disabled:opacity-50"
                >
                  {setDirMutation.isPending ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : null}
                  {t('common.save')}
                </button>
              </div>
              <p className="text-xs text-neutral-500 dark:text-neutral-400">
                {t('libraryLabels.labels.effectiveLabel')}{' '}
                <code className="rounded bg-neutral-100 px-1 py-0.5 font-mono text-xs dark:bg-neutral-800">
                  {inputValue != null && inputValue.trim() ? inputValue.trim() : data.effective_name}
                </code>
              </p>
              {!current.trim() && data.library_dir_name && (
                <p className="text-xs text-amber-600 dark:text-amber-400">
                  {t('libraryLabels.labels.resetToUUIDHint')}
                </p>
              )}
            </div>

            {/* Rename buttons per file type */}
            <div className="space-y-2">
              <p className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
                {t('libraryLabels.labels.executeRenameButton')}
              </p>
              <div className="flex flex-wrap gap-2">
                {FILE_TYPES_FOR_RENAME.map(ft => (
                  <button
                    key={ft}
                    onClick={() => renameMutation.mutate(ft)}
                    disabled={renameMutation.isPending}
                    className="flex cursor-pointer items-center gap-1.5 rounded border border-neutral-300 px-3 py-1.5 text-xs font-medium text-neutral-700 hover:bg-neutral-100 dark:border-neutral-600 dark:text-neutral-300 dark:hover:bg-neutral-800 disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    {renameMutation.isPending ? (
                      <Loader2 className="h-3 w-3 animate-spin" />
                    ) : (
                      <RefreshCw className="h-3 w-3" />
                    )}
                    {t(`fileTypes.${ft}`)}
                  </button>
                ))}
              </div>
              <p className="text-xs text-neutral-500 dark:text-neutral-400">
                {t('libraryLabels.labels.renameJobBackgroundHint')}
              </p>
            </div>
          </>
        )}

        <div className="flex justify-end border-t border-neutral-100 pt-3 dark:border-neutral-800">
          <button
            onClick={onClose}
            className="cursor-pointer rounded-lg border border-neutral-300 px-4 py-2 text-sm text-neutral-600 hover:bg-neutral-100 dark:border-neutral-600 dark:text-neutral-400 dark:hover:bg-neutral-800"
          >
            {t('common.close')}
          </button>
        </div>
      </div>
    </ModalShell>
  )
}

// ── User Actions Menu ───────────────────────────────────────────────────────

function UserActionsMenu({
  user,
  currentUserId,
  hotfolderGlobalMode,
  onAction,
}: {
  user: AdminUser
  currentUserId: string
  hotfolderGlobalMode: string
  onAction: (action: 'activate' | 'deactivate' | 'reset-password' | 'sessions' | 'revoke-sessions' | 'set-admin' | 'delete' | 'toggle-hotfolder' | 'library-dir') => void
}) {
  const { t } = useTranslation()
  const isDeleted = user.deleted_at != null
  const isSelf = user.id === currentUserId
  const hotfolderAvailable = hotfolderGlobalMode !== 'disabled'

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          className="cursor-pointer rounded p-1.5 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-neutral-800 dark:hover:text-neutral-300"
          title={t('adminUsers.table.actions')}
        >
          <MoreHorizontal className="h-4 w-4" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        {!isDeleted && (
          <>
            {user.is_active ? (
              <DropdownMenuItem onClick={() => onAction('deactivate')} className="gap-2">
                <ShieldOff className="h-4 w-4 text-amber-500" />
                {t('adminUsers.actions.deactivate')}
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem onClick={() => onAction('activate')} className="gap-2">
                <Shield className="h-4 w-4 text-green-500" />
                {t('adminUsers.actions.activate')}
              </DropdownMenuItem>
            )}

            {/* Promote / Demote admin */}
            {user.is_admin ? (
              <DropdownMenuItem
                onClick={() => !isSelf && onAction('set-admin')}
                disabled={isSelf}
                className="gap-2"
                title={isSelf ? t('adminUsers.errors.cannot_demote_self') : undefined}
              >
                <ShieldX className="h-4 w-4 text-violet-500" />
                <span className={isSelf ? 'opacity-50' : ''}>{t('adminUsers.actions.demoteFromAdmin')}</span>
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem onClick={() => onAction('set-admin')} className="gap-2">
                <ShieldCheck className="h-4 w-4 text-violet-500" />
                {t('adminUsers.actions.promoteToAdmin')}
              </DropdownMenuItem>
            )}

            {/* Hotfolder toggle — only if global hotfolder mode is not disabled */}
            {hotfolderAvailable && (
              <DropdownMenuItem onClick={() => onAction('toggle-hotfolder')} className="gap-2">
                {user.hotfolder_enabled ? (
                  <>
                    <FolderX className="h-4 w-4 text-orange-500" />
                    {t('adminUsers.actions.disableHotfolder')}
                  </>
                ) : (
                  <>
                    <FolderOpen className="h-4 w-4 text-orange-500" />
                    {t('adminUsers.actions.enableHotfolder')}
                  </>
                )}
              </DropdownMenuItem>
            )}

            <DropdownMenuItem onClick={() => onAction('reset-password')} className="gap-2">
              <KeyRound className="h-4 w-4 text-amber-500" />
              {t('adminUsers.actions.resetPassword')}
            </DropdownMenuItem>

            <DropdownMenuItem onClick={() => onAction('library-dir')} className="gap-2">
              <FolderEdit className="h-4 w-4 text-blue-500" />
              {t('libraryLabels.labels.editLibraryDirMenuItem')}
            </DropdownMenuItem>
          </>
        )}

        <DropdownMenuItem onClick={() => onAction('sessions')} className="gap-2">
          <Monitor className="h-4 w-4 text-blue-500" />
          {t('adminUsers.actions.showSessions')}
        </DropdownMenuItem>

        {!isDeleted && (
          <DropdownMenuItem
            onClick={() => onAction('revoke-sessions')}
            className="gap-2 text-red-600 focus:text-red-600 dark:text-red-400 dark:focus:text-red-400"
          >
            <Ban className="h-4 w-4" />
            {t('adminUsers.actions.revokeSessions')}
          </DropdownMenuItem>
        )}

        {!isDeleted && !isSelf && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={() => onAction('delete')}
              className="gap-2 text-red-600 focus:text-red-600 dark:text-red-400 dark:focus:text-red-400"
            >
              <Trash2 className="h-4 w-4" />
              {t('adminUsers.actions.delete')}
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// ── Main Page ───────────────────────────────────────────────────────────────

export default function AdminUsersPage() {
  const { t } = useTranslation()
  const [searchInput, setSearchInput] = useState('')
  const debouncedSearch = useDebounce(searchInput, 300)
  const [statusFilter, setStatusFilter] = useState('active')
  const [offset, setOffset] = useState(0)

  // Dialogs
  const [showCreateDialog, setShowCreateDialog] = useState(false)
  const [resetPasswordUser, setResetPasswordUser] = useState<AdminUser | null>(null)
  const [sessionsUser, setSessionsUser] = useState<AdminUser | null>(null)
  const [deleteUser, setDeleteUser] = useState<AdminUser | null>(null)
  const [libraryDirUser, setLibraryDirUser] = useState<AdminUser | null>(null)
  const [confirmAction, setConfirmAction] = useState<{
    type: 'activate' | 'deactivate' | 'revoke-sessions' | 'set-admin' | 'toggle-hotfolder'
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

  // Current user (to detect self)
  const { data: me } = useQuery({
    queryKey: ['me'],
    queryFn: getMe,
    staleTime: 60_000,
  })
  const currentUserId = me?.user_id ?? ''

  // Global hotfolder mode (to conditionally show hotfolder toggle)
  const { data: adminSettings } = useQuery({
    queryKey: ['admin-settings'],
    queryFn: getAdminSettings,
    staleTime: 60_000,
  })
  const hotfolderGlobalMode = (adminSettings?.hotfolder as Record<string, { value: string }> | undefined)?.mode?.value ?? 'disabled'

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

  // ── Mutations ──

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  function useConfirmMutation(fn: (id: string) => Promise<any>, title: string) {
    return useMutation({
      mutationFn: fn,
      onSuccess: () => {
        addToast({ type: 'success', title })
        queryClient.invalidateQueries({ queryKey: ['admin-users'] })
        setConfirmAction(null)
      },
      onError: (err: Error) => {
        addToast({ type: 'error', title: getAdminErrorMessage(err.message, t), duration: 0 })
        setConfirmAction(null)
      },
    })
  }

  const activateMutation = useConfirmMutation(adminActivateUser, t('adminUsers.toasts.activated'))
  const deactivateMutation = useConfirmMutation(adminDeactivateUser, t('adminUsers.toasts.deactivated'))
  const revokeSessionsMutation = useConfirmMutation(adminRevokeUserSessions, t('adminUsers.sessions.toasts.revoked'))

  const setAdminMutation = useMutation({
    mutationFn: ({ id, isAdmin }: { id: string; isAdmin: boolean }) => adminSetUserAdmin(id, isAdmin),
    onSuccess: (_data, vars) => {
      addToast({
        type: 'success',
        title: vars.isAdmin ? t('adminUsers.toasts.adminGranted') : t('adminUsers.toasts.adminRevoked'),
      })
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      setConfirmAction(null)
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: getAdminErrorMessage(err.message, t), duration: 0 })
      setConfirmAction(null)
    },
  })

  const hotfolderMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => adminSetUserHotfolder(id, enabled),
    onSuccess: (_data, vars) => {
      addToast({
        type: 'success',
        title: vars.enabled ? t('adminUsers.toasts.hotfolderEnabled') : t('adminUsers.toasts.hotfolderDisabled'),
      })
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      setConfirmAction(null)
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: getAdminErrorMessage(err.message, t), duration: 0 })
      setConfirmAction(null)
    },
  })

  function handleUserAction(
    user: AdminUser,
    action: 'activate' | 'deactivate' | 'reset-password' | 'sessions' | 'revoke-sessions' | 'set-admin' | 'delete' | 'toggle-hotfolder' | 'library-dir',
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
      case 'set-admin':
        setConfirmAction({ type: 'set-admin', user })
        break
      case 'toggle-hotfolder':
        setConfirmAction({ type: 'toggle-hotfolder', user })
        break
      case 'delete':
        setDeleteUser(user)
        break
      case 'library-dir':
        setLibraryDirUser(user)
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
      case 'set-admin':
        setAdminMutation.mutate({ id: user.id, isAdmin: !user.is_admin })
        break
      case 'toggle-hotfolder':
        hotfolderMutation.mutate({ id: user.id, enabled: !user.hotfolder_enabled })
        break
    }
  }

  const confirmPending =
    activateMutation.isPending ||
    deactivateMutation.isPending ||
    revokeSessionsMutation.isPending ||
    setAdminMutation.isPending ||
    hotfolderMutation.isPending

  const CONFIRM_CONFIG: Record<string, { title: string; message: string; label: string; cls?: string }> = {
    activate: {
      title: t('adminUsers.confirm.activate.title'),
      message: t('adminUsers.confirm.activate.message', { username: confirmAction?.user.username }),
      label: t('adminUsers.actions.activate'),
      cls: 'bg-green-600 hover:bg-green-700',
    },
    deactivate: {
      title: t('adminUsers.confirm.deactivate.title'),
      message: t('adminUsers.confirm.deactivate.message', { username: confirmAction?.user.username }),
      label: t('adminUsers.actions.deactivate'),
    },
    'revoke-sessions': {
      title: t('adminUsers.confirm.revokeSessions.title'),
      message: t('adminUsers.confirm.revokeSessions.message', { username: confirmAction?.user.username }),
      label: t('adminUsers.confirm.revokeSessions.label'),
    },
    'set-admin': {
      title: confirmAction?.user.is_admin ? t('adminUsers.actions.demoteFromAdmin') : t('adminUsers.actions.promoteToAdmin'),
      message: confirmAction?.user.is_admin
        ? t('adminUsers.confirm.setAdmin.demoteMessage', { username: confirmAction?.user.username })
        : t('adminUsers.confirm.setAdmin.promoteMessage', { username: confirmAction?.user.username }),
      label: confirmAction?.user.is_admin ? t('adminUsers.confirm.setAdmin.demoteLabel') : t('adminUsers.actions.promoteToAdmin'),
      cls: confirmAction?.user.is_admin ? undefined : 'bg-violet-600 hover:bg-violet-700',
    },
    'toggle-hotfolder': {
      title: confirmAction?.user.hotfolder_enabled ? t('adminUsers.actions.disableHotfolder') : t('adminUsers.actions.enableHotfolder'),
      message: confirmAction?.user.hotfolder_enabled
        ? t('adminUsers.confirm.hotfolder.disableMessage', { username: confirmAction?.user.username })
        : t('adminUsers.confirm.hotfolder.enableMessage', { username: confirmAction?.user.username }),
      label: confirmAction?.user.hotfolder_enabled ? t('adminUsers.actions.deactivate') : t('adminUsers.actions.activate'),
      cls: confirmAction?.user.hotfolder_enabled ? undefined : 'bg-orange-600 hover:bg-orange-700',
    },
  }

  return (
    <div className="space-y-4">
      {/* Filter bar */}
      <div className="flex flex-wrap items-end gap-3 rounded-lg border border-neutral-200 bg-neutral-50 p-4 dark:border-neutral-700 dark:bg-neutral-900">
        {/* Create button */}
        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-neutral-500">&nbsp;</label>
          <button
            onClick={() => setShowCreateDialog(true)}
            className="flex cursor-pointer items-center gap-2 h-9 rounded-lg bg-accent-DEFAULT px-4 text-sm font-medium text-white hover:bg-accent-600 transition-colors"
          >
            <Plus className="h-4 w-4" />
            {t('adminUsers.create.submit')}
          </button>
        </div>
        {/* Search */}
        <div className="flex flex-col gap-1 flex-1 min-w-[200px]">
          <label className="text-xs font-medium text-neutral-500">{t('adminUsers.filters.search')}</label>
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-neutral-400" />
            <input
              type="text"
              value={searchInput}
              onChange={e => handleSearchChange(e.target.value)}
              placeholder={t('adminUsers.filters.searchPlaceholder')}
              className="h-9 w-full rounded border border-neutral-300 bg-white pl-9 pr-3 text-sm dark:border-neutral-600 dark:bg-neutral-800"
            />
          </div>
        </div>

        {/* Status filter */}
        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-neutral-500">{t('adminUsers.table.status')}</label>
          <select
            value={statusFilter}
            onChange={e => handleStatusChange(e.target.value)}
            className="h-9 rounded border border-neutral-300 bg-white px-2 text-sm dark:border-neutral-600 dark:bg-neutral-800"
          >
            <option value="active">{t('adminUsers.status.active')}</option>
            <option value="inactive">{t('adminUsers.status.inactive')}</option>
            <option value="pending">{t('adminUsers.status.pending')}</option>
            <option value="deleted">{t('adminUsers.status.deleted')}</option>
            <option value="all">{t('adminUsers.status.all')}</option>
          </select>
        </div>
      </div>

      {/* Table */}
      <div className="overflow-x-auto rounded-lg border border-neutral-200 dark:border-neutral-700">
        <table className="w-full text-sm">
          <thead className="bg-neutral-50 dark:bg-neutral-900">
            <tr>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">{t('adminUsers.table.user')}</th>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">{t('adminUsers.table.email')}</th>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">{t('adminUsers.table.status')}</th>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">{t('adminUsers.table.source')}</th>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">{t('adminUsers.table.lastLogin')}</th>
              <th className="px-4 py-3 text-left font-medium text-neutral-500">{t('adminUsers.table.created')}</th>
              <th className="px-4 py-3 text-right font-medium text-neutral-500">{t('adminUsers.table.actions')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
            {isLoading && (
              <tr>
                <td colSpan={7} className="px-4 py-8 text-center text-neutral-500">
                  <div className="flex items-center justify-center gap-2">
                    <Loader2 className="h-4 w-4 animate-spin" />
                    {t('adminUsers.table.loading')}
                  </div>
                </td>
              </tr>
            )}
            {isError && (
              <tr>
                <td colSpan={7} className="px-4 py-8 text-center text-red-500">
                  {t('adminUsers.table.loadError')}
                </td>
              </tr>
            )}
            {!isLoading && !isError && users.length === 0 && (
              <tr>
                <td colSpan={7} className="px-4 py-8 text-center text-neutral-500">
                  {t('adminUsers.table.empty')}
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
                          {t('adminUsers.badges.admin')}
                        </span>
                      )}
                      {!user.email_verified && !isDeleted && (
                        <span
                          className="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium bg-amber-500/10 text-amber-600 dark:text-amber-400"
                          title={t('adminUsers.badges.emailUnverified')}
                        >
                          {t('adminUsers.badges.unverified')}
                        </span>
                      )}
                      {user.hotfolder_enabled && !isDeleted && (
                        <span
                          className="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium bg-orange-500/10 text-orange-600 dark:text-orange-400"
                          title={t('adminUsers.badges.hotfolderActive')}
                        >
                          {t('adminUsers.badges.hotfolder')}
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
                    {fmtTs(user.last_login_at)}
                  </td>

                  {/* Created + deleted info */}
                  <td className="whitespace-nowrap px-4 py-3 text-xs text-neutral-600 dark:text-neutral-400">
                    {fmtTs(user.created_at)}
                    {isDeleted && (
                      <div className="text-red-500 mt-0.5">
                        {t('adminUsers.table.deletedAt', { date: fmtTs(user.deleted_at) })}
                      </div>
                    )}
                  </td>

                  {/* Actions */}
                  <td className="px-4 py-3 text-right">
                    <UserActionsMenu
                      user={user}
                      currentUserId={currentUserId}
                      hotfolderGlobalMode={hotfolderGlobalMode}
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
            ? t('adminUsers.pagination.range', { start: offset + 1, end: Math.min(offset + PAGE_SIZE, total), total })
            : t('adminUsers.pagination.none')}
        </span>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
            disabled={offset === 0}
            className="cursor-pointer rounded-lg border border-neutral-300 px-3 py-1 hover:bg-neutral-100 disabled:cursor-not-allowed disabled:opacity-40 dark:border-neutral-600 dark:hover:bg-neutral-800"
          >
            {t('common.back')}
          </button>
          <span>
            {t('adminUsers.pagination.page', { current: currentPage, total: totalPages })}
          </span>
          <button
            onClick={() => setOffset(offset + PAGE_SIZE)}
            disabled={offset + PAGE_SIZE >= total}
            className="cursor-pointer rounded-lg border border-neutral-300 px-3 py-1 hover:bg-neutral-100 disabled:cursor-not-allowed disabled:opacity-40 dark:border-neutral-600 dark:hover:bg-neutral-800"
          >
            {t('common.next')}
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

      {deleteUser && (
        <DeleteUserDialog
          user={deleteUser}
          onClose={() => setDeleteUser(null)}
          onDeleted={() => queryClient.invalidateQueries({ queryKey: ['admin-users'] })}
        />
      )}

      {libraryDirUser && (
        <UserLibraryDirDialog
          user={libraryDirUser}
          onClose={() => setLibraryDirUser(null)}
        />
      )}

      {confirmAction && CONFIRM_CONFIG[confirmAction.type] && (
        <ConfirmModal
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
