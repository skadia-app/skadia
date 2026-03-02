interface EnrichmentTagsProps {
  timeOfDay?: string | null
  intensity?: string | null
  isRoundTrip?: boolean
  weatherTags?: string[]
}

const timeOfDayStyles: Record<string, { label: string; classes: string }> = {
  'vor-sonnenaufgang': {
    label: 'Vor Sonnenaufgang',
    classes: 'bg-blue-900/20 text-blue-300 dark:bg-blue-900/30 dark:text-blue-300',
  },
  'goldene-stunde': {
    label: 'Goldene Stunde',
    classes: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  },
  'tagestour': {
    label: 'Tagestour',
    classes: 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300',
  },
  'nach-sonnenuntergang': {
    label: 'Nach Sonnenuntergang',
    classes: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
  },
  'nacht': {
    label: 'Nacht',
    classes: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300',
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

export default function EnrichmentTags({ timeOfDay, intensity, isRoundTrip, weatherTags }: EnrichmentTagsProps) {
  const badges: { key: string; label: string; classes: string }[] = []

  if (timeOfDay && timeOfDayStyles[timeOfDay]) {
    const s = timeOfDayStyles[timeOfDay]
    badges.push({ key: `tod-${timeOfDay}`, label: s.label, classes: s.classes })
  }

  if (intensity && intensityStyles[intensity]) {
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
