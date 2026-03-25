import { createContext, useContext } from 'react'
import { Outlet } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { getMe, type MeData } from '@/api/auth'

interface UserContextType {
  user: MeData | undefined
  isAdmin: boolean
  isVerified: boolean
  hasPassword: boolean
  mustChangePassword: boolean
}

const UserContext = createContext<UserContextType | null>(null)

export function UserProvider({ children }: { children: React.ReactNode }) {
  const { data: user, isLoading } = useQuery({
    queryKey: ['me'],
    queryFn: getMe,
    staleTime: 5 * 60 * 1000,
    retry: false,
  })

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="animate-spin h-8 w-8 border-2 border-accent border-t-transparent rounded-full" />
      </div>
    )
  }

  return (
    <UserContext.Provider value={{
      user,
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
