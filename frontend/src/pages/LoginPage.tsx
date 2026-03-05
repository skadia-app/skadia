import { useNavigate } from 'react-router-dom'
import { isAuthenticated, isDevAuth, setToken } from '@/lib/auth'
import { useEffect } from 'react'

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

export default function LoginPage() {
  const navigate = useNavigate()

  useEffect(() => {
    if (isAuthenticated()) {
      navigate('/activities', { replace: true })
    }
  }, [navigate])

  function handleOIDCLogin() {
    window.location.href = `${API_BASE}/auth/login`
  }

  function handleDevLogin() {
    setToken('dev-token')
    navigate('/activities', { replace: true })
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-white dark:bg-neutral-950">
      <div className="w-full max-w-sm space-y-6 px-4">
        <div className="text-center space-y-2">
          <h1 className="text-3xl font-bold text-accent-DEFAULT tracking-tight">skadia</h1>
          <p className="text-sm text-neutral-500 dark:text-neutral-400">
            Melde dich an, um fortzufahren
          </p>
        </div>

        <div className="space-y-3">
          <button
            onClick={handleOIDCLogin}
            className="w-full px-4 py-3 rounded-lg bg-neutral-900 dark:bg-neutral-100
                       text-white dark:text-neutral-900 font-medium text-sm
                       hover:bg-neutral-800 dark:hover:bg-neutral-200 transition-colors"
          >
            Mit OIDC anmelden
          </button>

          {isDevAuth() && (
            <button
              onClick={handleDevLogin}
              className="w-full px-4 py-3 rounded-lg border border-neutral-200 dark:border-neutral-700
                         text-neutral-600 dark:text-neutral-300 text-sm
                         hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
            >
              Dev Login (Mock)
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
