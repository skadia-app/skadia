import { useState, useEffect } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { register, getAuthConfig, type AuthConfig } from '@/api/auth'
import { isAuthenticated } from '@/lib/auth'

const INPUT_CLASS =
  'w-full px-4 py-3 rounded-lg border border-neutral-200 dark:border-neutral-700 ' +
  'bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 ' +
  'placeholder:text-neutral-400 text-sm ' +
  'focus:outline-none focus:ring-1 focus:ring-accent-400'

export default function RegisterPage() {
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [success, setSuccess] = useState(false)
  const [config, setConfig] = useState<AuthConfig | null>(null)

  useEffect(() => {
    if (isAuthenticated()) {
      navigate('/activities', { replace: true })
      return
    }
    getAuthConfig()
      .then((cfg) => {
        if (cfg.registration_mode === 'invite') {
          navigate('/login', { replace: true })
          return
        }
        setConfig(cfg)
      })
      .catch(() => {})
  }, [navigate])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)

    if (password.length < 10) {
      setError('Das Passwort muss mindestens 10 Zeichen lang sein.')
      return
    }
    if (password !== confirmPassword) {
      setError('Die Passwörter stimmen nicht überein.')
      return
    }

    setLoading(true)
    try {
      await register(email, username, password)
      setSuccess(true)
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Registrierung fehlgeschlagen'
      setError(msg)
    } finally {
      setLoading(false)
    }
  }

  if (success) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-white dark:bg-neutral-950">
        <div className="w-full max-w-sm space-y-6 px-4">
          <div className="text-center space-y-2">
            <h1 className="text-3xl font-bold text-accent-DEFAULT tracking-tight">skadia</h1>
          </div>
          <div className="p-4 rounded-lg bg-green-50 dark:bg-green-950/30 border border-green-200 dark:border-green-800">
            <p className="text-sm text-green-700 dark:text-green-400 font-medium">
              Registrierung erfolgreich!
            </p>
            <p className="text-sm text-green-600 dark:text-green-500 mt-1">
              Wir haben dir eine Bestätigungs-E-Mail geschickt. Bitte klicke auf den Link in der
              E-Mail, um dein Konto zu aktivieren.
            </p>
          </div>
          <div className="text-center">
            <Link to="/login" className="text-sm text-accent-DEFAULT hover:underline">
              Zur Anmeldung
            </Link>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-white dark:bg-neutral-950">
      <div className="w-full max-w-sm space-y-6 px-4">
        <div className="text-center space-y-2">
          <h1 className="text-3xl font-bold text-accent-DEFAULT tracking-tight">skadia</h1>
          <p className="text-sm text-neutral-500 dark:text-neutral-400">Konto erstellen</p>
        </div>

        {error && (
          <div className="p-3 rounded-lg bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-800">
            <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="E-Mail"
            required
            autoComplete="email"
            className={INPUT_CLASS}
          />
          <input
            type="text"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="Benutzername"
            required
            autoComplete="username"
            className={INPUT_CLASS}
          />
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="Passwort (min. 10 Zeichen)"
            required
            minLength={10}
            autoComplete="new-password"
            className={INPUT_CLASS}
          />
          <input
            type="password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            placeholder="Passwort bestätigen"
            required
            autoComplete="new-password"
            className={INPUT_CLASS}
          />
          <button
            type="submit"
            disabled={loading || config === null}
            className="w-full px-4 py-3 rounded-lg bg-neutral-900 dark:bg-neutral-100
                       text-white dark:text-neutral-900 font-medium text-sm
                       hover:bg-neutral-800 dark:hover:bg-neutral-200
                       transition-colors disabled:opacity-50"
          >
            {loading ? 'Wird registriert…' : 'Registrieren'}
          </button>
        </form>

        <div className="text-center text-sm text-neutral-500 dark:text-neutral-400">
          Bereits ein Konto?{' '}
          <Link to="/login" className="font-medium text-accent-DEFAULT hover:underline">
            Anmelden
          </Link>
        </div>
      </div>
    </div>
  )
}
