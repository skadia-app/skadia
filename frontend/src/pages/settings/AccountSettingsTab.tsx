import { useState, useRef, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Check } from 'lucide-react'
import { useUser } from '@/providers/UserProvider'
import { useToast } from '@/lib/toast'
import { clearToken } from '@/lib/auth'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'
import { getAuthErrorMessage } from '@/pages/auth/errorMessages'
import { ModalShell } from '@/components/ui/ModalShell'
import {
  changePassword,
  changeEmail,
  updateMe,
  getSessions,
  revokeSession,
  deleteAccount,
  startDataExport, getLatestDataExport, getDataExportStatus,
  createDataExportDownloadSession,
  startDataImport, getDataImportStatus,
  createMultipartDataImport, uploadMultipartDataImportPart,
  startMultipartDataImport, cancelMultipartDataImport,
  type UserSession,
  type DataExportStatus,
  type DataImportStatus,
} from '@/api/auth'
import { triggerNativeDownload } from '@/api/client'
import { fmtDateTime, parseUserAgent } from '@/lib/format'
import type { UnitSystem } from '@/lib/units'
import { LANGUAGE_OPTIONS, type SupportedLanguage } from '@/i18n/languages'
import { SettingsSection as Section } from '@/components/ui/SettingsSection'

function getAccountErrorMessage(err: unknown): string {
  const code = err instanceof Error ? err.message.trim() : ''
  return getAuthErrorMessage(code && !code.includes(' ') ? code : 'internal_error')
}

// Profile section.

function ProfileSection() {
  const { t } = useTranslation()
  const { user } = useUser()
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const [username, setUsername] = useState(user?.username ?? '')
  const [displayName, setDisplayName] = useState(user?.display_name ?? '')

  const mutation = useMutation({
    mutationFn: updateMe,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['me'] })
      addToast({ type: 'success', title: t('account.profile.updated') })
    },
    onError: (err) => {
      addToast({ type: 'error', title: getAccountErrorMessage(err), duration: 0 })
    },
  })

  function handleSave(e: React.FormEvent) {
    e.preventDefault()
    const patch: { username?: string; display_name?: string } = {}
    if (username !== user?.username) patch.username = username
    if (displayName !== (user?.display_name ?? '')) patch.display_name = displayName
    if (Object.keys(patch).length === 0) return
    mutation.mutate(patch)
  }

  return (
    <Section title={t('account.profile.title')} description={t('account.profile.description')}>
      <form onSubmit={handleSave} className="space-y-4">
        <div className="space-y-1">
          <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {t('account.profile.username')}
          </label>
          <input
            type="text"
            value={username}
            onChange={e => setUsername(e.target.value)}
            className={AUTH_INPUT_CLASS}
            minLength={3}
            maxLength={30}
            required
          />
        </div>
        <div className="space-y-1">
          <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {t('account.profile.displayName')}
          </label>
          <input
            type="text"
            value={displayName}
            onChange={e => setDisplayName(e.target.value)}
            className={AUTH_INPUT_CLASS}
            maxLength={100}
          />
        </div>
        <button
          type="submit"
          disabled={mutation.isPending}
          className="px-4 py-2 rounded-lg bg-accent-DEFAULT text-white text-sm font-medium hover:bg-accent-600 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {mutation.isPending ? t('account.actions.saving') : t('account.actions.save')}
        </button>
      </form>
    </Section>
  )
}

function getLanguageAccessibleName(option: { label: string; nativeLabel: string }): string {
  return option.nativeLabel === option.label ? option.label : `${option.nativeLabel}, ${option.label}`
}

// Preferences section.

function PreferencesSection() {
  const { t } = useTranslation()
  const { locale, setLocale, units, setUnits } = useUser()
  const [savingLocale, setSavingLocale] = useState<SupportedLanguage | null>(null)
  const [savingUnits, setSavingUnits] = useState<UnitSystem | null>(null)
  const unitOptions: UnitSystem[] = ['metric', 'imperial']
  const unitDetailKeys = ['distanceElevation', 'weight', 'temperature', 'speed', 'pace']

  const handleLocaleChange = async (nextLocale: SupportedLanguage) => {
    if (nextLocale === locale || savingLocale) return
    setSavingLocale(nextLocale)
    try {
      await setLocale(nextLocale)
    } finally {
      setSavingLocale(null)
    }
  }

  const handleUnitChange = async (nextUnits: UnitSystem) => {
    if (nextUnits === units || savingUnits) return
    setSavingUnits(nextUnits)
    try {
      await setUnits(nextUnits)
    } finally {
      setSavingUnits(null)
    }
  }

  return (
    <Section title={t('account.preferences.title')} description={t('account.preferences.description')}>
      <div className="space-y-2">
        <p className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
          {t('account.preferences.languageLabel')}
        </p>
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-5">
          {LANGUAGE_OPTIONS.map(option => {
            const selected = option.code === locale
            return (
              <button
                key={option.code}
                type="button"
                aria-label={getLanguageAccessibleName(option)}
                aria-pressed={selected}
                disabled={savingLocale !== null}
                onClick={() => { void handleLocaleChange(option.code) }}
                className={[
                  'flex min-h-12 items-center gap-2 rounded-lg border px-3 py-2 text-left text-sm transition-colors disabled:opacity-60 disabled:cursor-wait',
                  selected
                    ? 'border-accent-DEFAULT bg-accent-DEFAULT text-white'
                    : 'border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800',
                ].join(' ')}
              >
                <span aria-hidden="true" className="text-lg leading-none">{option.flag}</span>
                <span className="min-w-0 flex-1 truncate">{option.nativeLabel}</span>
                {selected && <Check className="h-4 w-4 shrink-0" aria-hidden="true" />}
              </button>
            )
          })}
        </div>
      </div>

      <div className="space-y-2">
        <p className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
          {t('account.preferences.unitsLabel')}
        </p>
        <div className="grid gap-2 sm:grid-cols-2">
          {unitOptions.map(option => {
            const selected = option === units
            return (
              <button
                key={option}
                type="button"
                aria-pressed={selected}
                disabled={savingUnits !== null}
                onClick={() => { void handleUnitChange(option) }}
                className={[
                  'rounded-lg border px-4 py-3 text-left text-sm transition-colors disabled:opacity-60 disabled:cursor-wait',
                  selected
                    ? 'border-accent-DEFAULT bg-accent-DEFAULT text-white'
                    : 'border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800',
                ].join(' ')}
              >
                <span className="block font-medium">{t(`units.${option}`)}</span>
                <span className={['mt-1 block text-xs leading-5', selected ? 'text-white/85' : 'text-neutral-500 dark:text-neutral-400'].join(' ')}>
                  {unitDetailKeys.map(key => (
                    <span key={key} className="block">
                      {t(`units.${option}Details.${key}`)}
                    </span>
                  ))}
                </span>
              </button>
            )
          })}
        </div>
      </div>
    </Section>
  )
}

// ── Email Section ───────────────────────────────────────────────────────────

function EmailSection() {
  const { t } = useTranslation()
  const { user, hasPassword } = useUser()
  const { addToast } = useToast()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [newEmail, setNewEmail] = useState('')
  const [password, setPassword] = useState('')

  const mutation = useMutation({
    mutationFn: changeEmail,
    onSuccess: (data) => {
      setDialogOpen(false)
      setNewEmail('')
      setPassword('')
      if (data.status === 'verification_email_sent') {
        addToast({
          type: 'success',
          title: t('account.email.verificationSentTitle'),
          message: t('account.email.verificationSentMessage'),
        })
      } else {
        addToast({ type: 'success', title: t('account.email.changed') })
      }
    },
    onError: (err) => {
      addToast({ type: 'error', title: getAccountErrorMessage(err), duration: 0 })
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const data: { new_email: string; password?: string } = { new_email: newEmail }
    if (hasPassword) data.password = password
    mutation.mutate(data)
  }

  return (
    <Section title={t('account.email.title')} description={t('account.email.description')}>
      <div className="flex items-center gap-3">
        <span className="text-sm text-neutral-700 dark:text-neutral-300 font-mono">{user?.email ?? '-'}</span>
        <button
          onClick={() => setDialogOpen(true)}
          className="px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 text-sm text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
        >
          {t('account.email.change')}
        </button>
      </div>

      {dialogOpen && (
        <ModalShell onClose={() => setDialogOpen(false)}>
          <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-4">
            {t('account.email.change')}
          </h3>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-1">
              <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
                {t('account.email.newEmail')}
              </label>
              <input
                type="email"
                value={newEmail}
                onChange={e => setNewEmail(e.target.value)}
                className={AUTH_INPUT_CLASS}
                required
                autoFocus
              />
            </div>
            {hasPassword && (
              <div className="space-y-1">
                <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
                  {t('account.password.confirm')}
                </label>
                <input
                  type="password"
                  value={password}
                  onChange={e => setPassword(e.target.value)}
                  className={AUTH_INPUT_CLASS}
                  required
                  autoComplete="current-password"
                />
              </div>
            )}
            <div className="flex gap-3 justify-end">
              <button
                type="button"
                onClick={() => setDialogOpen(false)}
                className="px-4 py-2 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
              >
                {t('account.actions.cancel')}
              </button>
              <button
                type="submit"
                disabled={mutation.isPending}
                className="px-4 py-2 rounded-lg bg-accent-DEFAULT text-white text-sm font-medium hover:bg-accent-600 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {mutation.isPending ? t('account.actions.sending') : t('account.actions.change')}
              </button>
            </div>
          </form>
        </ModalShell>
      )}
    </Section>
  )
}

// ── Password Section ────────────────────────────────────────────────────────

function PasswordSection() {
  const { t } = useTranslation()
  const { hasPassword, mustChangePassword } = useUser()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const [currentPw, setCurrentPw] = useState('')
  const [newPw, setNewPw] = useState('')
  const [confirmPw, setConfirmPw] = useState('')

  const mutation = useMutation({
    mutationFn: () => changePassword(hasPassword ? currentPw : '', newPw),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: ['me'] })
      clearToken()
      navigate('/login?message=password_changed', { replace: true })
    },
    onError: (err) => {
      addToast({ type: 'error', title: getAccountErrorMessage(err), duration: 0 })
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (newPw !== confirmPw) {
      addToast({ type: 'error', title: t('account.password.mismatch'), duration: 0 })
      return
    }
    mutation.mutate()
  }

  // OAuth-only user without must_change_password — no password form needed
  if (!hasPassword && !mustChangePassword) {
    return (
      <Section title={t('account.password.title')}>
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          {t('account.password.oauthOnly')}
        </p>
      </Section>
    )
  }

  const showCurrentPw = hasPassword
  const isForceChange = mustChangePassword

  return (
    <Section title={t('account.password.title')} description={
      isForceChange && hasPassword
        ? undefined
        : isForceChange && !hasPassword
          ? t('account.password.setDescription')
          : t('account.password.description')
    }>
      {isForceChange && hasPassword && (
        <div className="p-3 rounded-lg bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-800">
          <p className="text-sm text-amber-700 dark:text-amber-400">
            {t('account.password.mustChange')}
          </p>
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-4">
        {showCurrentPw && (
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              {t('account.password.current')}
            </label>
            <input
              type="password"
              value={currentPw}
              onChange={e => setCurrentPw(e.target.value)}
              className={AUTH_INPUT_CLASS}
              required
              autoComplete="current-password"
            />
          </div>
        )}
        <div className="space-y-1">
          <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {t('account.password.new')}
          </label>
          <input
            type="password"
            value={newPw}
            onChange={e => setNewPw(e.target.value)}
            className={AUTH_INPUT_CLASS}
            required
            minLength={10}
            autoComplete="new-password"
          />
        </div>
        <div className="space-y-1">
          <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {t('account.password.confirm')}
          </label>
          <input
            type="password"
            value={confirmPw}
            onChange={e => setConfirmPw(e.target.value)}
            className={AUTH_INPUT_CLASS}
            required
            minLength={10}
            autoComplete="new-password"
          />
        </div>
        <button
          type="submit"
          disabled={mutation.isPending}
          className="px-4 py-2 rounded-lg bg-accent-DEFAULT text-white text-sm font-medium hover:bg-accent-600 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {mutation.isPending ? t('account.actions.changing') : t('account.password.change')}
        </button>
      </form>
    </Section>
  )
}

// ── Sessions Section ────────────────────────────────────────────────────────

function SessionsSection() {
  const { t } = useTranslation()
  const { addToast } = useToast()
  const queryClient = useQueryClient()
  const [revokeTarget, setRevokeTarget] = useState<string | null>(null) // family_id or 'all_others'
  const [revokingAll, setRevokingAll] = useState(false)

  const { data, isLoading } = useQuery({
    queryKey: ['my-sessions'],
    queryFn: getSessions,
  })

  const revokeMut = useMutation({
    mutationFn: (familyId: string) => revokeSession(familyId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['my-sessions'] })
      addToast({ type: 'success', title: t('account.sessions.ended') })
      setRevokeTarget(null)
    },
    onError: (err) => {
      addToast({ type: 'error', title: getAccountErrorMessage(err), duration: 0 })
      setRevokeTarget(null)
    },
  })

  async function handleRevokeAllOthers() {
    setRevokingAll(true)
    try {
      const sessions = data?.sessions ?? []
      const others = sessions.filter(s => !s.is_current)
      const results = await Promise.allSettled(others.map(s => revokeSession(s.family_id)))
      const failed = results.filter(r => r.status === 'rejected').length
      queryClient.invalidateQueries({ queryKey: ['my-sessions'] })
      if (failed > 0) {
        const succeeded = others.length - failed
        addToast({
          type: 'warning',
          title: t('account.sessions.partialEnded', { succeeded, total: others.length, count: others.length }),
          duration: 0,
        })
      } else {
        addToast({ type: 'success', title: t('account.sessions.endedCount', { count: others.length }) })
      }
      setRevokeTarget(null)
    } finally {
      setRevokingAll(false)
    }
  }

  const sessions: UserSession[] = data?.sessions ?? []
  const otherSessions = sessions.filter(s => !s.is_current)

  return (
    <Section title={t('account.sessions.title')} description={t('account.sessions.description')}>
      {isLoading ? (
        <p className="text-sm text-neutral-400">{t('account.sessions.loading')}</p>
      ) : sessions.length === 0 ? (
        <p className="text-sm text-neutral-400">{t('account.sessions.empty')}</p>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-neutral-200 dark:border-neutral-700 text-left text-neutral-500 dark:text-neutral-400">
                  <th className="py-2 pr-4 font-medium">{t('account.sessions.browser')}</th>
                  <th className="py-2 pr-4 font-medium">IP</th>
                  <th className="py-2 pr-4 font-medium">{t('account.sessions.created')}</th>
                  <th className="py-2 pr-4 font-medium">{t('account.sessions.expires')}</th>
                  <th className="py-2 font-medium"></th>
                </tr>
              </thead>
              <tbody>
                {sessions.map(s => (
                  <tr key={s.family_id} className="border-b border-neutral-100 dark:border-neutral-800/60">
                    <td className="py-2 pr-4 text-neutral-700 dark:text-neutral-300">
                      {parseUserAgent(s.user_agent)}
                      {s.is_current && (
                        <span className="ml-2 inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-green-100 dark:bg-green-900/40 text-green-700 dark:text-green-300">
                          {t('account.sessions.current')}
                        </span>
                      )}
                    </td>
                    <td className="py-2 pr-4 text-neutral-500 dark:text-neutral-400 font-mono text-xs">{s.ip_address}</td>
                    <td className="py-2 pr-4 text-neutral-500 dark:text-neutral-400">{fmtDateTime(s.created_at)}</td>
                    <td className="py-2 pr-4 text-neutral-500 dark:text-neutral-400">{fmtDateTime(s.expires_at)}</td>
                    <td className="py-2">
                      {!s.is_current && (
                        <button
                          onClick={() => setRevokeTarget(s.family_id)}
                          className="text-xs text-red-600 dark:text-red-400 hover:underline cursor-pointer"
                        >
                          {t('account.sessions.revoke')}
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {otherSessions.length > 1 && (
            <button
              onClick={() => setRevokeTarget('all_others')}
              className="px-3 py-1.5 rounded-lg border border-red-200 dark:border-red-800 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-950/30 transition-colors cursor-pointer"
            >
              {t('account.sessions.revokeOthers')}
            </button>
          )}
        </>
      )}

      {/* Confirmation dialog for single session revocation */}
      {revokeTarget && revokeTarget !== 'all_others' && (
        <ModalShell onClose={() => setRevokeTarget(null)}>
          <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-2">
            {t('account.sessions.revokeTitle')}
          </h3>
          <p className="text-sm text-neutral-600 dark:text-neutral-400 mb-4">
            {t('account.sessions.revokeMessage')}
          </p>
          <div className="flex gap-3 justify-end">
            <button
              onClick={() => setRevokeTarget(null)}
              className="px-4 py-2 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
            >
              {t('account.actions.cancel')}
            </button>
            <button
              onClick={() => revokeMut.mutate(revokeTarget)}
              disabled={revokeMut.isPending}
              className="px-4 py-2 rounded-lg bg-red-600 text-white text-sm font-medium hover:bg-red-700 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {revokeMut.isPending ? t('account.sessions.revoking') : t('account.sessions.revokeConfirm')}
            </button>
          </div>
        </ModalShell>
      )}

      {/* Confirmation dialog for revoking all other sessions */}
      {revokeTarget === 'all_others' && (
        <ModalShell onClose={() => setRevokeTarget(null)}>
          <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-2">
            {t('account.sessions.revokeOthersTitle')}
          </h3>
          <p className="text-sm text-neutral-600 dark:text-neutral-400 mb-4">
            {t('account.sessions.revokeOthersMessage', { count: otherSessions.length })}
          </p>
          <div className="flex gap-3 justify-end">
            <button
              onClick={() => setRevokeTarget(null)}
              className="px-4 py-2 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
            >
              {t('account.actions.cancel')}
            </button>
            <button
              onClick={handleRevokeAllOthers}
              disabled={revokingAll}
              className="px-4 py-2 rounded-lg bg-red-600 text-white text-sm font-medium hover:bg-red-700 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {revokingAll ? t('account.sessions.revoking') : t('account.sessions.revokeAllConfirm')}
            </button>
          </div>
        </ModalShell>
      )}
    </Section>
  )
}

// ── Data Import Section ─────────────────────────────────────────────────────

type ImportPhase = 'upload' | 'progress' | 'result'
type ImportMode = 'single' | 'multipart'

interface ImportResult {
  imported: number
  skipped: number
  errors: string[]
}

interface MultipartManifestPart {
  name: string
  role?: string
  sha256?: string
  size_bytes?: number
}

interface MultipartManifest {
  format_version?: number
  multipart?: boolean
  parts?: MultipartManifestPart[]
  [key: string]: unknown
}

const webImportPartLimitBytes = 4 * 1024 * 1024 * 1024
const accountDeletionConfirmation = 'ACCOUNT LÖSCHEN'
type TranslationFn = (key: string, options?: Record<string, unknown>) => string

export function formatDataImportProgressMessage(status: DataImportStatus, t: TranslationFn): string | null {
  const phaseName = status.phase_name ?? status.metadata?.phase_name
  const phase = status.phase ?? status.metadata?.phase

  switch (phaseName) {
    case 'files':
      return t('account.dataImport.progress.files')
    case 'profile':
      return t('account.dataImport.progress.profile')
    case 'activities':
      return t('account.dataImport.progress.activities')
    case 'health':
      return t('account.dataImport.progress.health')
    default:
      break
  }

  if (typeof phase === 'number') {
    switch (phase) {
      case 5:
        return t('account.dataImport.progress.activities')
      case 6:
        return t('account.dataImport.progress.activityData')
      case 7:
        return t('account.dataImport.progress.health')
      case 8:
        return t('account.dataImport.progress.references')
      case 9:
        return t('account.dataImport.progress.jsonReferences')
      case 10:
        return t('account.dataImport.progress.enrichment')
      default:
        return t('account.dataImport.progress.phase', { phase })
    }
  }

  if (typeof status.entities_imported === 'number' || typeof status.entities_skipped === 'number' || typeof status.entities_failed === 'number') {
    return t('account.dataImport.progress.data')
  }

  return null
}

function isValidMultipartPartName(name: string): boolean {
  return name === 'metadata.zip' || /^files-\d{4}\.zip$/.test(name)
}

function multipartExpectedParts(manifest: MultipartManifest): MultipartManifestPart[] {
  const parts = Array.isArray(manifest.parts) ? manifest.parts.filter(part => isValidMultipartPartName(part.name)) : []
  if (!parts.some(part => part.name === 'metadata.zip')) {
    return [{ name: 'metadata.zip', role: 'metadata' }, ...parts]
  }
  return parts
}

function formatBytes(bytes: number): string {
  if (bytes >= 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${Math.max(1, Math.round(bytes / 1024))} KB`
}

function parseImportResult(status: DataImportStatus): ImportResult {
  // The backend encodes final counts in metadata.message as JSON or
  // falls back to a plain message. We attempt a best-effort parse.
  if (typeof status.entities_imported === 'number' || typeof status.entities_skipped === 'number' || typeof status.entities_failed === 'number') {
    return {
      imported: status.entities_imported ?? 0,
      skipped: status.entities_skipped ?? 0,
      errors: status.error ? [status.error] : [],
    }
  }
  const msg = status.metadata?.message ?? status.message ?? ''
  try {
    const parsed = JSON.parse(msg) as Partial<ImportResult>
    return {
      imported: parsed.imported ?? 0,
      skipped: parsed.skipped ?? 0,
      errors: Array.isArray(parsed.errors) ? parsed.errors : [],
    }
  } catch {
    return { imported: 0, skipped: 0, errors: status.error ? [status.error] : [] }
  }
}

function DataImportSection() {
  const { t } = useTranslation()
  const { addToast } = useToast()
  const [phase, setPhase] = useState<ImportPhase>('upload')
  const [importMode, setImportMode] = useState<ImportMode>('single')
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [multipartFiles, setMultipartFiles] = useState<File[]>([])
  const [multipartManifest, setMultipartManifest] = useState<MultipartManifest | null>(null)
  const [multipartParts, setMultipartParts] = useState<MultipartManifestPart[]>([])
  const [multipartError, setMultipartError] = useState<string | null>(null)
  const [activeUploadJobID, setActiveUploadJobID] = useState<string | null>(null)
  const [uploading, setUploading] = useState(false)
  const [logLines, setLogLines] = useState<string[]>([])
  const [progressCurrent, setProgressCurrent] = useState(0)
  const [progressTotal, setProgressTotal] = useState(0)
  const [result, setResult] = useState<ImportResult | null>(null)
  const [errorsExpanded, setErrorsExpanded] = useState(false)
  const importAbortRef = useRef<AbortController | null>(null)
  const logEndRef = useRef<HTMLDivElement | null>(null)
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const multipartInputRef = useRef<HTMLInputElement | null>(null)

  useEffect(() => {
    return () => { importAbortRef.current?.abort() }
  }, [])

  // Auto-scroll log to bottom whenever new lines arrive
  useEffect(() => {
    if (typeof logEndRef.current?.scrollIntoView === 'function') {
      logEndRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [logLines])

  async function pollImportJob(jobID: string, abort: AbortController) {
    // Poll every 2 s (max ~30 min = 900 iterations)
    for (let i = 0; i < 900; i++) {
      if (abort.signal.aborted) return
      await abortableSleep(2000, abort.signal)
      const status = await getDataImportStatus(jobID)

      if (status.import_upload?.parts?.length) {
        const received = status.import_upload.parts.filter(part => part.received).length
        const total = status.import_upload.parts.length
        setProgressCurrent(received)
        setProgressTotal(total)
      } else if (status.progress_total && status.progress_total > 0) {
        setProgressCurrent(status.progress_current ?? 0)
        setProgressTotal(status.progress_total)
      }

      const statusMessage = formatDataImportProgressMessage(status, t)
      if (statusMessage) {
        const prefix = status.status === 'running' ? '●' : '✓'
        const line = `${prefix} ${statusMessage}`
        setLogLines(prev => {
          const last = prev[prev.length - 1] ?? ''
          if (last === line) return prev
          if (last.startsWith('●') && line.startsWith('✓')) {
            return [...prev.slice(0, -1), line]
          }
          return [...prev, line]
        })
      }

      if (status.status === 'completed') {
        const parsed = parseImportResult(status)
        setResult(parsed)
        setPhase('result')
        addToast({ type: 'success', title: t('account.dataImport.completedTitle') })
        return
      }
      if (status.status === 'failed') {
        const errMsg = status.error ?? t('account.dataImport.failedGeneric')
        addToast({ type: 'error', title: t('account.dataImport.failedTitle'), message: errMsg, duration: 0 })
        setResult({ imported: 0, skipped: 0, errors: [errMsg] })
        setPhase('result')
        return
      }
    }
    addToast({ type: 'error', title: t('account.dataImport.timeoutTitle'), message: t('account.dataImport.timeoutMessage'), duration: 0 })
    setPhase('upload')
  }

  async function handleImport() {
    if (importMode === 'multipart') {
      await handleMultipartImport()
      return
    }
    if (!selectedFile) return
    const abort = new AbortController()
    importAbortRef.current = abort
    setUploading(true)
    try {
      const { job_id } = await startDataImport(selectedFile)
      addToast({ type: 'info', title: t('account.dataImport.startedTitle') })
      setUploading(false)
      setPhase('progress')
      setLogLines([])
      await pollImportJob(job_id, abort)
    } catch (e) {
      if (e instanceof DOMException && e.name === 'AbortError') return
      if (!abort.signal.aborted) {
        addToast({ type: 'error', title: t('account.dataImport.failedTitle'), duration: 0 })
      }
      setPhase('upload')
    } finally {
      setUploading(false)
      importAbortRef.current = null
    }
  }

  async function handleMultipartFileSelection(files: FileList | null) {
    const selected = Array.from(files ?? [])
    setMultipartFiles(selected)
    setMultipartManifest(null)
    setMultipartParts([])
    setMultipartError(null)
    const manifestFile = selected.find(file => file.name === 'manifest.json')
    if (!manifestFile) {
      if (selected.length > 0) setMultipartError(t('account.dataImport.manifestMissing'))
      return
    }
    try {
      const parsed = JSON.parse(await manifestFile.text()) as MultipartManifest
      if (parsed.format_version !== 1 || parsed.multipart !== true) {
        setMultipartError(t('account.dataImport.manifestNotMultipart'))
        return
      }
      const expected = multipartExpectedParts(parsed)
      if (expected.length === 0) {
        setMultipartError(t('account.dataImport.manifestNoParts'))
        return
      }
      setMultipartManifest(parsed)
      setMultipartParts(expected)
    } catch {
      setMultipartError(t('account.dataImport.manifestReadFailed'))
    }
  }

  async function handleMultipartImport() {
    if (!multipartManifest || multipartMissingNames.length > 0 || multipartOversizedFiles.length > 0) return
    const abort = new AbortController()
    importAbortRef.current = abort
    setUploading(true)
    setLogLines([])
    setProgressCurrent(0)
    setProgressTotal(multipartParts.length)
    try {
      const session = await createMultipartDataImport(multipartManifest)
      setActiveUploadJobID(session.job_id)
      for (const part of multipartParts) {
        if (abort.signal.aborted) return
        const file = multipartFileByName.get(part.name)
        if (!file) throw new Error(`missing part ${part.name}`)
        await uploadMultipartDataImportPart(session.job_id, file)
        setProgressCurrent(current => current + 1)
        setLogLines(prev => [...prev, t('account.dataImport.partUploaded', { name: part.name })])
      }
      await startMultipartDataImport(session.job_id)
      setActiveUploadJobID(null)
      addToast({ type: 'info', title: t('account.dataImport.startedTitle') })
      setUploading(false)
      setPhase('progress')
      await pollImportJob(session.job_id, abort)
    } catch (e) {
      if (e instanceof DOMException && e.name === 'AbortError') return
      if (!abort.signal.aborted) {
        addToast({ type: 'error', title: t('account.dataImport.failedTitle'), duration: 0 })
      }
      setPhase('upload')
    } finally {
      setUploading(false)
      importAbortRef.current = null
    }
  }

  async function handleCancelMultipartUpload() {
    if (!activeUploadJobID) return
    await cancelMultipartDataImport(activeUploadJobID).catch(() => undefined)
    importAbortRef.current?.abort()
    setActiveUploadJobID(null)
    setUploading(false)
    setPhase('upload')
  }

  function handleReset() {
    setPhase('upload')
    setSelectedFile(null)
    setMultipartFiles([])
    setMultipartManifest(null)
    setMultipartParts([])
    setMultipartError(null)
    setActiveUploadJobID(null)
    setLogLines([])
    setProgressCurrent(0)
    setProgressTotal(0)
    setResult(null)
    setErrorsExpanded(false)
    if (fileInputRef.current) fileInputRef.current.value = ''
    if (multipartInputRef.current) multipartInputRef.current.value = ''
  }

  const multipartFileByName = new Map(multipartFiles.map(file => [file.name, file]))
  const multipartMissingNames = multipartParts
    .map(part => part.name)
    .filter(name => !multipartFileByName.has(name))
  const multipartOversizedFiles = multipartParts
    .map(part => multipartFileByName.get(part.name))
    .filter((file): file is File => Boolean(file && file.size > webImportPartLimitBytes))
  const canStartImport = importMode === 'single'
    ? Boolean(selectedFile)
    : Boolean(multipartManifest && multipartMissingNames.length === 0 && multipartOversizedFiles.length === 0 && !multipartError)

  const progressPct = progressTotal > 0
    ? Math.round((progressCurrent / progressTotal) * 100)
    : 0

  return (
    <Section title={t('account.dataImport.title')} description={t('account.dataImport.description')}>
      {phase === 'upload' && (
        <div className="space-y-4">
          <div className="inline-flex rounded-lg border border-neutral-200 dark:border-neutral-700 overflow-hidden">
            <button
              type="button"
              onClick={() => setImportMode('single')}
              className={`px-3 py-1.5 text-sm transition-colors cursor-pointer ${importMode === 'single' ? 'bg-neutral-900 text-white dark:bg-neutral-100 dark:text-neutral-900' : 'text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800'}`}
            >
              {t('account.dataImport.singleZip')}
            </button>
            <button
              type="button"
              onClick={() => setImportMode('multipart')}
              className={`px-3 py-1.5 text-sm transition-colors cursor-pointer ${importMode === 'multipart' ? 'bg-neutral-900 text-white dark:bg-neutral-100 dark:text-neutral-900' : 'text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800'}`}
            >
              {t('account.dataImport.multipartExport')}
            </button>
          </div>

          {importMode === 'single' ? (
            <>
              <div className="flex items-center gap-3">
                <input
                  ref={fileInputRef}
                  type="file"
                  accept=".zip"
                  className="hidden"
                  onChange={e => setSelectedFile(e.target.files?.[0] ?? null)}
                />
                <button
                  type="button"
                  onClick={() => fileInputRef.current?.click()}
                  className="px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 text-sm text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
                >
                  {t('account.dataImport.selectFile')}
                </button>
                <span className="text-sm text-neutral-500 dark:text-neutral-400 truncate max-w-xs">
                  {selectedFile ? selectedFile.name : t('account.dataImport.noFileSelected')}
                </span>
              </div>
              <p className="text-xs text-neutral-400 dark:text-neutral-500">{t('account.dataImport.uploadHint')}</p>
            </>
          ) : (
            <div className="space-y-3">
              <div className="flex items-center gap-3">
                <input
                  ref={multipartInputRef}
                  aria-label={t('account.dataImport.multipartFilesAria')}
                  type="file"
                  accept=".json,.zip"
                  multiple
                  className="hidden"
                  onChange={e => { void handleMultipartFileSelection(e.target.files) }}
                />
                <button
                  type="button"
                  onClick={() => multipartInputRef.current?.click()}
                  className="px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 text-sm text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
                >
                  {t('account.dataImport.selectMultipartFiles')}
                </button>
                <span className="text-sm text-neutral-500 dark:text-neutral-400">
                  {multipartFiles.length > 0 ? t('account.dataImport.fileCountSelected', { count: multipartFiles.length }) : t('account.dataImport.noFileSelected')}
                </span>
              </div>
              {multipartError && <p className="text-xs text-red-600 dark:text-red-400">{multipartError}</p>}
              {multipartManifest && (
                <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 divide-y divide-neutral-200 dark:divide-neutral-800">
                  {multipartParts.map(part => {
                    const file = multipartFileByName.get(part.name)
                    const tooLarge = Boolean(file && file.size > webImportPartLimitBytes)
                    return (
                      <div key={part.name} className="flex items-center justify-between gap-3 px-3 py-2 text-xs">
                        <div className="min-w-0">
                          <p className="truncate font-medium text-neutral-700 dark:text-neutral-300">{part.name}</p>
                          <p className={tooLarge ? 'text-red-600 dark:text-red-400' : 'text-neutral-500 dark:text-neutral-500'}>
                            {file ? formatBytes(file.size) : t('account.dataImport.missing')}
                            {tooLarge ? t('account.dataImport.webUploadLimitExceededSuffix') : ''}
                          </p>
                        </div>
                        <span className={file && !tooLarge ? 'text-green-600 dark:text-green-400' : 'text-neutral-400 dark:text-neutral-500'}>
                          {file && !tooLarge ? t('account.dataImport.ready') : t('account.dataImport.open')}
                        </span>
                      </div>
                    )
                  })}
                </div>
              )}
              {multipartMissingNames.length > 0 && (
                <p className="text-xs text-red-600 dark:text-red-400">
                  {t('account.dataImport.missingParts', { parts: multipartMissingNames.join(', ') })}
                </p>
              )}
              {multipartOversizedFiles.length > 0 && (
                <p className="text-xs text-red-600 dark:text-red-400">
                  {t('account.dataImport.oversizedPartsHint')}
                </p>
              )}
            </div>
          )}

          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={handleImport}
              disabled={!canStartImport || uploading}
              className="px-4 py-2 rounded-lg bg-accent-DEFAULT text-white text-sm font-medium hover:bg-accent-600 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {uploading ? t('account.dataImport.uploading') : t('account.dataImport.upload')}
            </button>
            {activeUploadJobID && (
              <button
                type="button"
                onClick={() => { void handleCancelMultipartUpload() }}
                className="px-3 py-2 rounded-lg border border-neutral-200 dark:border-neutral-700 text-sm text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
              >
                {t('account.actions.cancel')}
              </button>
            )}
          </div>
        </div>
      )}

      {phase === 'progress' && (
        <div className="space-y-4">
          {/* Progress bar */}
          <div className="space-y-1">
            <div className="flex justify-between text-xs text-neutral-500 dark:text-neutral-400">
              <span>{progressTotal > 0 ? `${progressCurrent} / ${progressTotal}` : t('account.dataImport.processing')}</span>
              {progressTotal > 0 && <span>{progressPct} %</span>}
            </div>
            <div className="h-2 rounded-full bg-neutral-200 dark:bg-neutral-700 overflow-hidden">
              <div
                className="h-full bg-accent-DEFAULT rounded-full transition-all duration-500"
                style={{ width: progressTotal > 0 ? `${progressPct}%` : '100%' }}
              />
            </div>
          </div>

          {/* Scrolling log */}
          <div className="h-48 overflow-y-auto rounded-lg bg-neutral-50 dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-700 p-3 font-mono text-xs text-neutral-700 dark:text-neutral-300 space-y-0.5">
            {logLines.length === 0 && (
              <p className="text-neutral-400 dark:text-neutral-500">{t('account.dataImport.waitingForStatus')}</p>
            )}
            {logLines.map((line, idx) => (
              <p key={idx} className={line.startsWith('●') ? 'text-accent-DEFAULT' : 'text-green-600 dark:text-green-400'}>
                {line}
              </p>
            ))}
            <div ref={logEndRef} />
          </div>
        </div>
      )}

      {phase === 'result' && result && (
        <div className="space-y-4">
          {/* Summary row */}
          <div className="flex flex-wrap gap-4 text-sm">
            <span className="text-green-600 dark:text-green-400 font-medium">
              {t('account.dataImport.resultImported', { count: result.imported })}
            </span>
            {result.skipped > 0 && (
              <span className="text-neutral-500 dark:text-neutral-400">
                {t('account.dataImport.resultSkipped', { count: result.skipped })}
              </span>
            )}
            {result.errors.length > 0 && (
              <span className="text-red-600 dark:text-red-400">
                {t('account.dataImport.resultErrors', { count: result.errors.length })}
              </span>
            )}
          </div>

          {/* Collapsible error list */}
          {result.errors.length > 0 && (
            <div className="rounded-lg border border-red-200 dark:border-red-800 overflow-hidden">
              <button
                type="button"
                onClick={() => setErrorsExpanded(v => !v)}
                className="w-full flex items-center justify-between px-3 py-2 text-sm font-medium text-red-700 dark:text-red-400 bg-red-50 dark:bg-red-950/30 hover:bg-red-100 dark:hover:bg-red-950/50 transition-colors cursor-pointer"
              >
                {t('account.dataImport.errorsHeading')}
                <span className="text-xs">{errorsExpanded ? '▲' : '▼'}</span>
              </button>
              {errorsExpanded && (
                <ul className="px-3 py-2 space-y-1 font-mono text-xs text-neutral-700 dark:text-neutral-300 max-h-40 overflow-y-auto">
                  {result.errors.map((err, idx) => (
                    <li key={idx} className="text-red-600 dark:text-red-400">{err}</li>
                  ))}
                </ul>
              )}
            </div>
          )}

          {/* Hint: enrichment */}
          <p className="text-xs text-neutral-500 dark:text-neutral-400">{t('account.dataImport.resultEnrichmentHint')}</p>

          {/* Hint: OAuth */}
          <p className="text-xs text-neutral-500 dark:text-neutral-400">{t('account.dataImport.resultOAuthHint')}</p>

          {/* New import button */}
          <button
            type="button"
            onClick={handleReset}
            className="px-4 py-2 rounded-lg border border-neutral-200 dark:border-neutral-700 text-sm text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
          >
            {t('account.dataImport.newImport')}
          </button>
        </div>
      )}
    </Section>
  )
}

// ── Danger Zone Section ─────────────────────────────────────────────────────

// abortableSleep resolves after `ms` milliseconds, or rejects with an
// AbortError immediately when the signal is aborted. This avoids letting a
// pending setTimeout race past a component unmount.
function abortableSleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException('Aborted', 'AbortError'))
      return
    }
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', onAbort)
      resolve()
    }, ms)
    const onAbort = () => {
      clearTimeout(timer)
      signal.removeEventListener('abort', onAbort)
      reject(new DOMException('Aborted', 'AbortError'))
    }
    signal.addEventListener('abort', onAbort)
  })
}

function formatExportExpiry(iso: string | undefined, locale: string): string | null {
  if (!iso) return null
  return new Intl.DateTimeFormat(locale, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(iso))
}

function DangerZoneSection() {
  const { t, i18n } = useTranslation()
  const { hasPassword } = useUser()
  const navigate = useNavigate()
  const { addToast } = useToast()
  const queryClient = useQueryClient()
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [deletePw, setDeletePw] = useState('')
  const [deleteConfirmation, setDeleteConfirmation] = useState('')
  const [exporting, setExporting] = useState(false)

  const exportAbortRef = useRef<AbortController | null>(null)

  // Cleanup polling on unmount
  useEffect(() => {
    return () => { exportAbortRef.current?.abort() }
  }, [])

  const { data: latestExport } = useQuery({
    queryKey: ['data-export-latest'],
    queryFn: getLatestDataExport,
    refetchOnWindowFocus: true,
    staleTime: 30_000,
  })

  const deleteMut = useMutation({
    mutationFn: deleteAccount,
    onSuccess: () => {
      clearToken()
      navigate('/login?message=deletion_scheduled', { replace: true })
    },
    onError: (err) => {
      addToast({ type: 'error', title: getAccountErrorMessage(err), duration: 0 })
    },
  })

  async function downloadDataExport(status: DataExportStatus) {
    if (status.unavailable_reason === 'artifact_missing') {
      addToast({
        type: 'error',
        title: t('account.dataExport.downloadFailedTitle'),
        message: t('account.dataExport.artifactMissing'),
        duration: 0,
      })
      queryClient.invalidateQueries({ queryKey: ['data-export-latest'] })
      return
    }
    if (status.unavailable_reason === 'artifact_backend_unavailable') {
      addToast({
        type: 'error',
        title: t('account.dataExport.downloadFailedTitle'),
        message: t('account.dataExport.backendUnavailable'),
        duration: 0,
      })
      return
    }
    if (!status.download_session_url && (!status.parts || status.parts.length === 0)) {
      addToast({ type: 'error', title: t('account.dataExport.invalidUrlTitle'), duration: 0 })
      return
    }
    try {
      const today = new Date().toISOString().slice(0, 10) // YYYY-MM-DD
      if (status.artifact_format === 'multipart' && status.parts && status.parts.length > 0) {
        const session = await createDataExportDownloadSession(status.job_id, { all: true })
        if (session.objects && session.objects.length > 0) {
          for (const object of session.objects) {
            triggerNativeDownload(object.url, object.name)
          }
        } else {
          triggerNativeDownload(session.url, `skadia-export-${today}-manifest.json`)
        }
      } else {
        const session = await createDataExportDownloadSession(status.job_id, { single: true })
        triggerNativeDownload(session.url, `skadia-export-${today}.zip`)
      }

      const detailParts: string[] = []
      if (status.sha256) detailParts.push(`SHA-256: ${status.sha256}`)
      if (status.size_bytes) {
        detailParts.push(`${(status.size_bytes / 1024 / 1024).toFixed(1)} MB`)
      }
      addToast({
        type: 'success',
        title: t('account.dataExport.completedTitle'),
        message: detailParts.length > 0 ? detailParts.join(' / ') : undefined,
        duration: 0,
      })
    } catch {
      addToast({ type: 'error', title: t('account.dataExport.downloadFailedTitle'), duration: 0 })
      queryClient.invalidateQueries({ queryKey: ['data-export-latest'] })
    }
  }

  async function handleExport() {
    const abort = new AbortController()
    exportAbortRef.current = abort
    setExporting(true)
    try {
      const { job_id } = await startDataExport()
      queryClient.invalidateQueries({ queryKey: ['data-export-latest'] })
      addToast({ type: 'info', title: t('account.dataExport.startedTitle') })

      // Poll for completion (max 10 min = 120 × 5s)
      for (let i = 0; i < 120; i++) {
        if (abort.signal.aborted) return
        await abortableSleep(5000, abort.signal)
        const status = await getDataExportStatus(job_id)
        if (status.status === 'completed' && (status.download_session_url || (status.parts && status.parts.length > 0))) {
          await downloadDataExport(status)
          queryClient.invalidateQueries({ queryKey: ['data-export-latest'] })
          return
        }
        if (status.status === 'failed' && status.partial && (status.download_session_url || (status.parts && status.parts.length > 0))) {
          await downloadDataExport(status)
          queryClient.invalidateQueries({ queryKey: ['data-export-latest'] })
          addToast({
            type: 'warning',
            title: t('account.dataExport.failedTitle'),
            message: t('account.dataExport.partialDownloaded'),
            duration: 0,
          })
          return
        }
        if (status.status === 'failed') {
          if (status.error === 'export_failed') {
            addToast({
              type: 'error',
              title: t('account.dataExport.failedTitle'),
              message: t('account.dataExport.failedGeneric'),
              duration: 0,
            })
          } else {
            addToast({
              type: 'error',
              title: t('account.dataExport.failedTitle'),
              message: status.error ?? t('account.dataExport.unknownError'),
              duration: 0,
            })
          }
          return
        }
      }
      addToast({
        type: 'error',
        title: t('account.dataExport.timeoutTitle'),
        message: t('account.dataExport.timeoutMessage'),
        duration: 0,
      })
    } catch (e) {
      if (e instanceof DOMException && e.name === 'AbortError') return
      if (!abort.signal.aborted) {
        addToast({ type: 'error', title: t('account.dataExport.failedTitle'), duration: 0 })
      }
    } finally {
      setExporting(false)
      exportAbortRef.current = null
    }
  }

  function handleDeleteSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (hasPassword) {
      deleteMut.mutate({ password: deletePw })
    } else {
      deleteMut.mutate({ confirmation: accountDeletionConfirmation })
    }
  }

  const deleteConfirmationPhrase = t('account.delete.confirmationPhrase')
  const latestExportExpiry = formatExportExpiry(latestExport?.expires_at, i18n.language || 'de')
  const latestVisibleExport = latestExport && (latestExport.status === 'completed' || latestExport.partial) ? latestExport : null
  const latestExportAvailable = latestVisibleExport
    ? !latestVisibleExport.unavailable_reason &&
      Boolean(latestVisibleExport.download_session_url || (latestVisibleExport.parts && latestVisibleExport.parts.length > 0)) &&
      (!latestVisibleExport.expires_at || new Date(latestVisibleExport.expires_at).getTime() > Date.now())
    : false

  return (
    <Section title={t('account.danger.title')} danger>
      <div className="space-y-4">
        {/* Data Export */}
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
              {t('account.dataExport.title')}
            </p>
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              {t('account.dataExport.description')}
            </p>
          </div>
          <button
            onClick={handleExport}
            disabled={exporting}
            className="px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 text-sm text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {exporting ? t('account.dataExport.exporting') : t('account.dataExport.export')}
          </button>
        </div>

        {latestVisibleExport && (
          <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 px-3 py-2 bg-neutral-50 dark:bg-neutral-900/40">
            <div className="flex items-center justify-between gap-3">
              <div className="min-w-0">
                <p className="text-xs font-medium text-neutral-800 dark:text-neutral-200">
                  {t('account.dataExport.latestHeading')}
                </p>
                <p className="text-xs text-neutral-500 dark:text-neutral-400">
                  {latestExportAvailable ? t('account.dataExport.available') : t('account.dataExport.expired')}
                  {latestExportExpiry ? ` · ${t('account.dataExport.expiresPrefix')} ${latestExportExpiry}` : ''}
                </p>
              </div>
              {latestExportAvailable && (
                <button
                  type="button"
                  onClick={() => { void downloadDataExport(latestVisibleExport) }}
                  className="flex-shrink-0 px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 text-sm text-neutral-700 dark:text-neutral-300 hover:bg-white dark:hover:bg-neutral-800 transition-colors cursor-pointer"
                >
                  {latestVisibleExport.artifact_format === 'multipart'
                    ? t('account.dataExport.downloadAllParts')
                    : t('account.dataExport.download')}
                </button>
              )}
            </div>
            {latestVisibleExport.parts && latestVisibleExport.parts.length > 0 && (
              <div className="mt-2 divide-y divide-neutral-200 dark:divide-neutral-800">
                {latestVisibleExport.parts.map(part => (
                  <div key={part.name} className="flex items-center justify-between gap-3 py-1.5 text-xs">
                    <div className="min-w-0">
                      <p className="truncate font-medium text-neutral-700 dark:text-neutral-300">{part.name}</p>
                      <p className="truncate text-neutral-500 dark:text-neutral-500">
                        {part.size_bytes ? `${(part.size_bytes / 1024 / 1024).toFixed(1)} MB` : ''}
                        {part.sha256 ? ` · SHA-256: ${part.sha256}` : ''}
                        {part.web_import_supported === false ? ` · ${t('account.dataExport.webImportCliOnly')}` : ''}
                      </p>
                    </div>
                    {latestExportAvailable && (
                      <button
                        type="button"
                        onClick={() => {
                          void createDataExportDownloadSession(latestVisibleExport.job_id, { part: part.name })
                            .then(session => triggerNativeDownload(session.url, part.name))
                            .catch(() => addToast({ type: 'error', title: t('account.dataExport.downloadFailedTitle'), duration: 0 }))
                        }}
                        className="flex-shrink-0 px-2 py-1 rounded border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-white dark:hover:bg-neutral-800 transition-colors cursor-pointer"
                      >
                        {t('account.dataExport.download')}
                      </button>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        <div className="border-t border-red-200 dark:border-red-800/60" />

        {/* Delete Account */}
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium text-red-700 dark:text-red-400">
              {t('account.delete.title')}
            </p>
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              {t('account.delete.description')}
            </p>
          </div>
          <button
            onClick={() => setDeleteDialogOpen(true)}
            className="px-3 py-1.5 rounded-lg border border-red-300 dark:border-red-800 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-950/30 transition-colors cursor-pointer"
          >
            {t('account.delete.title')}
          </button>
        </div>
      </div>

      {deleteDialogOpen && (
        <ModalShell onClose={() => setDeleteDialogOpen(false)}>
          <h3 className="text-lg font-semibold text-red-700 dark:text-red-400 mb-2">
            {t('account.delete.title')}
          </h3>
          <div className="p-3 rounded-lg bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-800 mb-4">
            <p className="text-sm text-red-700 dark:text-red-400">
              {t('account.delete.warning')}
            </p>
          </div>
          <form onSubmit={handleDeleteSubmit} className="space-y-4">
            {hasPassword ? (
              <div className="space-y-1">
                <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
                  {t('account.password.confirm')}
                </label>
                <input
                  type="password"
                  value={deletePw}
                  onChange={e => setDeletePw(e.target.value)}
                  className={AUTH_INPUT_CLASS}
                  required
                  autoComplete="current-password"
                  autoFocus
                />
              </div>
            ) : (
              <div className="space-y-1">
                <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
                  {t('account.delete.typeConfirmationPrefix')}{' '}
                  <span className="font-mono font-bold">
                    {deleteConfirmationPhrase}
                  </span>{' '}
                  {t('account.delete.typeConfirmationSuffix')}
                </label>
                <input
                  type="text"
                  value={deleteConfirmation}
                  onChange={e => setDeleteConfirmation(e.target.value)}
                  className={AUTH_INPUT_CLASS}
                  required
                  autoFocus
                  placeholder={t('account.delete.confirmationPlaceholder')}
                />
              </div>
            )}
            <div className="flex gap-3 justify-end">
              <button
                type="button"
                onClick={() => setDeleteDialogOpen(false)}
                className="px-4 py-2 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
              >
                {t('account.actions.cancel')}
              </button>
              <button
                type="submit"
                disabled={deleteMut.isPending || (!hasPassword && deleteConfirmation !== deleteConfirmationPhrase)}
                className="px-4 py-2 rounded-lg bg-red-600 text-white text-sm font-medium hover:bg-red-700 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {deleteMut.isPending ? t('account.delete.deleting') : t('account.delete.confirmDelete')}
              </button>
            </div>
          </form>
        </ModalShell>
      )}
    </Section>
  )
}

// ── Main Tab Component ──────────────────────────────────────────────────────

export default function AccountSettingsTab() {
  return (
    <div className="space-y-6">
      <ProfileSection />
      <PreferencesSection />
      <EmailSection />
      <PasswordSection />
      <SessionsSection />
      <DataImportSection />
      <DangerZoneSection />
    </div>
  )
}
