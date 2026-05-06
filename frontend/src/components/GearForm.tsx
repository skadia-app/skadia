import { useState, useMemo } from 'react'
import type { Gear, GearDefaultModifiers, GearDefaultSportPredicate } from '@/api/types'
import { selectableSportTupleOptions, sportDetailLabel } from '@/lib/sportTaxonomy'

const GEAR_TYPES = [
  { value: 'bike', label: 'Fahrrad' },
  { value: 'e_bike', label: 'E-Bike' },
  { value: 'shoes', label: 'Laufschuhe' },
  { value: 'goggles', label: 'Schwimmbrille' },
  { value: 'ski', label: 'Ski' },
  { value: 'skis', label: 'Ski' },
  { value: 'snowboard', label: 'Snowboard' },
  { value: 'wetsuit', label: 'Neoprenanzug' },
  { value: 'racket', label: 'Schläger' },
  { value: 'paddle', label: 'Paddel' },
  { value: 'other', label: 'Sonstiges' },
]

interface GearFormProps {
  gear?: Gear | null
  onSave: (data: Partial<Gear>) => void
  onCancel: () => void
  isSaving?: boolean
}

type ModifierChoice = 'any' | 'true' | 'false'

interface GearPredicateRow {
  id: string
  sportKey: string
  isElectric: ModifierChoice
  isIndoor: ModifierChoice
  isVirtual: ModifierChoice
}

interface GearPredicateOption {
  value: string
  label: string
}

export default function GearForm({ gear, onSave, onCancel, isSaving }: GearFormProps) {
  const baseSportOptions = useMemo(
    () => selectableSportTupleOptions().map((option) => ({
      value: sportTupleKey(option.value, option.subType),
      label: option.subType ? sportDetailLabel(option.value, option.subType) : option.label,
    })),
    [],
  )
  const [name, setName] = useState(gear?.name ?? '')
  const [type, setType] = useState(gear?.type ?? '')
  const [subType, setSubType] = useState(gear?.sub_type ?? '')
  const [brand, setBrand] = useState(gear?.brand ?? '')
  const [model, setModel] = useState(gear?.model ?? '')
  const [purchasedAt, setPurchasedAt] = useState(gear?.purchased_at?.split('T')[0] ?? '')
  const [defaultPredicateRows, setDefaultPredicateRows] = useState<GearPredicateRow[]>(
    () => predicateRowsFromGear(gear),
  )
  const [notes, setNotes] = useState(gear?.notes ?? '')
  const [weightG, setWeightG] = useState(gear?.weight_g?.toString() ?? '')

  const sportOptions = useMemo(
    () => optionsIncludingCurrentRows(baseSportOptions, defaultPredicateRows),
    [baseSportOptions, defaultPredicateRows],
  )

  function addDefaultPredicate() {
    const sportKey = sportOptions[0]?.value ?? 'generic'
    setDefaultPredicateRows((prev) => [...prev, newPredicateRow(prev.length, sportKey)])
  }

  function updateDefaultPredicate(index: number, patch: Partial<GearPredicateRow>) {
    setDefaultPredicateRows((prev) => prev.map((row, rowIndex) => (
      rowIndex === index ? { ...row, ...patch } : row
    )))
  }

  function removeDefaultPredicate(index: number) {
    setDefaultPredicateRows((prev) => prev.filter((_, rowIndex) => rowIndex !== index))
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim() || !type) return
    const defaultSportPredicates = predicatesFromRows(defaultPredicateRows)
    onSave({
      name: name.trim(),
      type,
      sub_type: subType.trim() || null,
      brand: brand.trim() || null,
      model: model.trim() || null,
      purchased_at: purchasedAt || null,
      default_for_sports: sportTypesFromPredicates(defaultSportPredicates),
      default_sport_predicates: defaultSportPredicates,
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
        <div className="space-y-2">
          {defaultPredicateRows.map((row, index) => (
            <div
              key={row.id}
              className="grid grid-cols-1 lg:grid-cols-[minmax(0,2fr)_repeat(3,minmax(7rem,0.8fr))_auto] gap-2 items-end"
            >
              <label className="block">
                <span className="block text-xs text-neutral-500 dark:text-neutral-400 mb-1">
                  Sportart
                </span>
                <select
                  aria-label={`Standard ${index + 1} Sportart`}
                  value={row.sportKey}
                  onChange={(e) => updateDefaultPredicate(index, { sportKey: e.target.value })}
                  className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
                >
                  {sportOptions.map((option) => (
                    <option key={option.value} value={option.value}>{option.label}</option>
                  ))}
                </select>
              </label>
              <ModifierSelect
                label="E-Antrieb"
                ariaLabel={`Standard ${index + 1} E-Antrieb`}
                value={row.isElectric}
                onChange={(value) => updateDefaultPredicate(index, { isElectric: value })}
              />
              <ModifierSelect
                label="Indoor"
                ariaLabel={`Standard ${index + 1} Indoor`}
                value={row.isIndoor}
                onChange={(value) => updateDefaultPredicate(index, { isIndoor: value })}
              />
              <ModifierSelect
                label="Virtuell"
                ariaLabel={`Standard ${index + 1} Virtuell`}
                value={row.isVirtual}
                onChange={(value) => updateDefaultPredicate(index, { isVirtual: value })}
              />
              <button
                type="button"
                aria-label={`Standard ${index + 1} entfernen`}
                onClick={() => removeDefaultPredicate(index)}
                className="h-10 px-3 rounded-lg text-sm text-neutral-600 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 cursor-pointer"
              >
                Entfernen
              </button>
            </div>
          ))}
          <button
            type="button"
            onClick={addDefaultPredicate}
            className="text-sm font-medium text-accent-600 dark:text-orange-400 hover:underline cursor-pointer"
          >
            + Standard hinzufügen
          </button>
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
          className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
        >
          {isSaving ? 'Speichert...' : 'Speichern'}
        </button>
      </div>
    </form>
  )
}

function ModifierSelect({
  label,
  ariaLabel,
  value,
  onChange,
}: {
  label: string
  ariaLabel: string
  value: ModifierChoice
  onChange: (value: ModifierChoice) => void
}) {
  return (
    <label className="block">
      <span className="block text-xs text-neutral-500 dark:text-neutral-400 mb-1">{label}</span>
      <select
        aria-label={ariaLabel}
        value={value}
        onChange={(e) => onChange(e.target.value as ModifierChoice)}
        className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
      >
        <option value="any">egal</option>
        <option value="true">Ja</option>
        <option value="false">Nein</option>
      </select>
    </label>
  )
}

function predicateRowsFromGear(gear: Gear | null | undefined): GearPredicateRow[] {
  const predicates = gear?.default_sport_predicates ?? []
  if (predicates.length > 0) {
    return predicates.map((predicate, index) => predicateRowFromPredicate(predicate, index))
  }
  return (gear?.default_for_sports ?? []).map((sportType, index) => newPredicateRow(index, sportType))
}

function predicateRowFromPredicate(predicate: GearDefaultSportPredicate, index: number): GearPredicateRow {
  const modifiers = predicate.modifiers ?? {}
  return {
    id: `default-${index}`,
    sportKey: sportTupleKey(predicate.sport_type, predicate.sub_type ?? undefined),
    isElectric: modifierChoice(modifiers.is_electric),
    isIndoor: modifierChoice(modifiers.is_indoor),
    isVirtual: modifierChoice(modifiers.is_virtual),
  }
}

function newPredicateRow(index: number, sportKey: string): GearPredicateRow {
  return {
    id: `default-${Date.now()}-${index}`,
    sportKey,
    isElectric: 'any',
    isIndoor: 'any',
    isVirtual: 'any',
  }
}

function modifierChoice(value: boolean | undefined): ModifierChoice {
  if (value === true) return 'true'
  if (value === false) return 'false'
  return 'any'
}

function sportTupleKey(sportType: string, subType?: string | null): string {
  return subType ? `${sportType}:${subType}` : sportType
}

function splitSportTupleKey(key: string): { sportType: string; subType?: string } {
  const separator = key.indexOf(':')
  if (separator < 0) return { sportType: key }
  const sportType = key.slice(0, separator)
  const subType = key.slice(separator + 1)
  return subType ? { sportType, subType } : { sportType }
}

function predicatesFromRows(rows: GearPredicateRow[]): GearDefaultSportPredicate[] {
  const predicates: GearDefaultSportPredicate[] = []
  const seen = new Set<string>()
  for (const row of rows) {
    const tuple = splitSportTupleKey(row.sportKey)
    if (!tuple.sportType) continue
    const predicate: GearDefaultSportPredicate = { sport_type: tuple.sportType }
    if (tuple.subType) predicate.sub_type = tuple.subType
    const modifiers = modifiersFromRow(row)
    if (modifiers) predicate.modifiers = modifiers
    const key = stablePredicateKey(predicate)
    if (seen.has(key)) continue
    seen.add(key)
    predicates.push(predicate)
  }
  return predicates
}

function modifiersFromRow(row: GearPredicateRow): GearDefaultModifiers | null {
  const modifiers: GearDefaultModifiers = {}
  assignModifier(modifiers, 'is_electric', row.isElectric)
  assignModifier(modifiers, 'is_indoor', row.isIndoor)
  assignModifier(modifiers, 'is_virtual', row.isVirtual)
  return Object.keys(modifiers).length > 0 ? modifiers : null
}

function assignModifier(
  modifiers: GearDefaultModifiers,
  key: keyof GearDefaultModifiers,
  value: ModifierChoice,
) {
  if (value === 'true') modifiers[key] = true
  if (value === 'false') modifiers[key] = false
}

function sportTypesFromPredicates(predicates: GearDefaultSportPredicate[]): string[] {
  const sportTypes: string[] = []
  const seen = new Set<string>()
  for (const predicate of predicates) {
    if (seen.has(predicate.sport_type)) continue
    seen.add(predicate.sport_type)
    sportTypes.push(predicate.sport_type)
  }
  return sportTypes
}

function stablePredicateKey(predicate: GearDefaultSportPredicate): string {
  const modifiers = predicate.modifiers ?? {}
  return [
    predicate.sport_type,
    predicate.sub_type ?? '',
    modifierKeyPart('electric', modifiers.is_electric),
    modifierKeyPart('indoor', modifiers.is_indoor),
    modifierKeyPart('virtual', modifiers.is_virtual),
  ].join(':')
}

function modifierKeyPart(key: string, value: boolean | undefined): string {
  return value === undefined ? '' : `${key}=${value}`
}

function optionsIncludingCurrentRows(
  baseOptions: GearPredicateOption[],
  rows: GearPredicateRow[],
): GearPredicateOption[] {
  const options = [...baseOptions]
  const seen = new Set(options.map((option) => option.value))
  for (const row of rows) {
    if (seen.has(row.sportKey)) continue
    const tuple = splitSportTupleKey(row.sportKey)
    options.push({ value: row.sportKey, label: sportDetailLabel(tuple.sportType, tuple.subType) })
    seen.add(row.sportKey)
  }
  return options
}
