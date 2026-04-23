import React from 'react'
import { SAC_COLOR_MAP, MTB_COLOR_MAP } from './segment-editor/constants'

interface DifficultyBadgeProps {
  sac?: string | null
  mtb?: string | null
  color?: string | null
}

const COLOR_MAP = {
  blue:  { bg: 'bg-blue-500/20',      text: 'text-blue-400',    label: 'Blue' },
  red:   { bg: 'bg-red-500/20',       text: 'text-red-400',     label: 'Red' },
  black: { bg: 'bg-neutral-600/40',   text: 'text-neutral-300', label: 'Black' },
}

export function DifficultyBadge({ sac, mtb, color }: DifficultyBadgeProps) {
  const style = color ? COLOR_MAP[color as keyof typeof COLOR_MAP] : null

  return (
    <span className="inline-flex items-center gap-1">
      {style && (
        <span
          className={`inline-block w-2.5 h-2.5 rounded-full ${style.bg} border border-current ${style.text}`}
          title={style.label}
        />
      )}
      {sac && <span className="text-xs font-mono text-neutral-400">{sac}</span>}
      {mtb && <span className="text-xs font-mono text-neutral-400">{mtb}</span>}
    </span>
  )
}
