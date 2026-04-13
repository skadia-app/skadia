import { useState, useRef, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
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
  startDataExport, getDataExportStatus,
  startDataImport, getDataImportStatus,
  type UserSession,
  type DataImportStatus,
} from '@/api/auth'
import { fmtDateTime, parseUserAgent } from '@/lib/format'
import { SettingsSection as Section } from '@/components/ui/SettingsSection'
import { accountSettingsStrings as S } from '@/lib/strings/account-settings'

// ── Profile Section ─────────────────────────────────────────────────────────

function ProfileSection() {
  const { user } = useUser()
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const [username, setUsername] = useState(user?.username ?? '')
  const [displayName, setDisplayName] = useState(user?.display_name ?? '')

  const mutation = useMutation({
    mutationFn: updateMe,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['me'] })
      addToast({ type: 'success', title: 'Profil aktualisiert' })
    },
    onError: (err) => {
      const msg = err instanceof Error ? err.message : 'Update failed'
      addToast({ type: 'error', title: getAuthErrorMessage(msg), duration: 0 })
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
    <Section title="Profil" description="Benutzername und Anzeigename.">
      <form onSubmit={handleSave} className="space-y-4">
        <div className="space-y-1">
          <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Benutzername</label>
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
          <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Anzeigename</label>
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
          {mutation.isPending ? 'Wird gespeichert...' : 'Speichern'}
        </button>
      </form>
    </Section>
  )
}

// ── Email Section ───────────────────────────────────────────────────────────

function EmailSection() {
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
        addToast({ type: 'success', title: 'Bestätigungsmail gesendet', message: 'Bitte klicke den Link in der E-Mail um die Änderung abzuschließen.' })
      } else {
        addToast({ type: 'success', title: 'E-Mail geändert' })
      }
    },
    onError: (err) => {
      const msg = err instanceof Error ? err.message : 'Email change failed'
      addToast({ type: 'error', title: getAuthErrorMessage(msg), duration: 0 })
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const data: { new_email: string; password?: string } = { new_email: newEmail }
    if (hasPassword) data.password = password
    mutation.mutate(data)
  }

  return (
    <Section title="E-Mail" description="Deine E-Mail-Adresse für Login und Benachrichtigungen.">
      <div className="flex items-center gap-3">
        <span className="text-sm text-neutral-700 dark:text-neutral-300 font-mono">{user?.email ?? '-'}</span>
        <button
          onClick={() => setDialogOpen(true)}
          className="px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 text-sm text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
        >
          E-Mail ändern
        </button>
      </div>

      {dialogOpen && (
        <ModalShell onClose={() => setDialogOpen(false)}>
          <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-4">E-Mail ändern</h3>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-1">
              <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Neue E-Mail</label>
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
                <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Passwort bestätigen</label>
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
                Abbrechen
              </button>
              <button
                type="submit"
                disabled={mutation.isPending}
                className="px-4 py-2 rounded-lg bg-accent-DEFAULT text-white text-sm font-medium hover:bg-accent-600 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {mutation.isPending ? 'Wird gesendet...' : 'Ändern'}
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
      const msg = err instanceof Error ? err.message : 'Password change failed'
      addToast({ type: 'error', title: getAuthErrorMessage(msg), duration: 0 })
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (newPw !== confirmPw) {
      addToast({ type: 'error', title: 'Passwörter stimmen nicht überein', duration: 0 })
      return
    }
    mutation.mutate()
  }

  // OAuth-only user without must_change_password — no password form needed
  if (!hasPassword && !mustChangePassword) {
    return (
      <Section title="Passwort">
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          Kein Passwort gesetzt (OAuth-Konto). Du meldest dich über einen externen Anbieter an.
        </p>
      </Section>
    )
  }

  const showCurrentPw = hasPassword
  const isForceChange = mustChangePassword

  return (
    <Section title="Passwort" description={
      isForceChange && hasPassword
        ? undefined
        : isForceChange && !hasPassword
          ? 'Bitte setze ein Passwort für dein Konto.'
          : 'Passwort ändern.'
    }>
      {isForceChange && hasPassword && (
        <div className="p-3 rounded-lg bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-800">
          <p className="text-sm text-amber-700 dark:text-amber-400">
            Dein Passwort muss geändert werden bevor du fortfahren kannst.
          </p>
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-4">
        {showCurrentPw && (
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Aktuelles Passwort</label>
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
          <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Neues Passwort</label>
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
          <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Passwort bestätigen</label>
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
          {mutation.isPending ? 'Wird geändert...' : 'Passwort ändern'}
        </button>
      </form>
    </Section>
  )
}

// ── Sessions Section ────────────────────────────────────────────────────────

function SessionsSection() {
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
      addToast({ type: 'success', title: 'Session beendet' })
      setRevokeTarget(null)
    },
    onError: (err) => {
      const msg = err instanceof Error ? err.message : 'Revocation failed'
      addToast({ type: 'error', title: getAuthErrorMessage(msg), duration: 0 })
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
        addToast({ type: 'warning', title: `${succeeded} von ${others.length} ${others.length === 1 ? 'Session' : 'Sessions'} beendet`, duration: 0 })
      } else {
        addToast({ type: 'success', title: `${others.length === 1 ? '1 Session' : `${others.length} Sessions`} beendet` })
      }
      setRevokeTarget(null)
    } finally {
      setRevokingAll(false)
    }
  }

  const sessions: UserSession[] = data?.sessions ?? []
  const otherSessions = sessions.filter(s => !s.is_current)

  return (
    <Section title="Aktive Sessions" description="Alle Geräte und Browser, auf denen du angemeldet bist.">
      {isLoading ? (
        <p className="text-sm text-neutral-400">Lade Sessions...</p>
      ) : sessions.length === 0 ? (
        <p className="text-sm text-neutral-400">Keine aktiven Sessions.</p>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-neutral-200 dark:border-neutral-700 text-left text-neutral-500 dark:text-neutral-400">
                  <th className="py-2 pr-4 font-medium">Browser</th>
                  <th className="py-2 pr-4 font-medium">IP</th>
                  <th className="py-2 pr-4 font-medium">Erstellt</th>
                  <th className="py-2 pr-4 font-medium">Läuft ab</th>
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
                          aktuell
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
                          Beenden
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
              Alle anderen Sessions beenden
            </button>
          )}
        </>
      )}

      {/* Confirmation dialog for single session revocation */}
      {revokeTarget && revokeTarget !== 'all_others' && (
        <ModalShell onClose={() => setRevokeTarget(null)}>
          <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-2">Session beenden?</h3>
          <p className="text-sm text-neutral-600 dark:text-neutral-400 mb-4">
            Das Gerät wird abgemeldet und muss sich erneut anmelden.
          </p>
          <div className="flex gap-3 justify-end">
            <button
              onClick={() => setRevokeTarget(null)}
              className="px-4 py-2 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
            >
              Abbrechen
            </button>
            <button
              onClick={() => revokeMut.mutate(revokeTarget)}
              disabled={revokeMut.isPending}
              className="px-4 py-2 rounded-lg bg-red-600 text-white text-sm font-medium hover:bg-red-700 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {revokeMut.isPending ? 'Wird beendet...' : 'Session beenden'}
            </button>
          </div>
        </ModalShell>
      )}

      {/* Confirmation dialog for revoking all other sessions */}
      {revokeTarget === 'all_others' && (
        <ModalShell onClose={() => setRevokeTarget(null)}>
          <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-2">Alle anderen Sessions beenden?</h3>
          <p className="text-sm text-neutral-600 dark:text-neutral-400 mb-4">
            {otherSessions.length === 1
              ? '1 andere Session wird abgemeldet.'
              : `${otherSessions.length} andere Sessions werden abgemeldet.`}
          </p>
          <div className="flex gap-3 justify-end">
            <button
              onClick={() => setRevokeTarget(null)}
              className="px-4 py-2 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
            >
              Abbrechen
            </button>
            <button
              onClick={handleRevokeAllOthers}
              disabled={revokingAll}
              className="px-4 py-2 rounded-lg bg-red-600 text-white text-sm font-medium hover:bg-red-700 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {revokingAll ? 'Wird beendet...' : 'Alle beenden'}
            </button>
          </div>
        </ModalShell>
      )}
    </Section>
  )
}

// ── Data Import Section ─────────────────────────────────────────────────────

type ImportPhase = 'upload' | 'progress' | 'result'

interface ImportResult {
  imported: number
  skipped: number
  errors: string[]
}

function parseImportResult(status: DataImportStatus): ImportResult {
  // The backend encodes final counts in metadata.message as JSON or
  // falls back to a plain message. We attempt a best-effort parse.
  const msg = status.metadata?.message ?? ''
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
  const { addToast } = useToast()
  const [phase, setPhase] = useState<ImportPhase>('upload')
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [uploading, setUploading] = useState(false)
  const [logLines, setLogLines] = useState<string[]>([])
  const [progressCurrent, setProgressCurrent] = useState(0)
  const [progressTotal, setProgressTotal] = useState(0)
  const [result, setResult] = useState<ImportResult | null>(null)
  const [errorsExpanded, setErrorsExpanded] = useState(false)
  const importAbortRef = useRef<AbortController | null>(null)
  const logEndRef = useRef<HTMLDivElement | null>(null)
  const fileInputRef = useRef<HTMLInputElement | null>(null)

  useEffect(() => {
    return () => { importAbortRef.current?.abort() }
  }, [])

  // Auto-scroll log to bottom whenever new lines arrive
  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [logLines])

  async function handleImport() {
    if (!selectedFile) return
    const abort = new AbortController()
    importAbortRef.current = abort
    setUploading(true)
    try {
      const { job_id } = await startDataImport(selectedFile)
      addToast({ type: 'info', title: S.dataImport.started.title })
      setUploading(false)
      setPhase('progress')
      setLogLines([])

      // Poll every 2 s (max ~30 min = 900 iterations)
      for (let i = 0; i < 900; i++) {
        if (abort.signal.aborted) return
        await abortableSleep(2000, abort.signal)
        const status = await getDataImportStatus(job_id)

        // Update progress bar
        if (status.progress_total && status.progress_total > 0) {
          setProgressCurrent(status.progress_current ?? 0)
          setProgressTotal(status.progress_total)
        }

        // Append phase log line if metadata arrived
        if (status.metadata?.message) {
          const prefix = status.status === 'running' ? '●' : '✓'
          const line = `${prefix} ${status.metadata.message}`
          // Avoid duplicate lines
          setLogLines(prev => {
            const last = prev[prev.length - 1] ?? ''
            if (last === line) return prev
            // Replace running-indicator of same phase with completed version
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
          addToast({ type: 'success', title: S.dataImport.completed.title })
          return
        }
        if (status.status === 'failed') {
          const errMsg = status.error ?? S.dataImport.failed.messageGeneric
          addToast({ type: 'error', title: S.dataImport.failed.title, message: errMsg, duration: 0 })
          setResult({ imported: 0, skipped: 0, errors: [errMsg] })
          setPhase('result')
          return
        }
      }
      addToast({ type: 'error', title: S.dataImport.timeout.title, message: S.dataImport.timeout.message, duration: 0 })
      setPhase('upload')
    } catch (e) {
      if (e instanceof DOMException && e.name === 'AbortError') return
      if (!abort.signal.aborted) {
        addToast({ type: 'error', title: S.dataImport.failed.title, duration: 0 })
      }
      setPhase('upload')
    } finally {
      setUploading(false)
      importAbortRef.current = null
    }
  }

  function handleReset() {
    setPhase('upload')
    setSelectedFile(null)
    setLogLines([])
    setProgressCurrent(0)
    setProgressTotal(0)
    setResult(null)
    setErrorsExpanded(false)
    if (fileInputRef.current) fileInputRef.current.value = ''
  }

  const progressPct = progressTotal > 0
    ? Math.round((progressCurrent / progressTotal) * 100)
    : 0

  return (
    <Section title="Daten importieren" description={S.dataImport.uploadDescription}>
      {phase === 'upload' && (
        <div className="space-y-4">
          {/* File picker row */}
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
              {S.dataImport.selectFile}
            </button>
            <span className="text-sm text-neutral-500 dark:text-neutral-400 truncate max-w-xs">
              {selectedFile ? selectedFile.name : S.dataImport.noFileSelected}
            </span>
          </div>

          {/* Size hint */}
          <p className="text-xs text-neutral-400 dark:text-neutral-500">{S.dataImport.uploadHint}</p>

          {/* Start button */}
          <button
            type="button"
            onClick={handleImport}
            disabled={!selectedFile || uploading}
            className="px-4 py-2 rounded-lg bg-accent-DEFAULT text-white text-sm font-medium hover:bg-accent-600 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {uploading ? S.dataImport.uploadButtonPending : S.dataImport.uploadButton}
          </button>
        </div>
      )}

      {phase === 'progress' && (
        <div className="space-y-4">
          {/* Progress bar */}
          <div className="space-y-1">
            <div className="flex justify-between text-xs text-neutral-500 dark:text-neutral-400">
              <span>{progressTotal > 0 ? `${progressCurrent} / ${progressTotal}` : 'Wird verarbeitet...'}</span>
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
              <p className="text-neutral-400 dark:text-neutral-500">Warte auf Status-Updates...</p>
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
              {result.imported === 1 ? '1' : result.imported} {S.dataImport.resultImported}
            </span>
            {result.skipped > 0 && (
              <span className="text-neutral-500 dark:text-neutral-400">
                {result.skipped === 1 ? '1' : result.skipped} {S.dataImport.resultSkipped}
              </span>
            )}
            {result.errors.length > 0 && (
              <span className="text-red-600 dark:text-red-400">
                {result.errors.length === 1 ? '1' : result.errors.length} {S.dataImport.resultErrors}
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
                {S.dataImport.errorsHeading}
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
          <p className="text-xs text-neutral-500 dark:text-neutral-400">{S.dataImport.resultEnrichmentHint}</p>

          {/* Hint: OAuth */}
          <p className="text-xs text-neutral-500 dark:text-neutral-400">{S.dataImport.resultOAuthHint}</p>

          {/* New import button */}
          <button
            type="button"
            onClick={handleReset}
            className="px-4 py-2 rounded-lg border border-neutral-200 dark:border-neutral-700 text-sm text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
          >
            {S.dataImport.newImportButton}
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

function DangerZoneSection() {
  const { hasPassword } = useUser()
  const navigate = useNavigate()
  const { addToast } = useToast()
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [deletePw, setDeletePw] = useState('')
  const [deleteConfirmation, setDeleteConfirmation] = useState('')
  const [exporting, setExporting] = useState(false)

  const exportAbortRef = useRef<AbortController | null>(null)

  // Cleanup polling on unmount
  useEffect(() => {
    return () => { exportAbortRef.current?.abort() }
  }, [])

  const deleteMut = useMutation({
    mutationFn: deleteAccount,
    onSuccess: () => {
      clearToken()
      navigate('/login?message=deletion_scheduled', { replace: true })
    },
    onError: (err) => {
      const msg = err instanceof Error ? err.message : 'Account deletion failed'
      addToast({ type: 'error', title: getAuthErrorMessage(msg), duration: 0 })
    },
  })

  async function handleExport() {
    const abort = new AbortController()
    exportAbortRef.current = abort
    setExporting(true)
    try {
      const { job_id } = await startDataExport()
      addToast({ type: 'info', title: S.dataExport.started.title })

      // Poll for completion (max 10 min = 120 × 5s)
      for (let i = 0; i < 120; i++) {
        if (abort.signal.aborted) return
        await abortableSleep(5000, abort.signal)
        const status = await getDataExportStatus(job_id)
        if (status.status === 'completed' && status.download_url) {
          if (!status.download_url.startsWith('/api/')) {
            addToast({ type: 'error', title: S.dataExport.invalidUrl.title, duration: 0 })
            return
          }
          // Programmatic anchor click — bypasses popup blockers that intercept
          // window.open() outside of synchronous user gestures.
          const a = document.createElement('a')
          a.href = status.download_url
          const today = new Date().toISOString().slice(0, 10) // YYYY-MM-DD
          a.download = `skadia-export-${today}.zip`
          a.rel = 'noopener noreferrer'
          document.body.appendChild(a)
          a.click()
          a.remove()

          const detailParts: string[] = []
          if (status.sha256) detailParts.push(`SHA-256: ${status.sha256}`)
          if (status.size_bytes) {
            detailParts.push(`${(status.size_bytes / 1024 / 1024).toFixed(1)} MB`)
          }
          addToast({
            type: 'success',
            title: S.dataExport.completed.title,
            message: detailParts.length > 0 ? detailParts.join(S.dataExport.detailSeparator) : undefined,
            duration: 0,
          })
          return
        }
        if (status.status === 'failed') {
          if (status.error === 'export_failed') {
            addToast({
              type: 'error',
              title: S.dataExport.failed.title,
              message: S.dataExport.failed.messageGeneric,
              duration: 0,
            })
          } else {
            addToast({
              type: 'error',
              title: S.dataExport.failed.title,
              message: status.error ?? 'Unbekannter Fehler',
              duration: 0,
            })
          }
          return
        }
      }
      addToast({ type: 'error', title: S.dataExport.timeout.title, message: S.dataExport.timeout.message, duration: 0 })
    } catch (e) {
      if (e instanceof DOMException && e.name === 'AbortError') return
      if (!abort.signal.aborted) {
        addToast({ type: 'error', title: S.dataExport.failed.title, duration: 0 })
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
      deleteMut.mutate({ confirmation: deleteConfirmation })
    }
  }

  return (
    <Section title="Gefahrenzone" danger>
      <div className="space-y-4">
        {/* Data Export */}
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100">Daten exportieren</p>
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              Alle deine Daten als ZIP-Datei herunterladen (DSGVO Art. 20).
            </p>
          </div>
          <button
            onClick={handleExport}
            disabled={exporting}
            className="px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 text-sm text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {exporting ? 'Wird exportiert...' : 'Exportieren'}
          </button>
        </div>

        <div className="border-t border-red-200 dark:border-red-800/60" />

        {/* Delete Account */}
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium text-red-700 dark:text-red-400">Account löschen</p>
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              Dein Account wird in 30 Tagen endgültig gelöscht.
            </p>
          </div>
          <button
            onClick={() => setDeleteDialogOpen(true)}
            className="px-3 py-1.5 rounded-lg border border-red-300 dark:border-red-800 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-950/30 transition-colors cursor-pointer"
          >
            Account löschen
          </button>
        </div>
      </div>

      {deleteDialogOpen && (
        <ModalShell onClose={() => setDeleteDialogOpen(false)}>
          <h3 className="text-lg font-semibold text-red-700 dark:text-red-400 mb-2">Account löschen</h3>
          <div className="p-3 rounded-lg bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-800 mb-4">
            <p className="text-sm text-red-700 dark:text-red-400">
              Dein Account wird in 30 Tagen gelöscht. Du erhältst eine E-Mail zum Abbrechen.
              Dieser Vorgang kann innerhalb der 30-Tage-Frist rückgängig gemacht werden.
            </p>
          </div>
          <form onSubmit={handleDeleteSubmit} className="space-y-4">
            {hasPassword ? (
              <div className="space-y-1">
                <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Passwort bestätigen</label>
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
                  Tippe <span className="font-mono font-bold">ACCOUNT LÖSCHEN</span> zur Bestätigung
                </label>
                <input
                  type="text"
                  value={deleteConfirmation}
                  onChange={e => setDeleteConfirmation(e.target.value)}
                  className={AUTH_INPUT_CLASS}
                  required
                  autoFocus
                  placeholder="ACCOUNT LÖSCHEN"
                />
              </div>
            )}
            <div className="flex gap-3 justify-end">
              <button
                type="button"
                onClick={() => setDeleteDialogOpen(false)}
                className="px-4 py-2 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
              >
                Abbrechen
              </button>
              <button
                type="submit"
                disabled={deleteMut.isPending || (!hasPassword && deleteConfirmation !== 'ACCOUNT LÖSCHEN')}
                className="px-4 py-2 rounded-lg bg-red-600 text-white text-sm font-medium hover:bg-red-700 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {deleteMut.isPending ? 'Wird gelöscht...' : 'Unwiderruflich löschen'}
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
      <EmailSection />
      <PasswordSection />
      <SessionsSection />
      <DataImportSection />
      <DangerZoneSection />
    </div>
  )
}
