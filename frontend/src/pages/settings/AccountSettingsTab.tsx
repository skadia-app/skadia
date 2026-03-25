import { useState } from 'react'
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
  getDataExport,
  type UserSession,
} from '@/api/auth'

// ── Helpers ─────────────────────────────────────────────────────────────────

function parseUserAgent(ua: string): string {
  // Basic browser detection without ua-parser-js
  if (/Firefox\//i.test(ua)) {
    const m = ua.match(/Firefox\/([\d.]+)/)
    return m ? `Firefox ${m[1]}` : 'Firefox'
  }
  if (/Edg\//i.test(ua)) {
    const m = ua.match(/Edg\/([\d.]+)/)
    return m ? `Edge ${m[1]}` : 'Edge'
  }
  if (/Chrome\//i.test(ua) && !/Chromium/i.test(ua)) {
    const m = ua.match(/Chrome\/([\d.]+)/)
    return m ? `Chrome ${m[1]}` : 'Chrome'
  }
  if (/Safari\//i.test(ua) && !/Chrome/i.test(ua)) {
    const m = ua.match(/Version\/([\d.]+)/)
    return m ? `Safari ${m[1]}` : 'Safari'
  }
  // Truncate unknown UAs
  return ua.length > 60 ? ua.slice(0, 57) + '...' : ua
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleDateString('de-DE', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}

// ── Section wrapper ─────────────────────────────────────────────────────────

function Section({ title, description, children, danger }: {
  title: string
  description?: string
  children: React.ReactNode
  danger?: boolean
}) {
  return (
    <section className={`rounded-xl p-6 space-y-4 border ${
      danger
        ? 'border-red-300 dark:border-red-800'
        : 'border-neutral-200 dark:border-neutral-800'
    }`}>
      <div>
        <h2 className={`text-lg font-medium ${
          danger
            ? 'text-red-700 dark:text-red-400'
            : 'text-neutral-900 dark:text-neutral-100'
        }`}>{title}</h2>
        {description && (
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">{description}</p>
        )}
      </div>
      {children}
    </section>
  )
}

// ── Profile Section ─────────────────────────────────────────────────────────

function ProfileSection() {
  const { user } = useUser()
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const [username, setUsername] = useState(user?.username ?? '')
  const [displayName, setDisplayName] = useState(user?.display_name ?? '')

  const mutation = useMutation({
    mutationFn: updateMe,
    onSuccess: (updated) => {
      queryClient.setQueryData(['me'], updated)
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
          className="px-4 py-2 rounded-lg bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 text-sm font-medium hover:bg-neutral-800 dark:hover:bg-neutral-200 transition-colors disabled:opacity-50"
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
      if (data.status === 'verification_sent') {
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
          className="px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 text-sm text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
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
                className="px-4 py-2 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
              >
                Abbrechen
              </button>
              <button
                type="submit"
                disabled={mutation.isPending}
                className="px-4 py-2 rounded-lg bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 text-sm font-medium hover:bg-neutral-800 dark:hover:bg-neutral-200 transition-colors disabled:opacity-50"
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
  const { addToast } = useToast()
  const [currentPw, setCurrentPw] = useState('')
  const [newPw, setNewPw] = useState('')
  const [confirmPw, setConfirmPw] = useState('')

  const mutation = useMutation({
    mutationFn: () => changePassword(hasPassword ? currentPw : '', newPw),
    onSuccess: () => {
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
          className="px-4 py-2 rounded-lg bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 text-sm font-medium hover:bg-neutral-800 dark:hover:bg-neutral-200 transition-colors disabled:opacity-50"
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
    const sessions = data?.sessions ?? []
    const others = sessions.filter(s => !s.is_current)
    for (const s of others) {
      await revokeSession(s.family_id).catch(() => {})
    }
    queryClient.invalidateQueries({ queryKey: ['my-sessions'] })
    addToast({ type: 'success', title: `${others.length === 1 ? '1 Session' : `${others.length} Sessions`} beendet` })
    setRevokeTarget(null)
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
                    <td className="py-2 pr-4 text-neutral-500 dark:text-neutral-400">{formatDate(s.created_at)}</td>
                    <td className="py-2 pr-4 text-neutral-500 dark:text-neutral-400">{formatDate(s.expires_at)}</td>
                    <td className="py-2">
                      {!s.is_current && (
                        <button
                          onClick={() => setRevokeTarget(s.family_id)}
                          className="text-xs text-red-600 dark:text-red-400 hover:underline"
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
              className="px-3 py-1.5 rounded-lg border border-red-200 dark:border-red-800 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-950/30 transition-colors"
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
              className="px-4 py-2 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
            >
              Abbrechen
            </button>
            <button
              onClick={() => revokeMut.mutate(revokeTarget)}
              disabled={revokeMut.isPending}
              className="px-4 py-2 rounded-lg bg-red-600 text-white text-sm font-medium hover:bg-red-700 transition-colors disabled:opacity-50"
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
              className="px-4 py-2 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
            >
              Abbrechen
            </button>
            <button
              onClick={handleRevokeAllOthers}
              className="px-4 py-2 rounded-lg bg-red-600 text-white text-sm font-medium hover:bg-red-700 transition-colors"
            >
              Alle beenden
            </button>
          </div>
        </ModalShell>
      )}
    </Section>
  )
}

// ── Danger Zone Section ─────────────────────────────────────────────────────

function DangerZoneSection() {
  const { hasPassword } = useUser()
  const navigate = useNavigate()
  const { addToast } = useToast()
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [deletePw, setDeletePw] = useState('')
  const [deleteConfirmation, setDeleteConfirmation] = useState('')
  const [exporting, setExporting] = useState(false)

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
    setExporting(true)
    try {
      const blob = await getDataExport()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `skadia-export-${new Date().toISOString().slice(0, 10)}.zip`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
      addToast({ type: 'success', title: 'Datenexport heruntergeladen' })
    } catch {
      addToast({ type: 'error', title: 'Datenexport fehlgeschlagen', duration: 0 })
    } finally {
      setExporting(false)
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
            className="px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 text-sm text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors disabled:opacity-50"
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
            className="px-3 py-1.5 rounded-lg border border-red-300 dark:border-red-800 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-950/30 transition-colors"
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
                className="px-4 py-2 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
              >
                Abbrechen
              </button>
              <button
                type="submit"
                disabled={deleteMut.isPending || (!hasPassword && deleteConfirmation !== 'ACCOUNT LÖSCHEN')}
                className="px-4 py-2 rounded-lg bg-red-600 text-white text-sm font-medium hover:bg-red-700 transition-colors disabled:opacity-50"
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
      <DangerZoneSection />
    </div>
  )
}
