interface Props {
  label: string
  value: string
  title?: string
}

export function ActivityStatCard({ label, value, title }: Props) {
  return (
    <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
      <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1" title={title}>{label}</div>
      <div className="text-base font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">{value}</div>
    </div>
  )
}
