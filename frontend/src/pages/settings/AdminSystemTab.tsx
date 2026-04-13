import { useState, useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { SettingsSection } from '@/components/ui/SettingsSection'
import Toggle from '@/components/Toggle'
import { useToast } from '@/lib/toast'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'
import {
  getAdminSettings,
  putAdminSettings,
  sendTestEmail,
  type SettingsResponse,
  type SettingField,
} from '@/api/adminSettings'

// ── Helpers ──────────────────────────────────────────────────────────────────

function getField(settings: SettingsResponse | null, section: string, key: string): SettingField {
  return settings?.[section]?.[key] ?? { value: '', source: 'default' }
}

function getValue(settings: SettingsResponse | null, section: string, key: string): string {
  return getField(settings, section, key).value ?? ''
}

function isLocked(settings: SettingsResponse | null, section: string, key: string): boolean {
  return getField(settings, section, key).locked === true
}

const STATUS_BADGE_GREEN = 'inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
const STATUS_BADGE_GRAY = 'inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400'
const STATUS_DOT_GREEN = 'h-2 w-2 rounded-full bg-emerald-500'
const STATUS_DOT_GRAY = 'h-2 w-2 rounded-full bg-neutral-400'

const RESET_LINK_CLASS = 'text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer transition-colors underline-offset-2 hover:underline'
const SAVE_BTN_CLASS = 'px-4 py-2 rounded-lg bg-accent-500 hover:bg-accent-600 text-white text-sm font-medium cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-not-allowed'

const ENV_VAR_NAMES: Record<string, string> = {
  'smtp.host': 'SMTP_HOST',
  'smtp.port': 'SMTP_PORT',
  'smtp.username': 'SMTP_USERNAME',
  'smtp.password': 'SMTP_PASSWORD',
  'smtp.from': 'SMTP_FROM',
  'smtp.tls': 'SMTP_TLS',
  'app.name': 'APP_NAME',
  'app.url': 'APP_URL',
  'registration.mode': 'REGISTRATION_MODE',
  'social.strava.client_id': 'STRAVA_CLIENT_ID',
  'social.strava.client_secret': 'STRAVA_CLIENT_SECRET',
  'social.google.client_id': 'GOOGLE_CLIENT_ID',
  'social.google.client_secret': 'GOOGLE_CLIENT_SECRET',
  'hotfolder.mode': 'HOTFOLDER_MODE',
  'library.sync_enabled': 'LIBRARY_SYNC_ENABLED',
  'library.sync_auto_import': 'LIBRARY_SYNC_AUTO_IMPORT',
  'timescale.compression': 'TIMESCALEDB_COMPRESSION',
}

// ── Sub-components ────────────────────────────────────────────────────────────

interface LockedHintProps {
  fieldKey: string
}

function LockedHint({ fieldKey }: LockedHintProps) {
  const varName = ENV_VAR_NAMES[fieldKey] ?? fieldKey.toUpperCase().replace(/\./g, '_')
  return (
    <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-1">
      Gesetzt per Umgebungsvariable — entferne <code className="font-mono">{varName}</code> aus .env um hier zu konfigurieren
    </p>
  )
}

interface LabeledInputProps {
  label: string
  type?: string
  value: string
  onChange: (val: string) => void
  disabled?: boolean
  placeholder?: string
  fieldKey: string
  lockedHint?: boolean
}

function LabeledInput({ label, type = 'text', value, onChange, disabled, placeholder, fieldKey, lockedHint }: LabeledInputProps) {
  return (
    <div className="space-y-1">
      <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{label}</label>
      <input
        type={type}
        value={value}
        onChange={e => onChange(e.target.value)}
        disabled={disabled}
        placeholder={placeholder}
        className={`${AUTH_INPUT_CLASS} ${disabled ? 'opacity-50 cursor-not-allowed' : ''}`}
      />
      {lockedHint && disabled && <LockedHint fieldKey={fieldKey} />}
    </div>
  )
}

interface LabeledSelectProps {
  label: string
  value: string
  onChange: (val: string) => void
  disabled?: boolean
  options: { value: string; label: string }[]
  fieldKey: string
}

function LabeledSelect({ label, value, onChange, disabled, options, fieldKey }: LabeledSelectProps) {
  return (
    <div className="space-y-1">
      <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{label}</label>
      <select
        value={value}
        onChange={e => onChange(e.target.value)}
        disabled={disabled}
        className={`${AUTH_INPUT_CLASS} ${disabled ? 'opacity-50 cursor-not-allowed' : ''}`}
      >
        {options.map(opt => (
          <option key={opt.value} value={opt.value}>{opt.label}</option>
        ))}
      </select>
      {disabled && <LockedHint fieldKey={fieldKey} />}
    </div>
  )
}

interface SectionActionsProps {
  onSave: () => void
  onReset: () => void
  saving: boolean
}

function SectionActions({ onSave, onReset, saving }: SectionActionsProps) {
  return (
    <div className="flex items-center justify-between pt-2">
      <button type="button" onClick={onReset} className={RESET_LINK_CLASS}>
        Auf Standard zurücksetzen
      </button>
      <button type="button" onClick={onSave} disabled={saving} className={SAVE_BTN_CLASS}>
        {saving ? 'Speichern…' : 'Speichern'}
      </button>
    </div>
  )
}

// ── Section 1 — Email (SMTP) ──────────────────────────────────────────────────

interface SmtpSectionProps {
  settings: SettingsResponse | null
  onSaved: () => void
}

function SmtpSection({ settings, onSaved }: SmtpSectionProps) {
  const { addToast } = useToast()
  const [host, setHost] = useState('')
  const [port, setPort] = useState('587')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('***')
  const [from, setFrom] = useState('')
  const [tlsMode, setTlsMode] = useState('starttls')
  const [saving, setSaving] = useState(false)
  const [testingEmail, setTestingEmail] = useState(false)

  useEffect(() => {
    if (!settings) return
    setHost(getValue(settings, 'smtp', 'host'))
    setPort(getValue(settings, 'smtp', 'port') || '587')
    setUsername(getValue(settings, 'smtp', 'username'))
    setPassword(getValue(settings, 'smtp', 'password') ? '***' : '')
    setFrom(getValue(settings, 'smtp', 'from'))
    setTlsMode(getValue(settings, 'smtp', 'tls') || 'starttls')
  }, [settings])

  const smtpConfigured = host.trim() !== ''

  async function handleSave() {
    setSaving(true)
    try {
      await putAdminSettings({
        'smtp.host': host || null,
        'smtp.port': port || null,
        'smtp.username': username || null,
        'smtp.password': password === '***' ? '***' : (password || null),
        'smtp.from': from || null,
        'smtp.tls': tlsMode || null,
      })
      addToast({ type: 'success', title: 'Einstellungen gespeichert' })
      onSaved()
    } catch (err) {
      addToast({ type: 'error', title: 'Speichern fehlgeschlagen', message: err instanceof Error ? err.message : undefined, duration: 0 })
    } finally {
      setSaving(false)
    }
  }

  function handleReset() {
    setHost('')
    setPort('587')
    setUsername('')
    setPassword('')
    setFrom('')
    setTlsMode('starttls')
  }

  async function handleTestEmail() {
    setTestingEmail(true)
    try {
      await sendTestEmail()
      addToast({ type: 'success', title: 'Test-E-Mail gesendet', message: 'Überprüfe deinen Posteingang' })
    } catch (err) {
      addToast({ type: 'error', title: 'Test-E-Mail fehlgeschlagen', message: err instanceof Error ? err.message : undefined, duration: 0 })
    } finally {
      setTestingEmail(false)
    }
  }

  return (
    <SettingsSection
      title="E-Mail (SMTP)"
      description="Ausgehende E-Mails für Verifizierung, Einladungen und Benachrichtigungen."
    >
      <div className="flex items-center justify-between">
        <span className={smtpConfigured ? STATUS_BADGE_GREEN : STATUS_BADGE_GRAY}>
          <span className={smtpConfigured ? STATUS_DOT_GREEN : STATUS_DOT_GRAY} />
          {smtpConfigured ? 'Konfiguriert' : 'Nicht konfiguriert'}
        </span>
        <button
          type="button"
          onClick={handleTestEmail}
          disabled={!smtpConfigured || testingEmail}
          className="text-sm text-accent-500 hover:text-accent-600 cursor-pointer transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          {testingEmail ? 'Senden…' : 'Test-E-Mail senden'}
        </button>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <LabeledInput
          label="Host"
          value={host}
          onChange={setHost}
          placeholder="smtp.example.com"
          disabled={isLocked(settings, 'smtp', 'host')}
          fieldKey="smtp.host"
          lockedHint
        />
        <LabeledInput
          label="Port"
          type="number"
          value={port}
          onChange={setPort}
          placeholder="587"
          disabled={isLocked(settings, 'smtp', 'port')}
          fieldKey="smtp.port"
          lockedHint
        />
        <LabeledInput
          label="Benutzername"
          value={username}
          onChange={setUsername}
          placeholder="user@example.com"
          disabled={isLocked(settings, 'smtp', 'username')}
          fieldKey="smtp.username"
          lockedHint
        />
        <LabeledInput
          label="Passwort"
          type="password"
          value={password}
          onChange={setPassword}
          placeholder="••••••••"
          disabled={isLocked(settings, 'smtp', 'password')}
          fieldKey="smtp.password"
          lockedHint
        />
        <LabeledInput
          label="Absender (From)"
          type="email"
          value={from}
          onChange={setFrom}
          placeholder="noreply@example.com"
          disabled={isLocked(settings, 'smtp', 'from')}
          fieldKey="smtp.from"
          lockedHint
        />
        <LabeledSelect
          label="TLS-Modus"
          value={tlsMode}
          onChange={setTlsMode}
          disabled={isLocked(settings, 'smtp', 'tls')}
          fieldKey="smtp.tls"
          options={[
            { value: 'starttls', label: 'STARTTLS' },
            { value: 'ssl', label: 'SSL/TLS' },
            { value: 'none', label: 'Keine Verschlüsselung' },
          ]}
        />
      </div>

      <SectionActions onSave={handleSave} onReset={handleReset} saving={saving} />
    </SettingsSection>
  )
}

// ── Section 2 — Registrierung & App ──────────────────────────────────────────

interface RegistrationSectionProps {
  settings: SettingsResponse | null
  smtpConfigured: boolean
  onSaved: () => void
}

function RegistrationSection({ settings, smtpConfigured, onSaved }: RegistrationSectionProps) {
  const { addToast } = useToast()
  const [registrationMode, setRegistrationMode] = useState('invite')
  const [appName, setAppName] = useState('')
  const [appUrl, setAppUrl] = useState('')
  const [originalAppUrl, setOriginalAppUrl] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!settings) return
    setRegistrationMode(getValue(settings, 'registration', 'mode') || 'invite')
    setAppName(getValue(settings, 'app', 'name'))
    const url = getValue(settings, 'app', 'url')
    setAppUrl(url)
    setOriginalAppUrl(url)
  }, [settings])

  const appUrlChanged = appUrl !== originalAppUrl

  async function handleSave() {
    setSaving(true)
    try {
      await putAdminSettings({
        'registration.mode': registrationMode || null,
        'app.name': appName || null,
        'app.url': appUrl || null,
      })
      addToast({ type: 'success', title: 'Einstellungen gespeichert' })
      setOriginalAppUrl(appUrl)
      onSaved()
    } catch (err) {
      addToast({ type: 'error', title: 'Speichern fehlgeschlagen', message: err instanceof Error ? err.message : undefined, duration: 0 })
    } finally {
      setSaving(false)
    }
  }

  function handleReset() {
    setRegistrationMode('invite')
    setAppName('')
    setAppUrl('')
  }

  const REGISTRATION_MODES = [
    { value: 'invite', label: 'Einladung', description: 'Nur per Einladungslink registrieren' },
    { value: 'open', label: 'Offen', description: 'Jeder kann sich registrieren' },
    { value: 'approval', label: 'Genehmigung', description: 'Registrierung erfordert Admin-Freigabe' },
  ]

  return (
    <SettingsSection
      title="Registrierung & App"
      description="Kontrolliere wie neue Benutzer Zugang erhalten und grundlegende App-Einstellungen."
    >
      <div className="space-y-3">
        <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Registrierungsmodus</label>
        <div className="space-y-2">
          {REGISTRATION_MODES.map(mode => (
            <label
              key={mode.value}
              className={`flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-colors ${
                registrationMode === mode.value
                  ? 'border-accent-400 bg-accent-50 dark:bg-accent-900/20 dark:border-accent-600'
                  : 'border-neutral-200 dark:border-neutral-700 hover:border-neutral-300 dark:hover:border-neutral-600'
              } ${isLocked(settings, 'registration', 'mode') ? 'opacity-50 cursor-not-allowed' : ''}`}
            >
              <input
                type="radio"
                name="registration_mode"
                value={mode.value}
                checked={registrationMode === mode.value}
                onChange={() => setRegistrationMode(mode.value)}
                disabled={isLocked(settings, 'registration', 'mode')}
                className="mt-0.5 accent-accent-500"
              />
              <div>
                <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{mode.label}</span>
                <p className="text-xs text-neutral-500 dark:text-neutral-400">{mode.description}</p>
              </div>
            </label>
          ))}
        </div>
        {isLocked(settings, 'registration', 'mode') && (
          <LockedHint fieldKey="registration.mode" />
        )}
        {registrationMode === 'open' && !smtpConfigured && (
          <div className="flex items-start gap-2 p-3 rounded-lg bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-700">
            <span className="text-amber-500 flex-shrink-0 mt-0.5">⚠</span>
            <p className="text-sm text-amber-700 dark:text-amber-400">
              Offene Registrierung erfordert E-Mail-Verifizierung — SMTP zuerst konfigurieren
            </p>
          </div>
        )}
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-2">
        <LabeledInput
          label="App-Name"
          value={appName}
          onChange={setAppName}
          placeholder="Skadia"
          disabled={isLocked(settings, 'app', 'name')}
          fieldKey="app.name"
          lockedHint
        />
        <div className="space-y-1">
          <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">App-URL</label>
          <input
            type="text"
            value={appUrl}
            onChange={e => setAppUrl(e.target.value)}
            placeholder="https://skadia.example.com"
            disabled={isLocked(settings, 'app', 'url')}
            className={`${AUTH_INPUT_CLASS} ${isLocked(settings, 'app', 'url') ? 'opacity-50 cursor-not-allowed' : ''}`}
          />
          {isLocked(settings, 'app', 'url') && <LockedHint fieldKey="app.url" />}
          {appUrlChanged && !isLocked(settings, 'app', 'url') && (
            <div className="flex items-start gap-2 p-2 rounded-lg bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-700 mt-1">
              <span className="text-amber-500 flex-shrink-0">⚠</span>
              <p className="text-xs text-amber-700 dark:text-amber-400">
                OAuth-Redirect-URIs bei Strava/Google aktualisieren
              </p>
            </div>
          )}
        </div>
      </div>

      <SectionActions onSave={handleSave} onReset={handleReset} saving={saving} />
    </SettingsSection>
  )
}

// ── Section 3 — Social Login ──────────────────────────────────────────────────

interface ProviderBlockProps {
  name: string
  clientId: string
  clientSecret: string
  onClientIdChange: (val: string) => void
  onClientSecretChange: (val: string) => void
  clientIdKey: string
  clientSecretKey: string
  clientIdLocked: boolean
  clientSecretLocked: boolean
}

function ProviderBlock({
  name,
  clientId,
  clientSecret,
  onClientIdChange,
  onClientSecretChange,
  clientIdKey,
  clientSecretKey,
  clientIdLocked,
  clientSecretLocked,
}: ProviderBlockProps) {
  const [open, setOpen] = useState(false)
  const active = clientId.trim() !== '' && clientSecret.trim() !== '' && clientSecret !== '***' || (clientId.trim() !== '' && clientSecret === '***')

  return (
    <div className="border border-neutral-200 dark:border-neutral-700 rounded-lg overflow-hidden">
      <button
        type="button"
        onClick={() => setOpen(v => !v)}
        className="w-full flex items-center justify-between px-4 py-3 cursor-pointer hover:bg-neutral-50 dark:hover:bg-neutral-800/40 transition-colors text-left"
      >
        <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{name}</span>
        <div className="flex items-center gap-3">
          <span className={active ? STATUS_BADGE_GREEN : STATUS_BADGE_GRAY}>
            <span className={active ? STATUS_DOT_GREEN : STATUS_DOT_GRAY} />
            {active ? 'Aktiv' : 'Inaktiv'}
          </span>
          <span className={`text-neutral-400 transition-transform duration-200 ${open ? 'rotate-180' : ''}`}>▾</span>
        </div>
      </button>
      {open && (
        <div className="px-4 pb-4 pt-2 space-y-3 border-t border-neutral-100 dark:border-neutral-800">
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Client ID</label>
            <input
              type="text"
              value={clientId}
              onChange={e => onClientIdChange(e.target.value)}
              disabled={clientIdLocked}
              placeholder="Client ID"
              className={`${AUTH_INPUT_CLASS} ${clientIdLocked ? 'opacity-50 cursor-not-allowed' : ''}`}
            />
            {clientIdLocked && <LockedHint fieldKey={clientIdKey} />}
          </div>
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Client Secret</label>
            <input
              type="password"
              value={clientSecret}
              onChange={e => onClientSecretChange(e.target.value)}
              disabled={clientSecretLocked}
              placeholder="Client Secret"
              className={`${AUTH_INPUT_CLASS} ${clientSecretLocked ? 'opacity-50 cursor-not-allowed' : ''}`}
            />
            {clientSecretLocked && <LockedHint fieldKey={clientSecretKey} />}
          </div>
        </div>
      )}
    </div>
  )
}

interface SocialLoginSectionProps {
  settings: SettingsResponse | null
  onSaved: () => void
}

function SocialLoginSection({ settings, onSaved }: SocialLoginSectionProps) {
  const { addToast } = useToast()
  const [stravaClientId, setStravaClientId] = useState('')
  const [stravaClientSecret, setStravaClientSecret] = useState('***')
  const [googleClientId, setGoogleClientId] = useState('')
  const [googleClientSecret, setGoogleClientSecret] = useState('***')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!settings) return
    setStravaClientId(getValue(settings, 'social', 'strava.client_id'))
    setStravaClientSecret(getValue(settings, 'social', 'strava.client_secret') ? '***' : '')
    setGoogleClientId(getValue(settings, 'social', 'google.client_id'))
    setGoogleClientSecret(getValue(settings, 'social', 'google.client_secret') ? '***' : '')
  }, [settings])

  async function handleSave() {
    setSaving(true)
    try {
      await putAdminSettings({
        'social.strava.client_id': stravaClientId || null,
        'social.strava.client_secret': stravaClientSecret === '***' ? '***' : (stravaClientSecret || null),
        'social.google.client_id': googleClientId || null,
        'social.google.client_secret': googleClientSecret === '***' ? '***' : (googleClientSecret || null),
      })
      addToast({ type: 'success', title: 'Einstellungen gespeichert' })
      onSaved()
    } catch (err) {
      addToast({ type: 'error', title: 'Speichern fehlgeschlagen', message: err instanceof Error ? err.message : undefined, duration: 0 })
    } finally {
      setSaving(false)
    }
  }

  function handleReset() {
    setStravaClientId('')
    setStravaClientSecret('')
    setGoogleClientId('')
    setGoogleClientSecret('')
  }

  return (
    <SettingsSection
      title="Social Login"
      description="OAuth-Provider für die Anmeldung mit externen Konten."
    >
      <div className="space-y-3">
        <ProviderBlock
          name="Strava"
          clientId={stravaClientId}
          clientSecret={stravaClientSecret}
          onClientIdChange={setStravaClientId}
          onClientSecretChange={setStravaClientSecret}
          clientIdKey="social.strava.client_id"
          clientSecretKey="social.strava.client_secret"
          clientIdLocked={isLocked(settings, 'social', 'strava.client_id')}
          clientSecretLocked={isLocked(settings, 'social', 'strava.client_secret')}
        />
        <ProviderBlock
          name="Google"
          clientId={googleClientId}
          clientSecret={googleClientSecret}
          onClientIdChange={setGoogleClientId}
          onClientSecretChange={setGoogleClientSecret}
          clientIdKey="social.google.client_id"
          clientSecretKey="social.google.client_secret"
          clientIdLocked={isLocked(settings, 'social', 'google.client_id')}
          clientSecretLocked={isLocked(settings, 'social', 'google.client_secret')}
        />
      </div>

      <SectionActions onSave={handleSave} onReset={handleReset} saving={saving} />
    </SettingsSection>
  )
}

// ── Section 4 — Dateisystem & Datenbank ──────────────────────────────────────

interface FilesystemSectionProps {
  settings: SettingsResponse | null
  onSaved: () => void
}

function FilesystemSection({ settings, onSaved }: FilesystemSectionProps) {
  const { addToast } = useToast()
  const [hotfolderMode, setHotfolderMode] = useState('disabled')
  const [librarySync, setLibrarySync] = useState(false)
  const [librarySyncAutoImport, setLibrarySyncAutoImport] = useState(false)
  const [timescaleCompression, setTimescaleCompression] = useState(false)
  const [showDecompressConfirm, setShowDecompressConfirm] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!settings) return
    setHotfolderMode(getValue(settings, 'hotfolder', 'mode') || 'disabled')
    setLibrarySync(getValue(settings, 'library', 'sync_enabled') === 'true')
    setLibrarySyncAutoImport(getValue(settings, 'library', 'sync_auto_import') === 'true')
    setTimescaleCompression(getValue(settings, 'timescale', 'compression') === 'true')
  }, [settings])

  function handleTimescaleToggle(newVal: boolean) {
    if (!newVal && timescaleCompression) {
      setShowDecompressConfirm(true)
    } else {
      setTimescaleCompression(newVal)
    }
  }

  async function handleSave() {
    setSaving(true)
    try {
      await putAdminSettings({
        'hotfolder.mode': hotfolderMode || null,
        'library.sync_enabled': String(librarySync),
        'library.sync_auto_import': String(librarySyncAutoImport),
        'timescale.compression': String(timescaleCompression),
      })
      addToast({ type: 'success', title: 'Einstellungen gespeichert' })
      onSaved()
    } catch (err) {
      addToast({ type: 'error', title: 'Speichern fehlgeschlagen', message: err instanceof Error ? err.message : undefined, duration: 0 })
    } finally {
      setSaving(false)
    }
  }

  function handleReset() {
    setHotfolderMode('disabled')
    setLibrarySync(false)
    setLibrarySyncAutoImport(false)
    setTimescaleCompression(false)
  }

  return (
    <SettingsSection
      title="Dateisystem & Datenbank"
      description="Hotfolder-Verhalten, Bibliotheks-Sync und Datenbank-Optimierungen."
    >
      {showDecompressConfirm && (
        <div className="p-4 rounded-lg border border-amber-200 dark:border-amber-700 bg-amber-50 dark:bg-amber-900/20 space-y-3">
          <p className="text-sm text-amber-800 dark:text-amber-300 font-medium">
            Dekomprimierung kann bei großen Datenmengen sehr lange dauern. Fortfahren?
          </p>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => { setTimescaleCompression(false); setShowDecompressConfirm(false) }}
              className="px-3 py-1.5 rounded-lg bg-amber-600 hover:bg-amber-700 text-white text-sm font-medium cursor-pointer transition-colors"
            >
              Ja, deaktivieren
            </button>
            <button
              type="button"
              onClick={() => setShowDecompressConfirm(false)}
              className="px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 text-sm font-medium cursor-pointer transition-colors hover:bg-neutral-50 dark:hover:bg-neutral-800"
            >
              Abbrechen
            </button>
          </div>
        </div>
      )}

      <LabeledSelect
        label="Hotfolder-Modus"
        value={hotfolderMode}
        onChange={setHotfolderMode}
        disabled={isLocked(settings, 'hotfolder', 'mode')}
        fieldKey="hotfolder.mode"
        options={[
          { value: 'disabled', label: 'Deaktiviert' },
          { value: 'admin-only', label: 'Nur Admin' },
          { value: 'self-service', label: 'Self-Service' },
        ]}
      />

      <div className="space-y-3 pt-2">
        {(() => {
          const isS3 = settings?._meta?.storage_backend === 's3'
          return (
            <>
              <div className="flex items-center justify-between gap-4 py-1">
                <div className="min-w-0 flex-1">
                  <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">Library Sync</span>
                  <p className="text-xs text-neutral-500 dark:text-neutral-400">Bibliotheksverzeichnis auf Dateiänderungen überwachen</p>
                  {isS3 && (
                    <p className="text-xs text-amber-600 dark:text-amber-400 mt-0.5">Nur bei lokalem Storage verfügbar (S3 aktiv)</p>
                  )}
                  {isLocked(settings, 'library', 'sync_enabled') && (
                    <LockedHint fieldKey="library.sync_enabled" />
                  )}
                </div>
                <Toggle
                  checked={librarySync}
                  onChange={setLibrarySync}
                  disabled={isS3 || isLocked(settings, 'library', 'sync_enabled')}
                  aria-label="Library Sync aktivieren"
                />
              </div>

              <div className="flex items-center justify-between gap-4 py-1 pl-4 border-l-2 border-neutral-200 dark:border-neutral-700">
                <div className="min-w-0 flex-1">
                  <span className={`text-sm font-medium ${(!librarySync || isS3) ? 'text-neutral-400 dark:text-neutral-600' : 'text-neutral-900 dark:text-neutral-100'}`}>
                    Library Sync Auto-Import
                  </span>
                  <p className="text-xs text-neutral-500 dark:text-neutral-400">Neue Dateien im Bibliotheksverzeichnis automatisch importieren</p>
                  {isS3 && (
                    <p className="text-xs text-amber-600 dark:text-amber-400 mt-0.5">Nur bei lokalem Storage verfügbar (S3 aktiv)</p>
                  )}
                  {isLocked(settings, 'library', 'sync_auto_import') && (
                    <LockedHint fieldKey="library.sync_auto_import" />
                  )}
                </div>
                <Toggle
                  checked={librarySyncAutoImport}
                  onChange={setLibrarySyncAutoImport}
                  disabled={isS3 || !librarySync || isLocked(settings, 'library', 'sync_auto_import')}
                  aria-label="Library Sync Auto-Import aktivieren"
                />
              </div>
            </>
          )
        })()}

        <div className="flex items-center justify-between gap-4 py-1">
          <div className="min-w-0 flex-1">
            <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">TimescaleDB-Kompression</span>
            <p className="text-xs text-neutral-500 dark:text-neutral-400">Aktivitätsstreams komprimieren (empfohlen bei großen Datenmengen)</p>
            {isLocked(settings, 'timescale', 'compression') && (
              <LockedHint fieldKey="timescale.compression" />
            )}
          </div>
          <Toggle
            checked={timescaleCompression}
            onChange={handleTimescaleToggle}
            disabled={isLocked(settings, 'timescale', 'compression')}
            aria-label="TimescaleDB-Kompression aktivieren"
          />
        </div>
      </div>

      <SectionActions onSave={handleSave} onReset={handleReset} saving={saving} />
    </SettingsSection>
  )
}

// ── Main Component ────────────────────────────────────────────────────────────

export default function AdminSystemTab() {
  const [settings, setSettings] = useState<SettingsResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const queryClient = useQueryClient()

  function loadSettings() {
    setLoadError(null)
    getAdminSettings()
      .then(data => {
        setSettings(data)
        queryClient.invalidateQueries({ queryKey: ['admin-settings'] })
      })
      .catch(err => setLoadError(err instanceof Error ? err.message : 'Laden fehlgeschlagen'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    loadSettings()
  }, [])

  const smtpConfigured = (settings?.smtp?.host?.value ?? '').trim() !== ''

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16 text-neutral-400 text-sm">
        Einstellungen werden geladen…
      </div>
    )
  }

  if (loadError) {
    return (
      <div className="flex flex-col items-center justify-center py-16 gap-3">
        <p className="text-red-500 text-sm">{loadError}</p>
        <button
          type="button"
          onClick={loadSettings}
          className="text-sm text-accent-500 hover:text-accent-600 cursor-pointer transition-colors"
        >
          Erneut versuchen
        </button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <SmtpSection settings={settings} onSaved={loadSettings} />
      <RegistrationSection settings={settings} smtpConfigured={smtpConfigured} onSaved={loadSettings} />
      <SocialLoginSection settings={settings} onSaved={loadSettings} />
      <FilesystemSection settings={settings} onSaved={loadSettings} />
    </div>
  )
}
