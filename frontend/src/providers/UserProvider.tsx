import { createContext, useContext, useEffect } from 'react'
import { Outlet } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getMe, updateMe, type MeData } from '@/api/auth'
import { i18n } from '@/i18n/i18n'
import type { SupportedLanguage } from '@/i18n/languages'
import type { UnitSystem } from '@/lib/units'

interface UserContextType {
  user: MeData | undefined
  locale: SupportedLanguage
  units: UnitSystem
  setLocale: (locale: SupportedLanguage) => Promise<void>
  setUnits: (units: UnitSystem) => Promise<void>
  isAdmin: boolean
  isVerified: boolean
  hasPassword: boolean
  mustChangePassword: boolean
}

const UserContext = createContext<UserContextType | null>(null)

export function UserProvider({ children }: { children: React.ReactNode }) {
  const queryClient = useQueryClient()
  const { data: user, isLoading } = useQuery({
    queryKey: ['me'],
    queryFn: getMe,
    staleTime: 5 * 60 * 1000,
    retry: false,
  })

  const preferenceMutation = useMutation({
    mutationFn: updateMe,
    onSuccess: (updatedUser) => {
      queryClient.setQueryData(['me'], updatedUser)
      void i18n.changeLanguage(updatedUser.locale)
    },
  })

  useEffect(() => {
    if (user?.locale) void i18n.changeLanguage(user.locale)
  }, [user?.locale])

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="animate-spin h-8 w-8 border-2 border-accent border-t-transparent rounded-full" />
      </div>
    )
  }

  const locale = user?.locale ?? 'de'
  const units = user?.units ?? 'metric'

  return (
    <UserContext.Provider value={{
      user,
      locale,
      units,
      setLocale: async (nextLocale) => {
        await preferenceMutation.mutateAsync({ locale: nextLocale })
      },
      setUnits: async (nextUnits) => {
        await preferenceMutation.mutateAsync({ units: nextUnits })
      },
      isAdmin: user?.is_admin ?? false,
      isVerified: user?.email_verified ?? false,
      hasPassword: user?.has_password ?? false,
      mustChangePassword: user?.must_change_password ?? false,
    }}>
      {children}
    </UserContext.Provider>
  )
}

/** Layout-route wrapper: provides UserContext to all child routes via Outlet. */
export function UserProviderLayout() {
  return (
    <UserProvider>
      <Outlet />
    </UserProvider>
  )
}

export function useUser() {
  const ctx = useContext(UserContext)
  if (!ctx) throw new Error('useUser must be used within UserProvider')
  return ctx
}

/**
 * Returns the current user's id, or `'anon'` when no UserProvider is mounted
 * (tests, public pages). Stable accessor for React-Query keys that need to
 * be user-scoped as defense-in-depth against cross-user cache leakage.
 *
 * The value is included in query-key tuples (e.g. `['user-map-layers', userId]`)
 * so cache reads from a stale user-id never collide with the new user's data,
 * even if `queryClient.clear()` somehow doesn't run on logout.
 */
export function useCurrentUserId(): string {
  const ctx = useContext(UserContext)
  return ctx?.user?.user_id ?? 'anon'
}
