interface EnrichmentTagsProps {
  timeOfDay?: string[] | null
  intensity?: string | null
  isRoundTrip?: boolean
  weatherTags?: string[]
}

const timeOfDayStyles: Record<string, { label: string; classes: string }> = {
  'nacht': {
    label: 'Nacht',
    classes: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300',
  },
  'astronomische-daemmerung': {
    label: 'Astro. Dämmerung',
    classes: 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300',
  },
  'nautische-daemmerung': {
    label: 'Naut. Dämmerung',
    classes: 'bg-blue-900/20 text-blue-300 dark:bg-blue-900/30 dark:text-blue-300',
  },
  'buergerliche-daemmerung': {
    label: 'Bürgerl. Dämmerung',
    classes: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
  },
  'goldene-stunde': {
    label: 'Goldene Stunde',
    classes: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  },
  'tag': {
    label: 'Tag',
    classes: 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300',
  },
}

const intensityStyles: Record<string, { label: string; classes: string }> = {
  'locker': {
    label: 'Locker',
    classes: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
  },
  'moderat': {
    label: 'Moderat',
    classes: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  },
  'intensiv': {
    label: 'Intensiv',
    classes: 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300',
  },
}

const hiddenTimeOfDay = new Set(['tag', 'goldene-stunde', 'buergerliche-daemmerung'])

export default function EnrichmentTags({ timeOfDay, intensity, isRoundTrip, weatherTags }: EnrichmentTagsProps) {
  const badges: { key: string; label: string; classes: string }[] = []

  if (timeOfDay) {
    for (const tod of timeOfDay) {
      if (hiddenTimeOfDay.has(tod)) continue
      const s = timeOfDayStyles[tod]
      if (s) {
        badges.push({ key: `tod-${tod}`, label: s.label, classes: s.classes })
      }
    }
  }

  if (intensity && intensity !== 'intensiv' && intensityStyles[intensity]) {
    const s = intensityStyles[intensity]
    badges.push({ key: `int-${intensity}`, label: s.label, classes: s.classes })
  }

  if (isRoundTrip) {
    badges.push({
      key: 'round-trip',
      label: 'Rundtour',
      classes: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
    })
  }

  if (weatherTags) {
    for (const tag of weatherTags) {
      badges.push({
        key: `weather-${tag}`,
        label: tag,
        classes: 'bg-cyan-100 text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-300',
      })
    }
  }

  if (badges.length === 0) return null

  return (
    <div className="flex flex-wrap gap-2">
      {badges.map(({ key, label, classes }) => (
        <span
          key={key}
          className={`px-2.5 py-1 rounded-full text-xs font-medium ${classes}`}
        >
          {label}
        </span>
      ))}
    </div>
  )
}
