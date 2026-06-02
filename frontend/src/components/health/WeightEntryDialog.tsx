import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ModalShell } from '@/components/ui/ModalShell'
import { useToast } from '@/lib/toast'
import { createWeightEntry } from '@/api/healthMutations'
import { getUserProfile } from '@/api/settings'
import { fmtLocalDateTimeInput } from '@/lib/format'
import { weightFromDisplayValue, weightInputUnit, weightToDisplayValue } from '@/lib/units'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'

function parseOptionalFloat(s: string): number | null {
  if (s.trim() === '') return null
  const v = parseFloat(s)
  return isNaN(v) ? null : v
}

// ── Component ─────────────────────────────────────────────────────────────────

interface Props {
  onClose: () => void
}

export default function WeightEntryDialog({ onClose }: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const { options: unitOptions } = useUnitFormatters()

  const [measuredAt, setMeasuredAt] = useState(() => fmtLocalDateTimeInput(new Date()))
  const [weightStr, setWeightStr] = useState('')
  const [bmiStr, setBmiStr] = useState('')
  const [bodyFatStr, setBodyFatStr] = useState('')
  const [showExtended, setShowExtended] = useState(false)

  // Extended fields
  const [muscleMassStr, setMuscleMassStr] = useState('')
  const [boneMassStr, setBoneMassStr] = useState('')
  const [proteinPctStr, setProteinPctStr] = useState('')
  const [subcutFatStr, setSubcutFatStr] = useState('')
  const [skelMusclePctStr, setSkelMusclePctStr] = useState('')
  const [leanBodyMassStr, setLeanBodyMassStr] = useState('')
  const [totalBodyWaterStr, setTotalBodyWaterStr] = useState('')

  // Load profile to auto-compute BMI
  const { data: profile } = useQuery({
    queryKey: ['user-profile'],
    queryFn: getUserProfile,
    staleTime: 5 * 60 * 1000,
  })

  const displayedWeight = parseFloat(weightStr)
  const weightKg = Number.isNaN(displayedWeight) ? NaN : weightFromDisplayValue(displayedWeight, unitOptions.units)
  const weightUnit = weightInputUnit(unitOptions.units)
  const weightPlaceholder = unitOptions.units === 'imperial' ? '154.0' : '70.0'
  const massPlaceholder = unitOptions.units === 'imperial' ? '110.0' : '50.0'
  const boneMassPlaceholder = unitOptions.units === 'imperial' ? '6.6' : '3.0'
  const leanBodyMassPlaceholder = unitOptions.units === 'imperial' ? '121.0' : '55.0'
  const isWeightValid = !isNaN(weightKg) && weightKg > 0 && weightKg < 700

  const parseOptionalDisplayWeightKg = (s: string): number | null => {
    const value = parseOptionalFloat(s)
    return value == null ? null : weightFromDisplayValue(value, unitOptions.units)
  }

  // Auto-compute BMI from profile height if weight is valid and BMI field is empty
  const heightCm = profile?.height_cm ?? null
  const computedBmi =
    bmiStr.trim() === '' && isWeightValid && heightCm != null && heightCm > 0
      ? parseFloat(((weightKg / Math.pow(heightCm / 100, 2))).toFixed(1))
      : null

  const bmiDisplay = bmiStr.trim() !== '' ? parseOptionalFloat(bmiStr) : computedBmi

  const canSubmit = isWeightValid

  const mutation = useMutation({
    mutationFn: () => {
      const weightG = Math.round(weightKg * 1000)
      const bmi = bmiStr.trim() !== '' ? parseOptionalFloat(bmiStr) : computedBmi

      // Stored health mass fields remain metric even when the UI displays pounds.
      const leanBodyMassKg = parseOptionalDisplayWeightKg(leanBodyMassStr)
      const leanBodyMassG = leanBodyMassKg != null ? Math.round(leanBodyMassKg * 1000) : null

      const muscleMassKg = parseOptionalDisplayWeightKg(muscleMassStr)
      const muscleMassG = muscleMassKg != null ? Math.round(muscleMassKg * 1000) : null
      const boneMassKg = parseOptionalDisplayWeightKg(boneMassStr)
      const boneMassG = boneMassKg != null ? Math.round(boneMassKg * 1000) : null

      return createWeightEntry({
        measured_at_utc: new Date(measuredAt).toISOString(),
        weight_g: weightG,
        bmi: bmi ?? undefined,
        body_fat_pct: parseOptionalFloat(bodyFatStr) ?? undefined,
        muscle_mass_g: muscleMassG ?? undefined,
        bone_mass_g: boneMassG ?? undefined,
        subcutaneous_fat_pct: parseOptionalFloat(subcutFatStr) ?? undefined,
        protein_pct: parseOptionalFloat(proteinPctStr) ?? undefined,
        skeletal_muscle_pct: parseOptionalFloat(skelMusclePctStr) ?? undefined,
        lean_body_mass_g: leanBodyMassG ?? undefined,
        total_body_water_l: parseOptionalFloat(totalBodyWaterStr) ?? undefined,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['health-weight'] })
      queryClient.invalidateQueries({ queryKey: ['health-overview'] })
      addToast({ type: 'success', title: t('healthDialogs.weight.saved') })
      onClose()
    },
    onError: (err) => {
      addToast({ type: 'error', title: t('healthDialogs.saveFailed'), message: String(err), duration: 0 })
    },
  })

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-md" className="max-h-[90vh] overflow-y-auto">
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-50 mb-1">
        {t('healthDialogs.weight.title')}
      </h2>
      <p className="text-xs text-neutral-400 dark:text-neutral-500 mb-5">
        {t('healthDialogs.manualMeasurement')}
      </p>

      <div className="space-y-4">
        {/* Date/time */}
        <div>
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            {t('healthDialogs.dateTime')}
          </label>
          <input
            type="datetime-local"
            value={measuredAt}
            onChange={e => setMeasuredAt(e.target.value)}
            className={AUTH_INPUT_CLASS}
          />
        </div>

        {/* Weight */}
        <div>
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            {t('healthDialogs.weight.weight')} <span className="text-red-500">*</span>
          </label>
          <div className="relative">
            <input
              type="number"
              min={weightToDisplayValue(0.1, unitOptions.units)}
              max={weightToDisplayValue(699, unitOptions.units)}
              step={0.1}
              placeholder={weightPlaceholder}
              value={weightStr}
              onChange={e => setWeightStr(e.target.value)}
              className={AUTH_INPUT_CLASS}
            />
            <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
              {weightUnit}
            </span>
          </div>
          {weightStr !== '' && !isWeightValid && (
            <p className="mt-1 text-xs text-red-500">{t('healthDialogs.weight.invalidWeight')}</p>
          )}
        </div>

        {/* Basic section — BMI + Body Fat */}
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
              BMI
            </label>
            <div className="relative">
              <input
                type="number"
                min={10}
                max={60}
                step={0.1}
                placeholder={computedBmi != null ? String(computedBmi) : '22.0'}
                value={bmiStr}
                onChange={e => setBmiStr(e.target.value)}
                className={AUTH_INPUT_CLASS}
              />
            </div>
            {computedBmi != null && bmiStr.trim() === '' && (
              <p className="mt-1 text-xs text-neutral-400">
                {t('healthDialogs.weight.autoBmi', { value: computedBmi })}
              </p>
            )}
            {bmiDisplay != null && (
              <p className="mt-1 text-xs text-neutral-500">
                {bmiDisplay < 18.5 ? t('healthDialogs.weight.bmiClasses.underweight') :
                 bmiDisplay < 25 ? t('healthDialogs.weight.bmiClasses.normal') :
                 bmiDisplay < 30 ? t('healthDialogs.weight.bmiClasses.overweight') :
                 t('healthDialogs.weight.bmiClasses.obesity')}
              </p>
            )}
          </div>

          <div>
            <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
              {t('healthDialogs.weight.bodyFat')}
            </label>
            <div className="relative">
              <input
                type="number"
                min={1}
                max={60}
                step={0.1}
                placeholder="20.0"
                value={bodyFatStr}
                onChange={e => setBodyFatStr(e.target.value)}
                className={AUTH_INPUT_CLASS}
              />
              <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
                %
              </span>
            </div>
          </div>
        </div>

        {/* Extended accordion */}
        <div>
          <button
            type="button"
            onClick={() => setShowExtended(v => !v)}
            className="cursor-pointer w-full flex items-center justify-between text-sm font-medium text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200 py-2 transition-colors"
          >
            <span>{t('healthDialogs.weight.extendedBodyComposition')}</span>
            <svg
              className={`w-4 h-4 transition-transform ${showExtended ? 'rotate-180' : ''}`}
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              aria-hidden="true"
            >
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
            </svg>
          </button>

          {showExtended && (
            <div className="space-y-3 pt-2 pb-1 border-t border-neutral-200 dark:border-neutral-700">
              {/* Row 1: Muscle Mass + Bone Mass */}
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1">
                    {t('healthDialogs.weight.muscleMass')}
                  </label>
                  <div className="relative">
                    <input
                      type="number"
                      min={0}
                      step={0.1}
                      placeholder={massPlaceholder}
                      value={muscleMassStr}
                      onChange={e => setMuscleMassStr(e.target.value)}
                      className={AUTH_INPUT_CLASS}
                    />
                    <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
                      {weightUnit}
                    </span>
                  </div>
                </div>

                <div>
                  <label className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1">
                    {t('healthDialogs.weight.boneMass')}
                  </label>
                  <div className="relative">
                    <input
                      type="number"
                      min={0}
                      step={0.1}
                      placeholder={boneMassPlaceholder}
                      value={boneMassStr}
                      onChange={e => setBoneMassStr(e.target.value)}
                      className={AUTH_INPUT_CLASS}
                    />
                    <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
                      {weightUnit}
                    </span>
                  </div>
                </div>
              </div>

              {/* Additional body-comp fields (hydration, visceral fat, metabolic age) are imported from synced devices */}

              {/* Row 2: Protein % + Subcutaneous Fat % */}
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1">
                    {t('healthDialogs.weight.protein')}
                  </label>
                  <div className="relative">
                    <input
                      type="number"
                      min={0}
                      max={100}
                      step={0.1}
                      placeholder="18.0"
                      value={proteinPctStr}
                      onChange={e => setProteinPctStr(e.target.value)}
                      className={AUTH_INPUT_CLASS}
                    />
                    <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
                      %
                    </span>
                  </div>
                </div>

                <div>
                  <label className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1">
                    {t('healthDialogs.weight.subcutaneousFat')}
                  </label>
                  <div className="relative">
                    <input
                      type="number"
                      min={0}
                      max={100}
                      step={0.1}
                      placeholder="15.0"
                      value={subcutFatStr}
                      onChange={e => setSubcutFatStr(e.target.value)}
                      className={AUTH_INPUT_CLASS}
                    />
                    <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
                      %
                    </span>
                  </div>
                </div>
              </div>

              {/* Row 4: Skeletal Muscle % + Lean Body Mass */}
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1">
                    {t('healthDialogs.weight.skeletalMuscle')}
                  </label>
                  <div className="relative">
                    <input
                      type="number"
                      min={0}
                      max={100}
                      step={0.1}
                      placeholder="40.0"
                      value={skelMusclePctStr}
                      onChange={e => setSkelMusclePctStr(e.target.value)}
                      className={AUTH_INPUT_CLASS}
                    />
                    <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
                      %
                    </span>
                  </div>
                </div>

                <div>
                  <label className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1">
                    {t('healthDialogs.weight.leanBodyMass')}
                  </label>
                  <div className="relative">
                    <input
                      type="number"
                      min={0}
                      step={0.1}
                      placeholder={leanBodyMassPlaceholder}
                      value={leanBodyMassStr}
                      onChange={e => setLeanBodyMassStr(e.target.value)}
                      className={AUTH_INPUT_CLASS}
                    />
                    <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
                      {weightUnit}
                    </span>
                  </div>
                </div>
              </div>

              {/* Row 5: Total Body Water */}
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1">
                    {t('healthDialogs.weight.bodyWater')}
                  </label>
                  <div className="relative">
                    <input
                      type="number"
                      min={0}
                      step={0.1}
                      placeholder="40.0"
                      value={totalBodyWaterStr}
                      onChange={e => setTotalBodyWaterStr(e.target.value)}
                      className={AUTH_INPUT_CLASS}
                    />
                    <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
                      L
                    </span>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Actions */}
      <div className="mt-6 flex gap-2 justify-end">
        <button
          onClick={onClose}
          className="cursor-pointer px-4 py-2 rounded-md text-sm font-medium bg-neutral-200 hover:bg-neutral-300 dark:bg-neutral-800 dark:hover:bg-neutral-700 text-neutral-900 dark:text-neutral-50"
        >
          {t('common.cancel')}
        </button>
        <button
          onClick={() => mutation.mutate()}
          disabled={!canSubmit || mutation.isPending}
          className="cursor-pointer px-4 py-2 rounded-md text-sm font-medium bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {mutation.isPending ? t('healthDialogs.saving') : t('healthDialogs.save')}
        </button>
      </div>
    </ModalShell>
  )
}
