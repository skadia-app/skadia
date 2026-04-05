import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ModalShell } from '@/components/ui/ModalShell'
import { useToast } from '@/lib/toast'
import { createWeightEntry } from '@/api/healthMutations'
import { getUserProfile } from '@/api/settings'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'

function toLocalDateTimeString(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function parseOptionalFloat(s: string): number | null {
  if (s.trim() === '') return null
  const v = parseFloat(s)
  return isNaN(v) ? null : v
}

function parseOptionalInt(s: string): number | null {
  if (s.trim() === '') return null
  const v = parseInt(s, 10)
  return isNaN(v) ? null : v
}

// ── Component ─────────────────────────────────────────────────────────────────

interface Props {
  onClose: () => void
}

export default function WeightEntryDialog({ onClose }: Props) {
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  const [measuredAt, setMeasuredAt] = useState(() => toLocalDateTimeString(new Date()))
  const [weightStr, setWeightStr] = useState('')
  const [bmiStr, setBmiStr] = useState('')
  const [bodyFatStr, setBodyFatStr] = useState('')
  const [showExtended, setShowExtended] = useState(false)

  // Extended fields
  const [muscleMassStr, setMuscleMassStr] = useState('')
  const [boneMassStr, setBoneMassStr] = useState('')
  const [hydrationStr, setHydrationStr] = useState('')  // stored separately — not in backend create, display-only note
  const [visceralFatStr, setVisceralFatStr] = useState('')  // display-only note
  const [proteinPctStr, setProteinPctStr] = useState('')
  const [subcutFatStr, setSubcutFatStr] = useState('')
  const [skelMusclePctStr, setSkelMusclePctStr] = useState('')
  const [leanBodyMassStr, setLeanBodyMassStr] = useState('')
  const [totalBodyWaterStr, setTotalBodyWaterStr] = useState('')
  const [metabolicAgeStr, setMetabolicAgeStr] = useState('')  // display-only, not in backend create

  // Load profile to auto-compute BMI
  const { data: profile } = useQuery({
    queryKey: ['user-profile'],
    queryFn: getUserProfile,
    staleTime: 5 * 60 * 1000,
  })

  const weightKg = parseFloat(weightStr)
  const isWeightValid = !isNaN(weightKg) && weightKg > 0 && weightKg < 700

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

      // lean_body_mass_g: if entered in kg, convert to g
      const leanBodyMassKg = parseOptionalFloat(leanBodyMassStr)
      const leanBodyMassG = leanBodyMassKg != null ? Math.round(leanBodyMassKg * 1000) : null

      // muscle_mass and bone_mass entered in kg, convert to g
      const muscleMassKg = parseOptionalFloat(muscleMassStr)
      const muscleMassG = muscleMassKg != null ? Math.round(muscleMassKg * 1000) : null
      const boneMassKg = parseOptionalFloat(boneMassStr)
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
      addToast({ type: 'success', title: 'Gewicht gespeichert' })
      onClose()
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Speichern fehlgeschlagen', message: String(err), duration: 0 })
    },
  })

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-md" className="max-h-[90vh] overflow-y-auto">
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-50 mb-1">
        Gewicht erfassen
      </h2>
      <p className="text-xs text-neutral-400 dark:text-neutral-500 mb-5">
        Manuelle Messung eintragen
      </p>

      <div className="space-y-4">
        {/* Date/time */}
        <div>
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            Datum &amp; Uhrzeit
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
            Gewicht <span className="text-red-500">*</span>
          </label>
          <div className="relative">
            <input
              type="number"
              min={0.1}
              max={699}
              step={0.1}
              placeholder="70.0"
              value={weightStr}
              onChange={e => setWeightStr(e.target.value)}
              className={AUTH_INPUT_CLASS}
            />
            <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
              kg
            </span>
          </div>
          {weightStr !== '' && !isWeightValid && (
            <p className="mt-1 text-xs text-red-500">Bitte ein gültiges Gewicht eingeben</p>
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
                Auto: {computedBmi} kg/m²
              </p>
            )}
            {bmiDisplay != null && (
              <p className="mt-1 text-xs text-neutral-500">
                {bmiDisplay < 18.5 ? 'Untergewicht' :
                 bmiDisplay < 25 ? 'Normalgewicht' :
                 bmiDisplay < 30 ? 'Übergewicht' :
                 'Adipositas'}
              </p>
            )}
          </div>

          <div>
            <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
              Körperfett
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
            <span>Erweiterte Körperzusammensetzung</span>
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
                    Muskelmasse
                  </label>
                  <div className="relative">
                    <input
                      type="number"
                      min={0}
                      step={0.1}
                      placeholder="50.0"
                      value={muscleMassStr}
                      onChange={e => setMuscleMassStr(e.target.value)}
                      className={AUTH_INPUT_CLASS}
                    />
                    <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
                      kg
                    </span>
                  </div>
                </div>

                <div>
                  <label className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1">
                    Knochenmasse
                  </label>
                  <div className="relative">
                    <input
                      type="number"
                      min={0}
                      step={0.1}
                      placeholder="3.0"
                      value={boneMassStr}
                      onChange={e => setBoneMassStr(e.target.value)}
                      className={AUTH_INPUT_CLASS}
                    />
                    <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
                      kg
                    </span>
                  </div>
                </div>
              </div>

              {/* Row 2: Hydration + Visceral Fat */}
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1">
                    Hydration
                  </label>
                  <div className="relative">
                    <input
                      type="number"
                      min={0}
                      max={100}
                      step={0.1}
                      placeholder="55.0"
                      value={hydrationStr}
                      onChange={e => setHydrationStr(e.target.value)}
                      className={AUTH_INPUT_CLASS}
                      disabled
                    />
                    <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
                      %
                    </span>
                  </div>
                  <p className="mt-1 text-xs text-neutral-400">Nur aus Gerätesync</p>
                </div>

                <div>
                  <label className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1">
                    Viszeralfett
                  </label>
                  <div className="relative">
                    <input
                      type="number"
                      min={0}
                      placeholder=""
                      value={visceralFatStr}
                      onChange={e => setVisceralFatStr(e.target.value)}
                      className={AUTH_INPUT_CLASS}
                      disabled
                    />
                  </div>
                  <p className="mt-1 text-xs text-neutral-400">Nur aus Gerätesync</p>
                </div>
              </div>

              {/* Row 3: Protein % + Subcutaneous Fat % */}
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1">
                    Protein
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
                    Subkutanfett
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
                    Skelettmuskel
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
                    Fettfreie Masse
                  </label>
                  <div className="relative">
                    <input
                      type="number"
                      min={0}
                      step={0.1}
                      placeholder="55.0"
                      value={leanBodyMassStr}
                      onChange={e => setLeanBodyMassStr(e.target.value)}
                      className={AUTH_INPUT_CLASS}
                    />
                    <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
                      kg
                    </span>
                  </div>
                </div>
              </div>

              {/* Row 5: Total Body Water + Metabolic Age */}
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1">
                    Körperwasser
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

                <div>
                  <label className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1">
                    Metabolisches Alter
                  </label>
                  <div className="relative">
                    <input
                      type="number"
                      min={1}
                      max={120}
                      step={1}
                      placeholder=""
                      value={metabolicAgeStr}
                      onChange={e => setMetabolicAgeStr(e.target.value)}
                      className={AUTH_INPUT_CLASS}
                      disabled
                    />
                    <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
                      J
                    </span>
                  </div>
                  <p className="mt-1 text-xs text-neutral-400">Nur aus Gerätesync</p>
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
          Abbrechen
        </button>
        <button
          onClick={() => mutation.mutate()}
          disabled={!canSubmit || mutation.isPending}
          className="cursor-pointer px-4 py-2 rounded-md text-sm font-medium bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {mutation.isPending ? 'Speichern…' : 'Speichern'}
        </button>
      </div>
    </ModalShell>
  )
}
