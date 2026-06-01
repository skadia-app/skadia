import { useTranslation } from 'react-i18next'
import type { Workout, WorkoutStep } from '@/api/types'
import { fmtDuration } from '@/lib/format'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

const INTENSITY_COLORS: Record<number, string> = {
  0: 'bg-orange-400', 1: 'bg-green-400', 2: 'bg-green-300', 3: 'bg-blue-400',
}

const INTENSITY_LABEL_KEYS: Record<number, string> = {
  0: 'interval', 1: 'warmup', 2: 'cooldown', 3: 'rest',
}

interface Props { workout: Workout }

function isRepeatStep(durationType?: number): boolean {
  return durationType != null && durationType >= 14 && durationType <= 19
}

function isTimeBasedStep(durationType?: number): boolean {
  return durationType === 0 || durationType == null
}

function stepWeight(step: WorkoutStep): number {
  if (isRepeatStep(step.duration_type)) return 0
  if (isTimeBasedStep(step.duration_type)) return step.duration_value ?? 0
  return 30000
}

export function WorkoutOverview({ workout }: Props) {
  const { t } = useTranslation()
  const { formatDistance } = useUnitFormatters()
  if (!workout.steps?.length) return null

  const renderSteps = workout.steps.filter(s => !isRepeatStep(s.duration_type))
  const totalWeight = renderSteps.reduce((acc, s) => acc + stepWeight(s), 0)
  if (totalWeight === 0) return null

  const repeatTargets = new Set(
    workout.steps.filter(s => isRepeatStep(s.duration_type)).map(s => s.duration_value ?? -1)
  )

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="px-4 py-3 bg-neutral-50 dark:bg-neutral-900/60 border-b border-neutral-200 dark:border-neutral-800">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
          {t('activityDetail.workout.title')}{workout.name ? `: ${workout.name}` : ''}
        </h3>
      </div>
      <div className="px-4 py-3">
        <div className="flex h-20 gap-0.5 rounded-lg overflow-hidden">
          {renderSteps.map((step, i) => {
            const w = stepWeight(step)
            const pct = (w / totalWeight) * 100
            if (pct < 0.5) return null
            const color = INTENSITY_COLORS[step.intensity ?? 0] ?? 'bg-neutral-400'
            const intensityKey = INTENSITY_LABEL_KEYS[step.intensity ?? 0]
            const label = step.step_name ?? (intensityKey ? t(`activityDetail.workout.intensity.${intensityKey}`) : '')
            const dur = isTimeBasedStep(step.duration_type) && step.duration_value != null
              ? fmtDuration(step.duration_value / 1000, 'short')
              : step.duration_type === 1 && step.duration_value != null
                ? formatDistance(step.duration_value)
                : ''
            const isLoopStart = repeatTargets.has(step.message_index)

            return (
              <div
                key={i}
                className={`${color} flex flex-col items-center justify-center text-white text-[10px] leading-tight overflow-hidden relative`}
                style={{ width: `${pct}%`, minWidth: pct > 3 ? undefined : '4px' }}
                title={`${label}${dur ? ` — ${dur}` : ''}`}
              >
                {pct > 8 && <span className="font-medium truncate px-1">{label}</span>}
                {pct > 12 && dur && <span className="opacity-80">{dur}</span>}
                {isLoopStart && (
                  <div className="absolute -bottom-0.5 left-0 right-0 h-1 bg-white/50 rounded-full" title={t('activityDetail.workout.repeatFromHere')} />
                )}
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
