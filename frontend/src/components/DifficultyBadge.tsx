import { SAC_COLOR_MAP, MTB_COLOR_MAP } from './segment-editor/constants'

interface DifficultyBadgeProps {
  sac?: string | null
  mtb?: string | null
  color?: string | null
}

const COLOR_MAP = {
  blue:  { bg: 'bg-blue-500/20',      text: 'text-blue-400',    label: 'Blau' },
  red:   { bg: 'bg-red-500/20',       text: 'text-red-400',     label: 'Rot' },
  black: { bg: 'bg-neutral-600/40',   text: 'text-neutral-300', label: 'Schwarz' },
}

const SCALE_COLOR_MAP: Record<string, { bg: string, text: string }> = {
  blue:  { bg: 'bg-blue-500/20',    text: 'text-blue-400' },
  red:   { bg: 'bg-red-500/20',     text: 'text-red-400' },
  black: { bg: 'bg-neutral-700/50', text: 'text-neutral-300' },
}

export function DifficultyBadge({ sac, mtb, color }: DifficultyBadgeProps) {
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
