// Application-wide singleton QueryClient.
//
// Lives in its own module so non-React code (e.g. lib/auth.ts) can call
// `clearQueryClientCache()` on logout without going through React context.
// This prevents cross-user data leakage when User A logs out and User B
// signs in within the same tab without a hard reload — the cache holds
// User A's user-scoped query results otherwise.

import { QueryClient } from '@tanstack/react-query'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 60 * 5,
      gcTime: 1000 * 60 * 60 * 24,
      retry: 1,
    },
  },
})

/**
 * Wipes all cached queries and mutations.
 *
 * Multi-user safety: invoked from clearToken() in lib/auth.ts so every
 * logout path (UserMenu, account-deletion, password-change, refresh
 * failure, cross-tab BroadcastChannel logout, 401 in apiFetch/apiClient)
 * discards user-scoped cache entries before the next user can perceive
 * them. Synchronous — safe to call before a navigate() or location.href
 * assignment.
 */
export function clearQueryClientCache(): void {
  queryClient.clear()
}
