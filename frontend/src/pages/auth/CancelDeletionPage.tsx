import { useEffect, useState } from 'react'
import { useSearchParams, Link } from 'react-router-dom'

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

export default function CancelDeletionPage() {
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token')
  const [status, setStatus] = useState<'loading' | 'success' | 'error'>('loading')
  const [errorMsg, setErrorMsg] = useState('')

  useEffect(() => {
    if (!token) { setStatus('error'); setErrorMsg('Kein Token angegeben.'); return }
    fetch(`${API_BASE}/auth/cancel-deletion`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token }),
    }).then(res => res.json()).then(data => {
      if (data.status === 'deletion_cancelled') setStatus('success')
      else { setStatus('error'); setErrorMsg(data.error || 'Unbekannter Fehler') }
    }).catch(() => { setStatus('error'); setErrorMsg('Verbindungsfehler') })
  }, [token])

  return (
    <div className="min-h-screen flex items-center justify-center bg-white dark:bg-neutral-950">
      <div className="w-full max-w-sm space-y-6 px-4 text-center">
        <div className="space-y-2">
          <h1 className="text-3xl font-bold text-accent-DEFAULT tracking-tight">skadia</h1>
          <p className="text-sm text-neutral-500 dark:text-neutral-400">Account-Löschung abbrechen</p>
        </div>

        {status === 'loading' && (
          <p className="text-sm text-neutral-500 dark:text-neutral-400">Löschung wird abgebrochen...</p>
        )}
        {status === 'success' && (
          <div className="space-y-4">
            <div className="p-4 rounded-lg bg-green-50 dark:bg-green-950/30 border border-green-200 dark:border-green-800">
              <p className="text-sm text-green-700 dark:text-green-400 font-medium">
                Account-Löschung abgebrochen!
              </p>
              <p className="text-sm text-green-600 dark:text-green-500 mt-1">
                Dein Account bleibt bestehen. Du kannst dich weiterhin anmelden.
              </p>
            </div>
            <Link to="/login" className="text-sm text-accent-DEFAULT hover:underline">
              Zur Anmeldung
            </Link>
          </div>
        )}
        {status === 'error' && (
          <div className="space-y-4">
            <div className="p-3 rounded-lg bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-800">
              <p className="text-sm text-red-600 dark:text-red-400">{errorMsg}</p>
            </div>
            <Link to="/login" className="text-sm text-accent-DEFAULT hover:underline">
              Zur Anmeldung
            </Link>
          </div>
        )}
      </div>
    </div>
  )
}
