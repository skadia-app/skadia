import { useState, useMemo } from 'react'
import type { Gear } from '@/api/types'
import { generateGearSportOptions } from '@/lib/sportOptions'

const GEAR_TYPES = [
  { value: 'bike', label: 'Fahrrad' },
  { value: 'shoes', label: 'Laufschuhe' },
  { value: 'goggles', label: 'Schwimmbrille' },
  { value: 'ski', label: 'Ski' },
  { value: 'paddle', label: 'Paddel' },
  { value: 'other', label: 'Sonstiges' },
]

interface GearFormProps {
  gear?: Gear | null
  onSave: (data: Partial<Gear>) => void
  onCancel: () => void
  isSaving?: boolean
}

export default function GearForm({ gear, onSave, onCancel, isSaving }: GearFormProps) {
  const sportOptions = useMemo(() => generateGearSportOptions(), [])
  const [name, setName] = useState(gear?.name ?? '')
  const [type, setType] = useState(gear?.type ?? '')
  const [subType, setSubType] = useState(gear?.sub_type ?? '')
  const [brand, setBrand] = useState(gear?.brand ?? '')
  const [model, setModel] = useState(gear?.model ?? '')
  const [purchasedAt, setPurchasedAt] = useState(gear?.purchased_at?.split('T')[0] ?? '')
  const [defaultForSports, setDefaultForSports] = useState<string[]>(gear?.default_for_sports ?? [])
  const [notes, setNotes] = useState(gear?.notes ?? '')
  const [weightG, setWeightG] = useState(gear?.weight_g?.toString() ?? '')

  function toggleSport(sport: string) {
    setDefaultForSports((prev) =>
      prev.includes(sport) ? prev.filter((s) => s !== sport) : [...prev, sport],
    )
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim() || !type) return
    onSave({
      name: name.trim(),
      type,
      sub_type: subType.trim() || null,
      brand: brand.trim() || null,
      model: model.trim() || null,
      purchased_at: purchasedAt || null,
      default_for_sports: defaultForSports,
      notes: notes.trim() || null,
      weight_g: weightG ? parseInt(weightG, 10) : null,
    })
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {/* Name */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
          Name *
        </label>
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
          className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
          placeholder="z.B. Canyon Spectral"
        />
      </div>

      {/* Type */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
          Typ *
        </label>
        <select
          value={type}
          onChange={(e) => setType(e.target.value)}
          required
          className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
        >
          <option value="">Bitte wählen...</option>
          {GEAR_TYPES.map(({ value, label }) => (
            <option key={value} value={value}>{label}</option>
          ))}
        </select>
      </div>

      {/* Sub type */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
          Untertyp
        </label>
        <input
          type="text"
          value={subType}
          onChange={(e) => setSubType(e.target.value)}
          className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
          placeholder="z.B. MTB, Rennrad, Trail"
        />
      </div>

      {/* Brand & Model */}
      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            Marke
          </label>
          <input
            type="text"
            value={brand}
            onChange={(e) => setBrand(e.target.value)}
            className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
          />
        </div>
        <div>
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            Modell
          </label>
          <input
            type="text"
            value={model}
            onChange={(e) => setModel(e.target.value)}
            className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
          />
        </div>
      </div>

      {/* Purchased at */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
          Kaufdatum
        </label>
        <input
          type="date"
          value={purchasedAt}
          onChange={(e) => setPurchasedAt(e.target.value)}
          className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
        />
      </div>

      {/* Weight */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
          Gewicht (g)
        </label>
        <input
          type="number"
          min={0}
          value={weightG}
          onChange={(e) => setWeightG(e.target.value)}
          className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
          placeholder="z.B. 8500"
        />
      </div>

      {/* Default for sports */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          Standard für Sportarten
        </label>
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          {sportOptions.map(({ value, label }) => (
            <label key={value} className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={defaultForSports.includes(value)}
                onChange={() => toggleSport(value)}
                className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT focus:ring-1"
              />
              <span className="text-sm text-neutral-700 dark:text-neutral-300">{label}</span>
            </label>
          ))}
        </div>
      </div>

      {/* Notes */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
          Notizen
        </label>
        <textarea
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
          rows={3}
          className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent resize-none"
        />
      </div>

      {/* Actions */}
      <div className="flex gap-3 justify-end pt-2">
        <button
          type="button"
          onClick={onCancel}
          className="cursor-pointer px-4 py-2 rounded-lg text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 transition-colors"
        >
          Abbrechen
        </button>
        <button
          type="submit"
          disabled={isSaving || !name.trim() || !type}
          className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {isSaving ? 'Speichert...' : 'Speichern'}
        </button>
      </div>
    </form>
  )
}
