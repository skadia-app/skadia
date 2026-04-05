import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ModalShell } from '@/components/ui/ModalShell'
import { useToast } from '@/lib/toast'
import { createBloodPressure } from '@/api/healthMutations'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'

// ── ESH/ESC 2018 blood pressure categories ────────────────────────────────────

interface BPCategory {
  label: string
  color: string        // Tailwind text class
  bg: string           // Tailwind background/border classes
}

function classifyBloodPressure(systolic: number, diastolic: number): BPCategory {
  // Category is determined by the higher of the two values
  if (systolic < 120 && diastolic < 80) {
    return { label: 'Optimal', color: 'text-emerald-600 dark:text-emerald-400', bg: 'bg-emerald-50 dark:bg-emerald-950/20 border-emerald-200 dark:border-emerald-800/40' }
  }
  if (systolic < 130 && diastolic < 85) {
    return { label: 'Normal', color: 'text-teal-600 dark:text-teal-400', bg: 'bg-teal-50 dark:bg-teal-950/20 border-teal-200 dark:border-teal-800/40' }
  }
  if (systolic < 140 && diastolic < 90) {
    return { label: 'Hochnormal', color: 'text-yellow-600 dark:text-yellow-400', bg: 'bg-yellow-50 dark:bg-yellow-950/20 border-yellow-200 dark:border-yellow-800/40' }
  }
  if (systolic < 160 && diastolic < 100) {
    return { label: 'Hypertonie Grad 1', color: 'text-orange-600 dark:text-orange-400', bg: 'bg-orange-50 dark:bg-orange-950/20 border-orange-200 dark:border-orange-800/40' }
  }
  if (systolic < 180 && diastolic < 110) {
    return { label: 'Hypertonie Grad 2', color: 'text-red-600 dark:text-red-400', bg: 'bg-red-50 dark:bg-red-950/20 border-red-200 dark:border-red-800/40' }
  }
  return { label: 'Hypertonie Grad 3', color: 'text-rose-700 dark:text-rose-400', bg: 'bg-rose-100 dark:bg-rose-950/30 border-rose-300 dark:border-rose-700/50' }
}

function toLocalDateTimeString(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// ── Component ─────────────────────────────────────────────────────────────────

interface Props {
  onClose: () => void
}

export default function BloodPressureEntryDialog({ onClose }: Props) {
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  const [systolicStr, setSystolicStr] = useState('')
  const [diastolicStr, setDiastolicStr] = useState('')
  const [pulseStr, setPulseStr] = useState('')
  const [measuredAt, setMeasuredAt] = useState(() => toLocalDateTimeString(new Date()))

  const systolic = parseInt(systolicStr, 10)
  const diastolic = parseInt(diastolicStr, 10)
  const pulse = pulseStr.trim() !== '' ? parseInt(pulseStr, 10) : undefined

  const isSystolicValid = !isNaN(systolic) && systolic >= 60 && systolic <= 300
  const isDiastolicValid = !isNaN(diastolic) && diastolic >= 30 && diastolic <= 200
  const isPulseValid = pulse === undefined || (!isNaN(pulse) && pulse >= 30 && pulse <= 250)
  const canPreview = isSystolicValid && isDiastolicValid
  const canSubmit = canPreview && isPulseValid

  const category = canPreview ? classifyBloodPressure(systolic, diastolic) : null

  const mutation = useMutation({
    mutationFn: () => {
      const measured = new Date(measuredAt).toISOString()
      return createBloodPressure({
        measured_at: measured,
        systolic,
        diastolic,
        pulse,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['health', 'blood-pressure'] })
      addToast({ type: 'success', title: 'Blutdruck gespeichert' })
      onClose()
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Speichern fehlgeschlagen', message: String(err), duration: 0 })
    },
  })

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-sm">
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-50 mb-1">
        Blutdruck erfassen
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

        {/* Systolic + Diastolic side by side */}
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
              Systolisch <span className="text-red-500">*</span>
            </label>
            <div className="relative">
              <input
                type="number"
                min={60}
                max={300}
                placeholder="120"
                value={systolicStr}
                onChange={e => setSystolicStr(e.target.value)}
                className={AUTH_INPUT_CLASS}
              />
              <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
                mmHg
              </span>
            </div>
            {systolicStr !== '' && !isSystolicValid && (
              <p className="mt-1 text-xs text-red-500">60–300 mmHg</p>
            )}
          </div>

          <div>
            <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
              Diastolisch <span className="text-red-500">*</span>
            </label>
            <div className="relative">
              <input
                type="number"
                min={30}
                max={200}
                placeholder="80"
                value={diastolicStr}
                onChange={e => setDiastolicStr(e.target.value)}
                className={AUTH_INPUT_CLASS}
              />
              <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
                mmHg
              </span>
            </div>
            {diastolicStr !== '' && !isDiastolicValid && (
              <p className="mt-1 text-xs text-red-500">30–200 mmHg</p>
            )}
          </div>
        </div>

        {/* Pulse */}
        <div>
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            Puls (optional)
          </label>
          <div className="relative">
            <input
              type="number"
              min={30}
              max={250}
              placeholder="70"
              value={pulseStr}
              onChange={e => setPulseStr(e.target.value)}
              className={AUTH_INPUT_CLASS}
            />
            <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-400 pointer-events-none">
              bpm
            </span>
          </div>
          {pulseStr !== '' && !isPulseValid && (
            <p className="mt-1 text-xs text-red-500">30–250 bpm</p>
          )}
        </div>

        {/* Category badge */}
        {category !== null && (
          <div className={`rounded-lg border p-3 ${category.bg}`}>
            <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-0.5">
              ESH/ESC 2018 Kategorie
            </div>
            <div className={`text-sm font-semibold ${category.color}`}>
              {category.label}
            </div>
            <div className="mt-1 text-xs text-neutral-400">
              {systolic}/{diastolic} mmHg
              {pulse !== undefined && !isNaN(pulse) ? ` · ${pulse} bpm` : ''}
            </div>
          </div>
        )}
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
