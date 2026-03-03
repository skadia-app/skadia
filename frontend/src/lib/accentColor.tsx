import { createContext, useContext, useEffect, useState } from 'react'

export interface AccentEntry {
  label: string
  shade500: string
  shade600: string
  shade400: string
  shade100: string
}

export const ACCENT_PALETTE: AccentEntry[] = [
  { label: 'Orange',   shade500: '#F97316', shade600: '#EA580C', shade400: '#FB923C', shade100: '#FFEDD5' },
  { label: 'Bernstein', shade500: '#F59E0B', shade600: '#D97706', shade400: '#FCD34D', shade100: '#FEF3C7' },
  { label: 'Grün',     shade500: '#22C55E', shade600: '#16A34A', shade400: '#4ADE80', shade100: '#DCFCE7' },
  { label: 'Cyan',     shade500: '#06B6D4', shade600: '#0891B2', shade400: '#22D3EE', shade100: '#CFFAFE' },
  { label: 'Blau',     shade500: '#3B82F6', shade600: '#2563EB', shade400: '#60A5FA', shade100: '#DBEAFE' },
  { label: 'Indigo',   shade500: '#6366F1', shade600: '#4F46E5', shade400: '#818CF8', shade100: '#E0E7FF' },
  { label: 'Violett',  shade500: '#8B5CF6', shade600: '#7C3AED', shade400: '#A78BFA', shade100: '#EDE9FE' },
  { label: 'Rosa',     shade500: '#EC4899', shade600: '#DB2777', shade400: '#F472B6', shade100: '#FCE7F3' },
  { label: 'Rot',      shade500: '#EF4444', shade600: '#DC2626', shade400: '#F87171', shade100: '#FEE2E2' },
]

export const DEFAULT_ACCENT_COLOR = '#F97316'

const STORAGE_KEY = 'skadia-accent-color'

function hexToHsl(hex: string): [number, number, number] {
  const r = parseInt(hex.slice(1, 3), 16) / 255
  const g = parseInt(hex.slice(3, 5), 16) / 255
  const b = parseInt(hex.slice(5, 7), 16) / 255
  const max = Math.max(r, g, b), min = Math.min(r, g, b)
  let h = 0, s = 0
  const l = (max + min) / 2
  if (max !== min) {
    const d = max - min
    s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
    switch (max) {
      case r: h = ((g - b) / d + (g < b ? 6 : 0)) / 6; break
      case g: h = ((b - r) / d + 2) / 6; break
      case b: h = ((r - g) / d + 4) / 6; break
    }
  }
  return [h * 360, s * 100, l * 100]
}

function hslToHex(h: number, s: number, l: number): string {
  const s1 = s / 100
  const l1 = l / 100
  const a = s1 * Math.min(l1, 1 - l1)
  const f = (n: number) => {
    const k = (n + h / 30) % 12
    const color = l1 - a * Math.max(Math.min(k - 3, 9 - k, 1), -1)
    return Math.round(255 * color).toString(16).padStart(2, '0')
  }
  return `#${f(0)}${f(8)}${f(4)}`
}

export function deriveAccentEntry(hex500: string): AccentEntry {
  const [h, s, l] = hexToHsl(hex500)
  return {
    label: 'Benutzerdefiniert',
    shade500: hex500,
    shade600: hslToHex(h, Math.min(s + 5, 100), Math.max(l - 12, 5)),
    shade400: hslToHex(h, Math.max(s - 5, 0), Math.min(l + 10, 92)),
    shade100: hslToHex(h, Math.max(s - 30, 10), Math.min(l + 40, 96)),
  }
}

export function applyAccentColor(entry: AccentEntry) {
  const root = document.documentElement
  root.style.setProperty('--color-orange-500', entry.shade500)
  root.style.setProperty('--color-orange-600', entry.shade600)
  root.style.setProperty('--color-orange-400', entry.shade400)
  root.style.setProperty('--color-orange-100', entry.shade100)
}

function findEntry(hex: string): AccentEntry {
  return ACCENT_PALETTE.find(e => e.shade500.toLowerCase() === hex.toLowerCase()) ?? ACCENT_PALETTE[0]
}

interface AccentColorContextValue {
  accent: AccentEntry
  setAccent: (entry: AccentEntry) => void
}

const AccentColorContext = createContext<AccentColorContextValue>({
  accent: ACCENT_PALETTE[0],
  setAccent: () => {},
})

export function AccentColorProvider({ children }: { children: React.ReactNode }) {
  const [accent, setAccentState] = useState<AccentEntry>(() => {
    const stored = localStorage.getItem(STORAGE_KEY)
    return stored ? findEntry(stored) : ACCENT_PALETTE[0]
  })

  useEffect(() => {
    applyAccentColor(accent)
  }, [accent])

  function setAccent(entry: AccentEntry) {
    localStorage.setItem(STORAGE_KEY, entry.shade500)
    setAccentState(entry)
    applyAccentColor(entry)
  }

  return (
    <AccentColorContext.Provider value={{ accent, setAccent }}>
      {children}
    </AccentColorContext.Provider>
  )
}

export function useAccentColor() {
  return useContext(AccentColorContext)
}
