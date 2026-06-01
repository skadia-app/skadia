import { useState, useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { SettingsSection } from '@/components/ui/SettingsSection'
import Toggle from '@/components/Toggle'
import { useToast } from '@/lib/toast'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'
import {
  getAdminSettings,
  getEmailTemplates,
  previewEmailTemplate,
  putAdminSettings,
  resetEmailTemplate,
  sendTestEmail,
  saveEmailTemplate,
  sendEmailTemplateTest,
  type SettingsResponse,
  type SettingField,
  type EmailTemplate,
  type RenderedEmailTemplate,
} from '@/api/adminSettings'

// ── Helpers ──────────────────────────────────────────────────────────────────

function getField(settings: SettingsResponse | null, section: string, key: string): SettingField {
  return settings?.[section]?.[key] ?? { value: '', source: 'default' }
}

function getValue(settings: SettingsResponse | null, section: string, key: string): string {
  return getField(settings, section, key).value ?? ''
}

function isSmtpDeliveryConfigured(host: string, port: string, username: string, password: string, from: string) {
  const hasCredentials = username.trim() !== '' || password !== ''
  const credentialsComplete = !hasCredentials || (username.trim() !== '' && password !== '')
  return host.trim() !== '' && port.trim() !== '' && from.trim() !== '' && credentialsComplete
}

function isLocked(settings: SettingsResponse | null, section: string, key: string): boolean {
  return getField(settings, section, key).locked === true
}

function translateKnownTemplate(t: ReturnType<typeof useTranslation>['t'], key: string): string {
  const translationKey = `settings.adminSystem.emailTemplates.labels.${key}`
  const translated = t(translationKey)
  if (translated !== translationKey) return translated
  return t('settings.adminSystem.emailTemplates.labels.unknown')
}

const STATUS_BADGE_GREEN = 'inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
const STATUS_BADGE_GRAY = 'inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400'
const STATUS_DOT_GREEN = 'h-2 w-2 rounded-full bg-emerald-500'
const STATUS_DOT_GRAY = 'h-2 w-2 rounded-full bg-neutral-400'

const RESET_LINK_CLASS = 'text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer transition-colors underline-offset-2 hover:underline'
const SAVE_BTN_CLASS = 'px-4 py-2 rounded-lg bg-accent-500 hover:bg-accent-600 text-white text-sm font-medium cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-not-allowed'
const SECONDARY_BTN_CLASS = 'px-3 py-1.5 rounded border border-neutral-300 dark:border-neutral-700 text-sm text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed transition-colors'

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
  'social.strava_client_id': 'STRAVA_CLIENT_ID',
  'social.strava_client_secret': 'STRAVA_CLIENT_SECRET',
  'social.google_client_id': 'GOOGLE_CLIENT_ID',
  'social.google_client_secret': 'GOOGLE_CLIENT_SECRET',
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
  const { t } = useTranslation()
  const varName = ENV_VAR_NAMES[fieldKey] ?? fieldKey.toUpperCase().replace(/\./g, '_')
  return (
    <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-1">
      {t('settings.adminSystem.lockedHint.before')} <code className="font-mono">{varName}</code> {t('settings.adminSystem.lockedHint.after')}
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
  const { t } = useTranslation()
  return (
    <div className="flex items-center justify-between pt-2">
      <button type="button" onClick={onReset} className={RESET_LINK_CLASS}>
        {t('settings.adminSystem.actions.reset')}
      </button>
      <button type="button" onClick={onSave} disabled={saving} className={SAVE_BTN_CLASS}>
        {saving ? t('common.saving') : t('common.save')}
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
  const { t } = useTranslation()
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
    setPassword(getField(settings, 'smtp', 'password').has_value ? '***' : '')
    setFrom(getValue(settings, 'smtp', 'from'))
    setTlsMode(getValue(settings, 'smtp', 'tls') || 'starttls')
  }, [settings])

  const smtpConfigured = isSmtpDeliveryConfigured(host, port, username, password, from)

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
      addToast({ type: 'success', title: t('settings.adminSystem.toasts.saved') })
      onSaved()
    } catch (err) {
      addToast({ type: 'error', title: t('settings.adminSystem.toasts.saveFailed'), message: err instanceof Error ? err.message : undefined, duration: 0 })
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
      addToast({ type: 'success', title: t('settings.adminSystem.smtp.testSent'), message: t('settings.adminSystem.smtp.checkInbox') })
    } catch (err) {
      addToast({ type: 'error', title: t('settings.adminSystem.smtp.testFailed'), message: err instanceof Error ? err.message : undefined, duration: 0 })
    } finally {
      setTestingEmail(false)
    }
  }

  return (
    <SettingsSection
      title={t('settings.adminSystem.smtp.title')}
      description={t('settings.adminSystem.smtp.description')}
    >
      <div className="flex items-center justify-between">
        <span className={smtpConfigured ? STATUS_BADGE_GREEN : STATUS_BADGE_GRAY}>
          <span className={smtpConfigured ? STATUS_DOT_GREEN : STATUS_DOT_GRAY} />
          {smtpConfigured ? t('common.configured') : t('common.notConfigured')}
        </span>
        <button
          type="button"
          onClick={handleTestEmail}
          disabled={!smtpConfigured || testingEmail}
          className="text-sm text-accent-500 hover:text-accent-600 cursor-pointer transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          {testingEmail ? t('settings.adminSystem.smtp.sending') : t('settings.adminSystem.smtp.sendTest')}
        </button>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <LabeledInput
          label={t('settings.adminSystem.smtp.host')}
          value={host}
          onChange={setHost}
          placeholder="smtp.example.com"
          disabled={isLocked(settings, 'smtp', 'host')}
          fieldKey="smtp.host"
          lockedHint
        />
        <LabeledInput
          label={t('settings.adminSystem.smtp.port')}
          type="number"
          value={port}
          onChange={setPort}
          placeholder="587"
          disabled={isLocked(settings, 'smtp', 'port')}
          fieldKey="smtp.port"
          lockedHint
        />
        <LabeledInput
          label={t('settings.adminSystem.smtp.username')}
          value={username}
          onChange={setUsername}
          placeholder="user@example.com"
          disabled={isLocked(settings, 'smtp', 'username')}
          fieldKey="smtp.username"
          lockedHint
        />
        <LabeledInput
          label={t('settings.adminSystem.smtp.password')}
          type="password"
          value={password}
          onChange={setPassword}
          placeholder="••••••••"
          disabled={isLocked(settings, 'smtp', 'password')}
          fieldKey="smtp.password"
          lockedHint
        />
        <LabeledInput
          label={t('settings.adminSystem.smtp.from')}
          type="email"
          value={from}
          onChange={setFrom}
          placeholder="noreply@example.com"
          disabled={isLocked(settings, 'smtp', 'from')}
          fieldKey="smtp.from"
          lockedHint
        />
        <LabeledSelect
          label={t('settings.adminSystem.smtp.tlsMode')}
          value={tlsMode}
          onChange={setTlsMode}
          disabled={isLocked(settings, 'smtp', 'tls')}
          fieldKey="smtp.tls"
          options={[
            { value: 'starttls', label: 'STARTTLS' },
            { value: 'ssl', label: 'SSL/TLS' },
            { value: 'none', label: t('settings.adminSystem.smtp.noEncryption') },
          ]}
        />
      </div>

      <SectionActions onSave={handleSave} onReset={handleReset} saving={saving} />
    </SettingsSection>
  )
}

// ── Section 2 — Email templates ──────────────────────────────────────────────

interface EmailTemplatesSectionProps {
  smtpConfigured: boolean
}

function EmailTemplatesSection({ smtpConfigured }: EmailTemplatesSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const { data: templates = [], isLoading } = useQuery({
    queryKey: ['admin-email-templates'],
    queryFn: getEmailTemplates,
    staleTime: 60_000,
  })
  const [selectedKey, setSelectedKey] = useState('')
  const [subject, setSubject] = useState('')
  const [html, setHtml] = useState('')
  const [text, setText] = useState('')
  const [preview, setPreview] = useState<RenderedEmailTemplate | null>(null)
  const [busy, setBusy] = useState(false)

  const selected = templates.find(t => t.key === selectedKey) ?? templates[0]

  useEffect(() => {
    if (!selectedKey && templates.length > 0) setSelectedKey(templates[0].key)
  }, [selectedKey, templates])

  useEffect(() => {
    if (!selected) return
    setSubject(selected.subject)
    setHtml(selected.html)
    setText(selected.text)
    setPreview(null)
  }, [selected])

  async function handleSave() {
    if (!selected) return
    setBusy(true)
    try {
      await saveEmailTemplate(selected.key, { subject, html, text, enabled: true })
      await queryClient.invalidateQueries({ queryKey: ['admin-email-templates'] })
      addToast({ type: 'success', title: t('settings.adminSystem.emailTemplates.toasts.saved') })
    } catch (err) {
      addToast({ type: 'error', title: t('settings.adminSystem.toasts.saveFailed'), message: err instanceof Error ? err.message : undefined, duration: 0 })
    } finally {
      setBusy(false)
    }
  }

  async function handlePreview() {
    if (!selected) return
    setBusy(true)
    try {
      const rendered = await previewEmailTemplate(selected.key, { subject, html, text, enabled: true })
      setPreview(rendered)
    } catch (err) {
      addToast({ type: 'error', title: t('settings.adminSystem.emailTemplates.toasts.previewFailed'), message: err instanceof Error ? err.message : undefined, duration: 0 })
    } finally {
      setBusy(false)
    }
  }

  async function handleTest() {
    if (!selected) return
    setBusy(true)
    try {
      await sendEmailTemplateTest(selected.key)
      addToast({ type: 'success', title: t('settings.adminSystem.smtp.testSent') })
    } catch (err) {
      addToast({ type: 'error', title: t('settings.adminSystem.smtp.testFailed'), message: err instanceof Error ? err.message : undefined, duration: 0 })
    } finally {
      setBusy(false)
    }
  }

  async function handleReset() {
    if (!selected) return
    setBusy(true)
    try {
      await resetEmailTemplate(selected.key)
      await queryClient.invalidateQueries({ queryKey: ['admin-email-templates'] })
      addToast({ type: 'success', title: t('settings.adminSystem.emailTemplates.toasts.defaultRestored') })
    } catch (err) {
      addToast({ type: 'error', title: t('settings.adminSystem.emailTemplates.toasts.resetFailed'), message: err instanceof Error ? err.message : undefined, duration: 0 })
    } finally {
      setBusy(false)
    }
  }

  function handleHtmlChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
    setHtml(e.currentTarget.value)
  }

  function handleTextChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
    setText(e.currentTarget.value)
  }

  return (
    <SettingsSection
      title={t('settings.adminSystem.emailTemplates.title')}
      description={t('settings.adminSystem.emailTemplates.description')}
    >
      {isLoading || !selected ? (
        <div className="py-8 text-sm text-neutral-500">{t('settings.adminSystem.emailTemplates.loading')}</div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-[220px_1fr] gap-5">
          <div className="space-y-2">
            {templates.map((template: EmailTemplate) => (
              <button
                key={template.key}
                type="button"
                onClick={() => setSelectedKey(template.key)}
                className={`w-full text-left px-3 py-2 rounded border text-sm transition-colors ${
                  selected.key === template.key
                    ? 'border-accent-400 bg-accent-50 text-accent-700 dark:bg-accent-900/20 dark:border-accent-600 dark:text-accent-300'
                    : 'border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800'
                }`}
              >
                <span className="block font-medium">{translateKnownTemplate(t, template.key)}</span>
                <span className="text-xs text-neutral-500">{template.source === 'db' ? t('settings.adminSystem.emailTemplates.customized') : t('settings.adminSystem.emailTemplates.standard')}</span>
              </button>
            ))}
          </div>

          <div className="space-y-4">
            <LabeledInput
              label={t('settings.adminSystem.emailTemplates.subject')}
              value={subject}
              onChange={setSubject}
              fieldKey="email.subject"
            />
            <div className="space-y-1">
              <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">HTML</label>
              <textarea value={html} onChange={handleHtmlChange} rows={8} className={`${AUTH_INPUT_CLASS} font-mono text-xs min-h-48`} />
            </div>
            <div className="space-y-1">
              <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Text</label>
              <textarea value={text} onChange={handleTextChange} rows={6} className={`${AUTH_INPUT_CLASS} font-mono text-xs min-h-36`} />
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <button type="button" onClick={handleSave} disabled={busy} className={SAVE_BTN_CLASS}>{t('common.save')}</button>
              <button type="button" onClick={handlePreview} disabled={busy} className={SECONDARY_BTN_CLASS}>{t('settings.adminSystem.emailTemplates.preview')}</button>
              <button type="button" onClick={handleTest} disabled={busy || !smtpConfigured} className={SECONDARY_BTN_CLASS}>{t('settings.adminSystem.emailTemplates.sendTest')}</button>
              <button type="button" onClick={handleReset} disabled={busy || selected.source !== 'db'} className={RESET_LINK_CLASS}>{t('settings.adminSystem.actions.reset')}</button>
            </div>
            {preview && (
              <div className="space-y-2 border-t border-neutral-200 dark:border-neutral-800 pt-4">
                <div className="text-sm font-medium text-neutral-800 dark:text-neutral-200">{preview.subject}</div>
                <pre className="whitespace-pre-wrap rounded bg-neutral-100 dark:bg-neutral-900 p-3 text-xs text-neutral-700 dark:text-neutral-300 overflow-auto">{preview.text}</pre>
              </div>
            )}
          </div>
        </div>
      )}
    </SettingsSection>
  )
}

// ── Section 3 — Registration and app ─────────────────────────────────────────

interface RegistrationSectionProps {
  settings: SettingsResponse | null
  smtpConfigured: boolean
  onSaved: () => void
}

function RegistrationSection({ settings, smtpConfigured, onSaved }: RegistrationSectionProps) {
  const { t } = useTranslation()
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
      addToast({ type: 'success', title: t('settings.adminSystem.toasts.saved') })
      setOriginalAppUrl(appUrl)
      onSaved()
    } catch (err) {
      addToast({ type: 'error', title: t('settings.adminSystem.toasts.saveFailed'), message: err instanceof Error ? err.message : undefined, duration: 0 })
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
    { value: 'invite', label: t('settings.adminSystem.registration.modes.invite.label'), description: t('settings.adminSystem.registration.modes.invite.description') },
    { value: 'open', label: t('settings.adminSystem.registration.modes.open.label'), description: t('settings.adminSystem.registration.modes.open.description') },
    { value: 'approval', label: t('settings.adminSystem.registration.modes.approval.label'), description: t('settings.adminSystem.registration.modes.approval.description') },
  ]

  return (
    <SettingsSection
      title={t('settings.adminSystem.registration.title')}
      description={t('settings.adminSystem.registration.description')}
    >
      <div className="space-y-3">
        <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('settings.adminSystem.registration.modeLabel')}</label>
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
              {t('settings.adminSystem.registration.openNeedsSmtp')}
            </p>
          </div>
        )}
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-2">
        <LabeledInput
          label={t('settings.adminSystem.registration.appName')}
          value={appName}
          onChange={setAppName}
          placeholder="Skadia"
          disabled={isLocked(settings, 'app', 'name')}
          fieldKey="app.name"
          lockedHint
        />
        <div className="space-y-1">
          <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('settings.adminSystem.registration.appUrl')}</label>
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
                {t('settings.adminSystem.registration.oauthRedirectHint')}
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
  const { t } = useTranslation()
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
            {active ? t('common.active') : t('common.inactive')}
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
  const { t } = useTranslation()
  const { addToast } = useToast()
  const [stravaClientId, setStravaClientId] = useState('')
  const [stravaClientSecret, setStravaClientSecret] = useState('***')
  const [googleClientId, setGoogleClientId] = useState('')
  const [googleClientSecret, setGoogleClientSecret] = useState('***')
  const [saving, setSaving] = useState(false)
  const oauthEncryptionConfigured = settings?._meta?.oauth_encryption_configured !== 'false'

  useEffect(() => {
    if (!settings) return
    setStravaClientId(getValue(settings, 'social', 'strava_client_id'))
    setStravaClientSecret(getField(settings, 'social', 'strava_client_secret').has_value ? '***' : '')
    setGoogleClientId(getValue(settings, 'social', 'google_client_id'))
    setGoogleClientSecret(getField(settings, 'social', 'google_client_secret').has_value ? '***' : '')
  }, [settings])

  async function handleSave() {
    setSaving(true)
    try {
      await putAdminSettings({
        'social.strava_client_id': stravaClientId || null,
        'social.strava_client_secret': stravaClientSecret === '***' ? '***' : (stravaClientSecret || null),
        'social.google_client_id': googleClientId || null,
        'social.google_client_secret': googleClientSecret === '***' ? '***' : (googleClientSecret || null),
      })
      addToast({ type: 'success', title: t('settings.adminSystem.toasts.saved') })
      onSaved()
    } catch (err) {
      addToast({ type: 'error', title: t('settings.adminSystem.toasts.saveFailed'), message: err instanceof Error ? err.message : undefined, duration: 0 })
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
      title={t('settings.adminSystem.social.title')}
      description={t('settings.adminSystem.social.description')}
    >
      <div className="space-y-3">
        {!oauthEncryptionConfigured && (
          <div className="rounded border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-700 dark:bg-amber-950/30 dark:text-amber-200">
            {t('settings.adminSystem.social.missingEncryptionKey')}
          </div>
        )}
        <ProviderBlock
          name="Strava"
          clientId={stravaClientId}
          clientSecret={stravaClientSecret}
          onClientIdChange={setStravaClientId}
          onClientSecretChange={setStravaClientSecret}
          clientIdKey="social.strava_client_id"
          clientSecretKey="social.strava_client_secret"
          clientIdLocked={isLocked(settings, 'social', 'strava_client_id')}
          clientSecretLocked={isLocked(settings, 'social', 'strava_client_secret')}
        />
        <ProviderBlock
          name="Google"
          clientId={googleClientId}
          clientSecret={googleClientSecret}
          onClientIdChange={setGoogleClientId}
          onClientSecretChange={setGoogleClientSecret}
          clientIdKey="social.google_client_id"
          clientSecretKey="social.google_client_secret"
          clientIdLocked={isLocked(settings, 'social', 'google_client_id')}
          clientSecretLocked={isLocked(settings, 'social', 'google_client_secret')}
        />
      </div>

      <SectionActions onSave={handleSave} onReset={handleReset} saving={saving} />
    </SettingsSection>
  )
}

// ── Section 4 — Filesystem and database ──────────────────────────────────────

interface FilesystemSectionProps {
  settings: SettingsResponse | null
  onSaved: () => void
}

function FilesystemSection({ settings, onSaved }: FilesystemSectionProps) {
  const { t } = useTranslation()
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
      addToast({ type: 'success', title: t('settings.adminSystem.toasts.saved') })
      onSaved()
    } catch (err) {
      addToast({ type: 'error', title: t('settings.adminSystem.toasts.saveFailed'), message: err instanceof Error ? err.message : undefined, duration: 0 })
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
      title={t('settings.adminSystem.filesystem.title')}
      description={t('settings.adminSystem.filesystem.description')}
    >
      {showDecompressConfirm && (
        <div className="p-4 rounded-lg border border-amber-200 dark:border-amber-700 bg-amber-50 dark:bg-amber-900/20 space-y-3">
          <p className="text-sm text-amber-800 dark:text-amber-300 font-medium">
            {t('settings.adminSystem.filesystem.decompressWarning')}
          </p>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => { setTimescaleCompression(false); setShowDecompressConfirm(false) }}
              className="px-3 py-1.5 rounded-lg bg-amber-600 hover:bg-amber-700 text-white text-sm font-medium cursor-pointer transition-colors"
            >
              {t('settings.adminSystem.filesystem.confirmDisable')}
            </button>
            <button
              type="button"
              onClick={() => setShowDecompressConfirm(false)}
              className="px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 text-sm font-medium cursor-pointer transition-colors hover:bg-neutral-50 dark:hover:bg-neutral-800"
            >
              {t('common.cancel')}
            </button>
          </div>
        </div>
      )}

      <LabeledSelect
        label={t('settings.adminSystem.filesystem.hotfolderMode')}
        value={hotfolderMode}
        onChange={setHotfolderMode}
        disabled={isLocked(settings, 'hotfolder', 'mode')}
        fieldKey="hotfolder.mode"
        options={[
          { value: 'disabled', label: t('common.disabled') },
          { value: 'admin-only', label: t('settings.adminSystem.filesystem.adminOnly') },
          { value: 'self-service', label: t('settings.adminSystem.filesystem.selfService') },
        ]}
      />

      <div className="space-y-3 pt-2">
        {(() => {
          const isS3 = settings?._meta?.storage_backend === 's3'
          return (
            <>
              <div className="flex items-center justify-between gap-4 py-1">
                <div className="min-w-0 flex-1">
                  <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{t('settings.adminSystem.filesystem.librarySync')}</span>
                  <p className="text-xs text-neutral-500 dark:text-neutral-400">{t('settings.adminSystem.filesystem.librarySyncDescription')}</p>
                  {isS3 && (
                    <p className="text-xs text-amber-600 dark:text-amber-400 mt-0.5">{t('settings.adminSystem.filesystem.localStorageOnly')}</p>
                  )}
                  {isLocked(settings, 'library', 'sync_enabled') && (
                    <LockedHint fieldKey="library.sync_enabled" />
                  )}
                </div>
                <Toggle
                  checked={librarySync}
                  onChange={setLibrarySync}
                  disabled={isS3 || isLocked(settings, 'library', 'sync_enabled')}
                  aria-label={t('settings.adminSystem.filesystem.enableLibrarySync')}
                />
              </div>

              <div className="flex items-center justify-between gap-4 py-1 pl-4 border-l-2 border-neutral-200 dark:border-neutral-700">
                <div className="min-w-0 flex-1">
                  <span className={`text-sm font-medium ${(!librarySync || isS3) ? 'text-neutral-400 dark:text-neutral-600' : 'text-neutral-900 dark:text-neutral-100'}`}>
                    {t('settings.adminSystem.filesystem.librarySyncAutoImport')}
                  </span>
                  <p className="text-xs text-neutral-500 dark:text-neutral-400">{t('settings.adminSystem.filesystem.librarySyncAutoImportDescription')}</p>
                  {isS3 && (
                    <p className="text-xs text-amber-600 dark:text-amber-400 mt-0.5">{t('settings.adminSystem.filesystem.localStorageOnly')}</p>
                  )}
                  {isLocked(settings, 'library', 'sync_auto_import') && (
                    <LockedHint fieldKey="library.sync_auto_import" />
                  )}
                </div>
                <Toggle
                  checked={librarySyncAutoImport}
                  onChange={setLibrarySyncAutoImport}
                  disabled={isS3 || !librarySync || isLocked(settings, 'library', 'sync_auto_import')}
                  aria-label={t('settings.adminSystem.filesystem.enableLibrarySyncAutoImport')}
                />
              </div>
            </>
          )
        })()}

        <div className="flex items-center justify-between gap-4 py-1">
          <div className="min-w-0 flex-1">
            <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{t('settings.adminSystem.filesystem.timescaleCompression')}</span>
            <p className="text-xs text-neutral-500 dark:text-neutral-400">{t('settings.adminSystem.filesystem.timescaleDescription')}</p>
            {isLocked(settings, 'timescale', 'compression') && (
              <LockedHint fieldKey="timescale.compression" />
            )}
          </div>
          <Toggle
            checked={timescaleCompression}
            onChange={handleTimescaleToggle}
            disabled={isLocked(settings, 'timescale', 'compression')}
            aria-label={t('settings.adminSystem.filesystem.enableTimescaleCompression')}
          />
        </div>
      </div>

      <SectionActions onSave={handleSave} onReset={handleReset} saving={saving} />
    </SettingsSection>
  )
}

// ── Main Component ────────────────────────────────────────────────────────────

export default function AdminSystemTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data: settings, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['admin-settings'],
    queryFn: getAdminSettings,
    staleTime: 60_000,
  })

  function handleSaved() {
    queryClient.invalidateQueries({ queryKey: ['admin-settings'] })
  }

  const smtpConfigured = isSmtpDeliveryConfigured(
    settings?.smtp?.host?.value ?? '',
    settings?.smtp?.port?.value ?? '',
    settings?.smtp?.username?.value ?? '',
    settings?.smtp?.password?.has_value ? '***' : (settings?.smtp?.password?.value ?? ''),
    settings?.smtp?.from?.value ?? '',
  )

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-16 text-neutral-400 text-sm">
        {t('settings.adminSystem.loading')}
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex flex-col items-center justify-center py-16 gap-3">
        <p className="text-red-500 text-sm">{error instanceof Error ? error.message : t('settings.adminSystem.loadFailed')}</p>
        <button
          type="button"
          onClick={() => refetch()}
          className="text-sm text-accent-500 hover:text-accent-600 cursor-pointer transition-colors"
        >
          {t('common.retry')}
        </button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <SmtpSection settings={settings ?? null} onSaved={handleSaved} />
      <EmailTemplatesSection smtpConfigured={smtpConfigured} />
      <RegistrationSection settings={settings ?? null} smtpConfigured={smtpConfigured} onSaved={handleSaved} />
      <SocialLoginSection settings={settings ?? null} onSaved={handleSaved} />
      <FilesystemSection settings={settings ?? null} onSaved={handleSaved} />
    </div>
  )
}
