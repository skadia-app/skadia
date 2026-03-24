import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { setToken } from '@/lib/auth'
import { exchangeAuthCode } from '@/api/auth'
import { getAuthErrorMessage } from '@/pages/auth/errorMessages'

export default function CallbackPage() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const code = searchParams.get('code')
    const errorParam = searchParams.get('error')

    if (errorParam) {
      navigate(`/login?error=${errorParam}`, { replace: true })
      return
    }

    if (!code) {
      navigate('/login', { replace: true })
      return
    }

    exchangeAuthCode(code)
      .then(data => {
        setToken(data.token)
        navigate('/activities', { replace: true })
      })
      .catch(err => {
        const msg = err instanceof Error ? err.message : 'exchange_failed'
        setError(getAuthErrorMessage(msg))
      })
  }, [searchParams, navigate])

  if (error) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-white dark:bg-neutral-950">
        <div className="text-center space-y-2">
          <p className="text-sm text-red-500">{error}</p>
          <button
            onClick={() => navigate('/login', { replace: true })}
            className="text-sm text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300"
          >
            Zurück zur Anmeldung
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-white dark:bg-neutral-950">
      <span className="text-sm text-neutral-400">Anmeldung wird verarbeitet...</span>
    </div>
  )
}
