import { SAC_COLOR_MAP, MTB_COLOR_MAP } from './segment-editor/constants'

interface DifficultyBadgeProps {
  sac?: string | null
  mtb?: string | null
  color?: string | null
}

// DAV-Skipisten-Konvention: blau (leicht) — rot (mittel) — schwarz (schwer).
// „Schwarz" muss im Light-Mode tatsächlich dunkel sein, nicht transparentes Grau,
// sonst wirkt ein T4-Badge blasser als T2. Im Dark-Mode invertieren wir den
// Schwarz-Ton (heller Hintergrund auf dunkler Karte), damit er nicht mit der
// Card-Fläche verschmilzt. Blau/Rot nutzen Light: satte Pastell-Fülle,
// Dark: transparenter Akzent wie vorher.
const COLOR_MAP = {
  blue:  { bg: 'bg-blue-500',                          text: 'text-white',                           label: 'Blau' },
  red:   { bg: 'bg-red-600',                           text: 'text-white',                           label: 'Rot' },
  black: { bg: 'bg-neutral-900 dark:bg-neutral-100',   text: 'text-white dark:text-neutral-900',     label: 'Schwarz' },
}

const SCALE_COLOR_MAP: Record<string, { bg: string, text: string }> = {
  blue:  { bg: 'bg-blue-100 dark:bg-blue-500/20',       text: 'text-blue-800 dark:text-blue-300' },
  red:   { bg: 'bg-red-100 dark:bg-red-500/20',         text: 'text-red-800 dark:text-red-300' },
  black: { bg: 'bg-neutral-900 dark:bg-neutral-200',    text: 'text-white dark:text-neutral-900' },
}

export function DifficultyBadge({ sac, mtb, color }: DifficultyBadgeProps) {
  if (!color && !sac && !mtb) return null

  const style = color ? COLOR_MAP[color as keyof typeof COLOR_MAP] : null

  return (
    <span className="inline-flex items-center gap-1">
      {style && (
        <span
          className={`inline-block w-2 h-2 rounded-full ${style.bg} border border-current ${style.text}`}
          title={style.label}
        />
      )}
      
      {sac && (
        <span className={`px-1.5 py-0.5 rounded text-[10px] font-bold uppercase tracking-tight border ${
          SAC_COLOR_MAP[sac] 
            ? `${SCALE_COLOR_MAP[SAC_COLOR_MAP[sac]].bg} ${SCALE_COLOR_MAP[SAC_COLOR_MAP[sac]].text} border-current/20`
            : 'bg-neutral-800 text-neutral-400 border-neutral-700'
        }`}>
          {sac}
        </span>
      )}

      {mtb && (
        <span className={`px-1.5 py-0.5 rounded text-[10px] font-bold uppercase tracking-tight border ${
          MTB_COLOR_MAP[mtb]
            ? `${SCALE_COLOR_MAP[MTB_COLOR_MAP[mtb]].bg} ${SCALE_COLOR_MAP[MTB_COLOR_MAP[mtb]].text} border-current/20`
            : 'bg-neutral-800 text-neutral-400 border-neutral-700'
        }`}>
          {mtb}
        </span>
      )}
    </span>
  )
}

export default DifficultyBadge
