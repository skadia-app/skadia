import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useUser } from '@/providers/UserProvider'
import { resendVerification } from '@/api/auth'

export function EmailVerifiedBanner() {
  const { t } = useTranslation()
  const { isVerified, user } = useUser()
  const [sent, setSent] = useState(false)
  const [sending, setSending] = useState(false)

  if (isVerified || !user) return null

  const handleResend = async () => {
    setSending(true)
    try {
      await resendVerification()
      setSent(true)
    } catch {
      // fail silently — SMTP may not be configured
    } finally {
      setSending(false)
    }
  }

  return (
    <div className="bg-amber-500/10 border-b border-amber-500/20 px-4 py-2 text-sm text-amber-700 dark:text-amber-400 flex items-center justify-between">
      <span>{t('auth.emailBanner.unverified')}</span>
      {sent ? (
        <span className="text-green-600 dark:text-green-400">{t('auth.emailBanner.sent')}</span>
      ) : (
        <button
          onClick={handleResend}
          disabled={sending}
          className="cursor-pointer underline hover:no-underline font-medium disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {sending ? t('auth.common.sending') : t('auth.emailBanner.resend')}
        </button>
      )}
    </div>
  )
}
