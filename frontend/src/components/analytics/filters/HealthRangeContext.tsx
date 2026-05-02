import { createContext, useContext, useState, type ReactNode } from 'react'
import { toLocalISO } from '@/lib/dateUtils'

interface HealthRange {
  from: string
  to: string
}

interface HealthRangeContextValue {
  from: string
  to: string
  setRange: (r: HealthRange) => void
}

const HealthRangeContext = createContext<HealthRangeContextValue | null>(null)

function defaultRange(): HealthRange {
  const to = new Date()
  const from = new Date()
  from.setDate(from.getDate() - 30)
  return { from: toLocalISO(from), to: toLocalISO(to) }
}

export function HealthRangeProvider({ children }: { children: ReactNode }) {
  const [range, setRange] = useState<HealthRange>(defaultRange)

  return (
    <HealthRangeContext.Provider value={{ from: range.from, to: range.to, setRange }}>
      {children}
    </HealthRangeContext.Provider>
  )
}

export function useHealthRange(): HealthRangeContextValue {
  const ctx = useContext(HealthRangeContext)
  if (!ctx) throw new Error('useHealthRange must be used within HealthRangeProvider')
  return ctx
}
