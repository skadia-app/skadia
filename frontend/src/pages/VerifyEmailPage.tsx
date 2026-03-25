import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { verifyEmail } from '@/api/auth'
import { getAuthErrorMessage } from '@/pages/auth/errorMessages'

export default function VerifyEmailPage() {
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') ?? ''

  const [loading, setLoading] = useState(false)
  const [success, setSuccess] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleVerify() {
    if (!token) {
      setError('Ungültiger oder fehlender Bestätigungs-Link.')
      return
    }
    setLoading(true)
    setError(null)
    try {
      await verifyEmail(token)
      setSuccess(true)
    } catch {
      setError(getAuthErrorMessage('invalid_or_expired_token'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-white dark:bg-neutral-950">
      <div className="w-full max-w-sm space-y-6 px-4">
        <div className="text-center space-y-2">
          <h1 className="text-3xl font-bold text-accent-DEFAULT tracking-tight">skadia</h1>
          <p className="text-sm text-neutral-500 dark:text-neutral-400">E-Mail verifizieren</p>
        </div>

        {success ? (
          <>
            <div className="p-4 rounded-lg bg-green-50 dark:bg-green-950/30 border border-green-200 dark:border-green-800">
              <p className="text-sm text-green-700 dark:text-green-400 font-medium">
                E-Mail erfolgreich verifiziert!
              </p>
              <p className="text-sm text-green-600 dark:text-green-500 mt-1">
                Du kannst dich jetzt anmelden.
              </p>
            </div>
            <div className="text-center">
              <Link to="/login" className="text-sm text-accent-DEFAULT hover:underline">
                Zur Anmeldung
              </Link>
            </div>
          </>
        ) : (
          <>
            {error && (
              <div className="p-3 rounded-lg bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-800">
                <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
              </div>
            )}

            {!token && !error && (
              <div className="p-3 rounded-lg bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-800">
                <p className="text-sm text-red-600 dark:text-red-400">
                  Ungültiger oder fehlender Bestätigungs-Link.
                </p>
              </div>
            )}

            <div className="space-y-4">
              <p className="text-sm text-neutral-600 dark:text-neutral-400 text-center">
                Klicke auf den Button, um deine E-Mail-Adresse zu bestätigen.
              </p>
              <button
                type="button"
                onClick={handleVerify}
                disabled={loading || !token}
                className="w-full cursor-pointer px-4 py-3 rounded-lg bg-neutral-900 dark:bg-neutral-100
                           text-white dark:text-neutral-900 font-medium text-sm
                           hover:bg-neutral-800 dark:hover:bg-neutral-200
                           transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {loading ? 'Wird bestätigt…' : 'Bestätigen'}
              </button>
            </div>

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
