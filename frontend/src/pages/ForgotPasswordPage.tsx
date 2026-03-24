import { useState } from 'react'
import { Link } from 'react-router-dom'
import { forgotPassword } from '@/api/auth'

const INPUT_CLASS =
  'w-full px-4 py-3 rounded-lg border border-neutral-200 dark:border-neutral-700 ' +
  'bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 ' +
  'placeholder:text-neutral-400 text-sm ' +
  'focus:outline-none focus:ring-1 focus:ring-accent-400'

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState('')
  const [loading, setLoading] = useState(false)
  const [submitted, setSubmitted] = useState(false)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setLoading(true)
    try {
      await forgotPassword(email)
    } catch {
      // Intentionally ignored — always show success for timing-safety
    } finally {
      setLoading(false)
      setSubmitted(true)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-white dark:bg-neutral-950">
      <div className="w-full max-w-sm space-y-6 px-4">
        <div className="text-center space-y-2">
          <h1 className="text-3xl font-bold text-accent-DEFAULT tracking-tight">skadia</h1>
          <p className="text-sm text-neutral-500 dark:text-neutral-400">Passwort zurücksetzen</p>
        </div>

        {submitted ? (
          <>
            <div className="p-4 rounded-lg bg-green-50 dark:bg-green-950/30 border border-green-200 dark:border-green-800">
              <p className="text-sm text-green-700 dark:text-green-400">
                Falls ein Konto mit dieser E-Mail existiert, haben wir dir einen Link zum
                Zurücksetzen geschickt.
              </p>
            </div>
            <div className="text-center">
              <Link to="/login" className="text-sm text-accent-DEFAULT hover:underline">
                Zurück zur Anmeldung
              </Link>
            </div>
          </>
        ) : (
          <>
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
              <button
                type="submit"
                disabled={loading}
                className="w-full px-4 py-3 rounded-lg bg-neutral-900 dark:bg-neutral-100
                           text-white dark:text-neutral-900 font-medium text-sm
                           hover:bg-neutral-800 dark:hover:bg-neutral-200
                           transition-colors disabled:opacity-50"
              >
                {loading ? 'Wird gesendet…' : 'Zurücksetzen'}
              </button>
            </form>

            <div className="text-center">
              <Link to="/login" className="text-sm text-accent-DEFAULT hover:underline">
                Zurück zur Anmeldung
              </Link>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
