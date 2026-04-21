// frontend/src/components/route-planner/ScaleLegend.tsx
//
// Legende für den Map-Overlay der Route-Difficulty-Scales (SAC/MTB/VF).
// Erscheint nur wenn ein Scale-Pin aktiv ist und zeigt die 6 Rainbow-Farben
// passend zu den Map-Layer-Farben aus scaleColors.ts (SAC_COLORS/MTB_COLORS/
// VF_COLORS). Badge-Dots nutzen dagegen DAV-Konvention (3 Farben) — die
// Legende erklärt daher primär das Map-Overlay, nicht die Badge-Dots.
import { SAC_COLORS, MTB_COLORS, VF_COLORS, VF_UNKNOWN_COLOR } from '@/lib/scaleColors'

type ScaleKind = 'sac' | 'mtb' | 'vf'

interface Props {
  kind: ScaleKind | null
  hasUnknownVf?: boolean   // true wenn vf_unknown_ranges auf der Route existieren
}

const SAC_LEVELS = ['T1', 'T2', 'T3', 'T4', 'T5', 'T6'] as const
const MTB_LEVELS = ['S0', 'S1', 'S2', 'S3', 'S4', 'S5'] as const
const VF_LEVELS  = ['K1', 'K2', 'K3', 'K4', 'K5', 'K6'] as const

const TITLES: Record<ScaleKind, string> = {
  sac: 'SAC-Wanderskala',
  mtb: 'MTB-Schwierigkeit',
  vf:  'Klettersteig (Via Ferrata)',
}

export default function ScaleLegend({ kind, hasUnknownVf }: Props) {
  if (!kind) return null

  const entries =
    kind === 'sac' ? SAC_LEVELS.map(l => ({ level: l, color: SAC_COLORS[l] })) :
    kind === 'mtb' ? MTB_LEVELS.map(l => ({ level: l, color: MTB_COLORS[l] })) :
                     VF_LEVELS.map(l => ({ level: l, color: VF_COLORS[l] }))

  return (
    <div
      className="absolute top-20 right-4 z-10 rounded-lg bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm shadow-md p-2 text-xs text-neutral-700 dark:text-neutral-300 min-w-[140px]"
      role="region"
      aria-label={`Legende für ${TITLES[kind]}`}
    >
      <div className="font-medium mb-1">{TITLES[kind]}</div>
      <ul className="space-y-0.5">
        {entries.map(({ level, color }) => (
          <li key={level} className="flex items-center gap-2">
            <span
              className="inline-block w-3 h-3 rounded-sm shrink-0"
              style={{ backgroundColor: color }}
              aria-hidden="true"
            />
            <span className="font-medium tabular-nums">{level}</span>
          </li>
        ))}
        {kind === 'vf' && hasUnknownVf && (
          <li className="flex items-center gap-2 mt-1 pt-1 border-t border-neutral-200 dark:border-neutral-700">
            <span
              className="inline-block w-3 h-3 rounded-sm shrink-0"
              style={{ backgroundColor: VF_UNKNOWN_COLOR }}
              aria-hidden="true"
            />
            <span className="font-medium">K?</span>
            <span className="ml-auto text-[10px] text-neutral-500 dark:text-neutral-400">unscaled</span>
          </li>
        )}
      </ul>
    </div>
  )
}
