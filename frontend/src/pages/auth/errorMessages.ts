/** Auth error code -> user-facing message mapping.
 * Keys are error codes from the backend (snake_case, English).
 * Values are user-facing messages (German, will be replaced by i18n later).
 *
 * Context annotations help future translators understand the usage.
 */
export const AUTH_ERROR_MESSAGES: Record<string, string> = {
  // Login errors
  invalid_credentials: 'E-Mail oder Passwort ung\u00FCltig.', // context: login form, wrong email/password
  account_delayed: 'Zu viele Fehlversuche. Bitte versuche es sp\u00E4ter erneut.', // context: progressive delay after failed logins
  email_not_verified: 'Bitte verifiziere zuerst deine E-Mail-Adresse.', // context: login blocked until email verified
  account_inactive: 'Dein Konto ist noch nicht freigeschaltet.', // context: approval-mode account pending admin activation

  // Registration errors
  password_too_short: 'Passwort muss mindestens 10 Zeichen lang sein.', // context: registration/reset password form
  password_too_long: 'Passwort ist zu lang (max. 1000 Zeichen).', // context: registration/reset password form
  password_compromised:
    'Dieses Passwort wurde in einem Datenleck gefunden. Bitte w\u00E4hle ein anderes.', // context: HIBP check failed
  invalid_email: 'Bitte gib eine g\u00FCltige E-Mail-Adresse ein.', // context: email format validation
  username_invalid_length: 'Benutzername muss zwischen 2 und 100 Zeichen lang sein.', // context: registration
  username_invalid_chars: 'Benutzername enth\u00E4lt ung\u00FCltige Zeichen.', // context: registration
  email_and_password_required: 'E-Mail und Passwort sind erforderlich.', // context: login form empty fields

  // Social login errors
  email_exists:
    'Diese E-Mail ist bereits registriert. Bitte melde dich an und verkn\u00FCpfe dein Konto in den Einstellungen.', // context: social login email collision
  provider_error: 'Fehler beim Social Login. Bitte versuche es erneut.', // context: OAuth provider returned error
  exchange_failed: 'Anmeldung fehlgeschlagen. Bitte versuche es erneut.', // context: auth code exchange failed
  invalid_state: 'Ung\u00FCltige Sitzung. Bitte versuche es erneut.', // context: OAuth state mismatch

  // Password reset
  invalid_or_expired_token: 'Ung\u00FCltiger oder abgelaufener Link.', // context: email verification or password reset token

  // Change password
  wrong_password: 'Aktuelles Passwort ist falsch.', // context: change-password form
  no_password_set: 'Kein Passwort gesetzt. Verwende Social Login.', // context: OAuth-only user trying to change password
  current_and_new_password_required: 'Aktuelles und neues Passwort sind erforderlich.', // context: change-password form

  // Session
  session_expired: 'Sitzung abgelaufen. Bitte erneut anmelden.', // context: refresh token invalid/expired

  // Generic
  too_many_requests: 'Zu viele Anfragen. Bitte warte einen Moment.', // context: rate limit hit
  internal_error: 'Ein interner Fehler ist aufgetreten.', // context: server error
  invalid_request: 'Ung\u00FCltige Anfrage.', // context: malformed request body
  unauthorized: 'Nicht autorisiert.', // context: missing/invalid JWT
}

/** Get a user-facing error message for an auth error code.
 * Falls back to the raw code if no mapping exists.
 */
export function getAuthErrorMessage(code: string): string {
  return AUTH_ERROR_MESSAGES[code] ?? code
}
