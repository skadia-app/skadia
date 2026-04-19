// frontend/src/components/activity/MapColorLegend.tsx
import type { ScaleDefinition } from '@/lib/mapColorUtils'

interface Props {
  scale: ScaleDefinition | null
}

export default function MapColorLegend({ scale }: Props) {
  // Hide but keep slot to prevent layout shifts
  const hidden = !scale || scale.type === 'constant'

  return (
    <div
      className={`absolute top-3 right-14 z-10 rounded-lg bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm shadow-md p-2 text-xs text-neutral-700 dark:text-neutral-300 max-w-[220px] ${
        hidden ? 'invisible' : ''
      }`}
      aria-hidden={hidden}
    >
      {scale && scale.type === 'linear' && <LinearLegend scale={scale} />}
      {scale && scale.type === 'zones' && <ZonesLegend scale={scale} />}
    </div>
  )
}

function LinearLegend({ scale }: { scale: Extract<ScaleDefinition, { type: 'linear' }> }) {
  const gradient = scale.stops
    .map(s => s.color)
    .join(', ')
  const min = scale.stops[0].value
  const max = scale.stops[scale.stops.length - 1].value
  const mid = (min + max) / 2

  const fmt = (v: number) => {
    if (scale.metric === 'pace') {
      const mins = Math.floor(v)
      const sec = Math.round((v - mins) * 60)
      return `${mins}:${String(sec).padStart(2, '0')}`
    }
    return String(Math.round(v))
  }

  return (
    <div>
      <div className="font-medium mb-1">{metricName(scale.metric)}</div>
      <div
        className="h-2 rounded-full mb-1"
        style={{ background: `linear-gradient(to right, ${gradient})` }}
      />
      <div className="flex justify-between text-[10px] tabular-nums">
        <span>{fmt(min)}</span>
        <span>{fmt(mid)}</span>
        <span>{fmt(max)}</span>
      </div>
      <div className="text-[10px] text-neutral-500 mt-0.5">{scale.unit}</div>
    </div>
  )
}

function ZonesLegend({ scale }: { scale: Extract<ScaleDefinition, { type: 'zones' }> }) {
  return (
    <div>
      <div className="font-medium mb-1">{metricName(scale.metric)}</div>
      <ul className="space-y-0.5 max-h-40 overflow-y-auto">
        {scale.zoneColors.map((color, i) => (
          <li key={i} className="flex items-center gap-1.5 text-[10px]">
            <span className="inline-block w-3 h-3 rounded-sm" style={{ background: color }} />
            <span className="tabular-nums">
              Z{i + 1}:{' '}
              {i === scale.zoneColors.length - 1 || scale.zoneHighs[i] >= 9999
                ? <>&gt; {i > 0 ? scale.zoneHighs[i - 1] : 0}{' '}{scale.unit}</>
                : <>≤ {scale.zoneHighs[i]}{' '}{scale.unit}</>}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}

function metricName(m: ScaleDefinition['metric']): string {
  const mapping: Record<typeof m, string> = {
    sport: 'Sportart',
    hr: 'Herzfrequenz',
    speed: 'Geschwindigkeit',
    pace: 'Tempo',
    altitude: 'Höhe',
    cadence: 'Frequenz',
    power: 'Leistung',
    temperature: 'Temperatur',
  }
  return mapping[m]
}
