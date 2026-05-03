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
    // shade600: darker hover shade — capped to max 65% lightness so it stays visible on white backgrounds
    shade600: hslToHex(h, Math.min(s + 5, 100), Math.min(Math.max(l - 12, 5), 65)),
    // shade400: lighter shade — clamped to 50–70% lightness so it's visible on dark backgrounds
    // without becoming so light that white toggle thumbs/button text disappear on top of it
    shade400: hslToHex(h, Math.max(s - 5, 0), Math.max(Math.min(l + 10, 70), 50)),
    // shade100: light tint for icon containers — capped to max 88% so it stays visible on white page backgrounds
    shade100: hslToHex(h, Math.max(s - 20, 8), Math.min(Math.max(l + 40, 82), 88)),
  }
}

export function applyAccentColor(entry: AccentEntry) {
  const root = document.documentElement
  root.style.setProperty('--color-orange-500', entry.shade500)
  root.style.setProperty('--color-orange-600', entry.shade600)
  root.style.setProperty('--color-orange-400', entry.shade400)
  root.style.setProperty('--color-orange-100', entry.shade100)
  // Also expose as --color-accent-* for components that reference accent directly
  root.style.setProperty('--color-accent-DEFAULT', entry.shade500)
  root.style.setProperty('--color-accent-600', entry.shade600)
  root.style.setProperty('--color-accent-400', entry.shade400)
  root.style.setProperty('--color-accent-100', entry.shade100)

  // Inject visibility safeguards that override the inline styles above via !important.
  // - Light mode: if shade500 is too light (>60% HSL lightness), use shade600 as the default
  //   interactive color so buttons/toggles don't disappear on white backgrounds.
  // - Dark mode: always use shade400 (≥55% lightness) so elements don't disappear on dark backgrounds.
  const [, , l500] = hexToHsl(entry.shade500)
  const isLightAccent = l500 > 60
  const lightDefault = isLightAccent ? entry.shade600 : entry.shade500
  const lightHover = isLightAccent
    ? (() => { const [hh, hs, hl] = hexToHsl(entry.shade600); return hslToHex(hh, hs, Math.max(hl - 15, 5)) })()
    : entry.shade600

  // Compute foreground colors: use dark text/thumb when background is too light for white
  // Light-mode threshold 55%: covers shade600 ≈ l=65% for near-white accents
  // Dark-mode threshold 65%: shade400 is capped at 70%, palette orange is 65% (exactly not > 65)
  const [, , lLightDefault] = hexToHsl(lightDefault)
  const [, , lDarkDefault] = hexToHsl(entry.shade400)
  const lightFg = lLightDefault > 55 ? '#171717' : '#ffffff'
  const darkFg  = lDarkDefault  > 65 ? '#171717' : '#ffffff'

  let styleTag = document.getElementById('skadia-accent-override') as HTMLStyleElement | null
  if (!styleTag) {
    styleTag = document.createElement('style')
    styleTag.id = 'skadia-accent-override'
    document.head.appendChild(styleTag)
  }
  styleTag.textContent = [
    // Light-mode overrides (noop for normal colors where lightDefault = shade500)
    ':root {',
    `  --color-accent-DEFAULT: ${lightDefault} !important;`,
    `  --color-accent-600: ${lightHover} !important;`,
    `  --color-orange-500: ${lightDefault} !important;`,
    `  --color-orange-600: ${lightHover} !important;`,
    '}',
    // Light-mode foreground: button text + toggle thumb
    `[class~="bg-accent-DEFAULT"], [class~="bg-orange-500"] { color: ${lightFg} !important; }`,
    `[role="switch"][aria-checked="true"] span { background-color: ${lightFg} !important; }`,
    // Dark-mode color overrides
    '.dark {',
    `  --color-accent-DEFAULT: ${entry.shade400} !important;`,
    `  --color-accent-600: ${entry.shade500} !important;`,
    `  --color-orange-500: ${entry.shade400} !important;`,
    `  --color-orange-600: ${entry.shade500} !important;`,
    '}',
    // Dark-mode foreground: button text + toggle thumb
    `.dark [class~="bg-accent-DEFAULT"], .dark [class~="bg-orange-500"] { color: ${darkFg} !important; }`,
    `.dark [role="switch"][aria-checked="true"] span { background-color: ${darkFg} !important; }`,
  ].join('\n')
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
    return stored
      ? (ACCENT_PALETTE.find(e => e.shade500.toLowerCase() === stored.toLowerCase()) ?? deriveAccentEntry(stored))
      : ACCENT_PALETTE[0]
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
