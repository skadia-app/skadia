interface Props {
  meta: Record<string, unknown>
}

const COLOR_MAP = {
  blue:  { bg: 'bg-blue-500/20',      text: 'text-blue-400',    label: 'Blau' },
  red:   { bg: 'bg-red-500/20',       text: 'text-red-400',     label: 'Rot' },
  black: { bg: 'bg-neutral-600/40',   text: 'text-neutral-300', label: 'Schwarz' },
}

export default function DifficultyBadge({ meta }: Props) {
  const color = meta.difficulty_color as string | undefined
  const sac = meta.sac_scale as string | undefined
  const mtb = meta.mtb_scale as string | undefined

  if (!color && !sac && !mtb) return null

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
