// Globale Sport-Einstellungen: Farben + "nicht vorhandene ausblenden"
// localStorage keys:
// - 'skadia-sport-colors-global': { [sportType: string]: string } — Hex-Farben pro Sport
// - 'skadia-hide-unused-sports': boolean — Standard: true

import { createContext, useContext, useState, useCallback, type ReactNode } from 'react'
import { sportColor } from './sportTaxonomy'

// ── Types ─────────────────────────────────────────────────────────────────────

interface GlobalSportSettings {
  /** Gibt die effektive Farbe fuer einen Sport-Type zurueck (Custom > Default) */
  getSportColor: (sportType: string) => string
  /** Setzt die Farbe fuer einen Sport-Type */
  setSportColor: (sportType: string, color: string) => void
  /** Setzt Farbe auf Standard zurueck */
  resetSportColor: (sportType: string) => void
  /** Alle gesetzten Custom-Farben */
  customColors: Record<string, string>
  /** Alle Custom-Farben zuruecksetzen */
  resetAllColors: () => void
  /** "Nicht vorhandene ausblenden" Setting */
  hideUnused: boolean
  setHideUnused: (value: boolean) => void
}

// ── Storage keys ─────────────────────────────────────────────────────────────

const COLORS_KEY = 'skadia-sport-colors-global'
const HIDE_UNUSED_KEY = 'skadia-hide-unused-sports'

// ── Helpers ───────────────────────────────────────────────────────────────────

function loadColors(): Record<string, string> {
  try {
    const raw = localStorage.getItem(COLORS_KEY)
    if (raw) return JSON.parse(raw)
  } catch {
    // ignore
  }
  return {}
}

function saveColors(colors: Record<string, string>) {
  localStorage.setItem(COLORS_KEY, JSON.stringify(colors))
}

function loadHideUnused(): boolean {
  try {
    const raw = localStorage.getItem(HIDE_UNUSED_KEY)
    if (raw !== null) return raw === 'true'
  } catch {
    // ignore
  }
  return true // default: true
}

// ── Context ───────────────────────────────────────────────────────────────────

const GlobalSportSettingsContext = createContext<GlobalSportSettings | null>(null)

export function GlobalSportSettingsProvider({ children }: { children: ReactNode }) {
  const [customColors, setCustomColors] = useState<Record<string, string>>(loadColors)
  const [hideUnused, setHideUnusedState] = useState<boolean>(loadHideUnused)

  const getSportColor = useCallback(
    (sportType: string): string => {
      return customColors[sportType] ?? sportColor(sportType)
    },
    [customColors],
  )

  const setSportColor = useCallback((sportType: string, color: string) => {
    setCustomColors((prev) => {
      const next = { ...prev, [sportType]: color }
      saveColors(next)
      return next
    })
  }, [])

  const resetSportColor = useCallback((sportType: string) => {
    setCustomColors((prev) => {
      const next = { ...prev }
      delete next[sportType]
      saveColors(next)
      return next
    })
  }, [])

  const resetAllColors = useCallback(() => {
    setCustomColors({})
    saveColors({})
  }, [])

  const setHideUnused = useCallback((value: boolean) => {
    setHideUnusedState(value)
    localStorage.setItem(HIDE_UNUSED_KEY, String(value))
  }, [])

  return (
    <GlobalSportSettingsContext.Provider
      value={{ getSportColor, setSportColor, resetSportColor, customColors, resetAllColors, hideUnused, setHideUnused }}
    >
      {children}
    </GlobalSportSettingsContext.Provider>
  )
}

// ── Hook ──────────────────────────────────────────────────────────────────────

export function useGlobalSportSettings(): GlobalSportSettings {
  const ctx = useContext(GlobalSportSettingsContext)
  if (!ctx) throw new Error('useGlobalSportSettings must be used within GlobalSportSettingsProvider')
  return ctx
}
