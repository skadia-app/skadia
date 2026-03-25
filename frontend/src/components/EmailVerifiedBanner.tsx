import { useState } from 'react'
import { useUser } from '@/providers/UserProvider'
import { resendVerification } from '@/api/auth'

const STRINGS = {
  unverified: 'Deine E-Mail-Adresse ist noch nicht bestätigt. Bitte prüfe dein Postfach.',
  resend: 'Erneut senden',
  sending: 'Wird gesendet...',
  sent: 'Bestätigungs-E-Mail gesendet!',
}

export function EmailVerifiedBanner() {
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
      <span>{STRINGS.unverified}</span>
      {sent ? (
        <span className="text-green-600 dark:text-green-400">{STRINGS.sent}</span>
      ) : (
        <button
          onClick={handleResend}
          disabled={sending}
          className="underline hover:no-underline font-medium disabled:opacity-50"
        >
          {sending ? STRINGS.sending : STRINGS.resend}
        </button>
      )}
    </div>
  )
}
