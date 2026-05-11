import { useState, useMemo, useEffect } from 'react'
import type { Gear, GearDefaultModifiers, GearDefaultSportPredicate, GearComponent } from '@/api/types'
import { selectableSportTupleOptions, sportDetailLabel } from '@/lib/sportTaxonomy'
import { modifierLabel, type SportModifierKey } from '@/lib/sportModifiers'
import { GearApplicableModifierKeys } from '@/lib/generatedGearApplicableModifiers'
import { getActiveSchema, lookupGearType } from '@/lib/gearSchema'
import type { Attribute, RecommendedAttribute } from '@/lib/gearSchema'
import { gearTypeOptionsForScope, gearTypeAttributes, optionSetOptions } from '@/lib/gearSchemaHelpers'
import type { GearCategoryScope, GearTypeOption } from '@/lib/gearSchemaTypes'
import ComponentSlotList from '@/components/gear/ComponentSlotList'
import { isAttributeVisible, type EvalContext } from '@/lib/gearConditionEval'

// ── Scope-aware placeholders ──────────────────────────────────────────────────

const NAME_PLACEHOLDER: Record<GearCategoryScope, string> = {
  bike: 'z.B. Canyon Spectral',
  shoes: 'z.B. Trail-Schuh Sommer',
  winter: 'z.B. Tourenski Winter',
  water: 'z.B. SUP-Board See',
  diving: 'z.B. Atemregler Set',
  climb: 'z.B. Klettergurt',
  apparel: 'z.B. Regenjacke',
  other: 'z.B. Ersatzteilbox',
}

const WEIGHT_PLACEHOLDER: Record<GearCategoryScope, string> = {
  bike: 'z.B. 13500',
  shoes: 'z.B. 280',
  winter: 'z.B. 1800',
  water: 'z.B. 9500',
  diving: 'z.B. 850',
  climb: 'z.B. 400',
  apparel: 'z.B. 200',
  other: 'z.B. 850',
}

// ── Form prop types ───────────────────────────────────────────────────────────

interface GearFormProps {
  gear?: Gear | null
  categoryScope?: GearCategoryScope
  onSave: (data: Partial<Gear>) => void
  onCancel: () => void
  isSaving?: boolean
}

// ── Sport-predicate types (Plan-1-Konsum: GearApplicableModifierKeys) ─────────

type ModifierChoice = 'any' | 'true' | 'false'
const GEAR_DEFAULT_MODIFIER_KEYS = [...GearApplicableModifierKeys] as SportModifierKey[]

interface GearPredicateRow {
  id: string
  sportKey: string
  modifiers: Partial<Record<SportModifierKey, ModifierChoice>>
}

interface GearPredicateOption {
  value: string
  label: string
}

// ── Scope derivation from gear type key ───────────────────────────────────────

function scopeForType(typeKey: string): GearCategoryScope {
  if (!typeKey) return 'other'
  const schema = getActiveSchema()
  const gt = schema.gearTypes.find(t => t.key === typeKey)
  const scope = gt?.scope as GearCategoryScope | undefined
  if (scope && scope in NAME_PLACEHOLDER) return scope
  return 'other'
}

// ── Main component ────────────────────────────────────────────────────────────

export default function GearForm({ gear, categoryScope, onSave, onCancel, isSaving }: GearFormProps) {
  const schema = getActiveSchema()
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
  const [attributes, setAttributes] = useState<Record<string, string>>(() => {
    const raw = gear?.attributes ?? {}
    return Object.fromEntries(
      Object.entries(raw as Record<string, unknown>).map(([k, v]) => [k, v == null ? '' : String(v)]),
    )
  })
  const [notes, setNotes] = useState(gear?.notes ?? '')
  const [weightG, setWeightG] = useState(gear?.weight_g?.toString() ?? '')
  const [components, setComponents] = useState<GearComponent[]>(gear?.components ?? [])

  // Derive scope: prefer explicit prop, then derive from type, then fallback to 'other'.
  const scope: GearCategoryScope = categoryScope ?? (type ? scopeForType(type) : 'other')

  // Schema-driven type options scoped to the current category.
  const typeOptions = useMemo((): GearTypeOption[] => {
    const scoped = gearTypeOptionsForScope(schema, scope)
    if (type && !scoped.some(opt => opt.value === type)) {
      const current = schema.gearTypes.find(gt => gt.key === type)
      if (current) {
        return [{ value: current.key, label: current.labelDe }, ...scoped]
      }
    }
    return scoped
  }, [schema, scope, type])

  // Auto-select single option for new gear (no gear.id) in 'other' scope.
  useEffect(() => {
    if (!gear && !type && scope === 'other' && typeOptions.length === 1) {
      setType(typeOptions[0].value)
    }
  }, [gear, scope, type, typeOptions])

  const selectedSchemaType = lookupGearType(type)
  const showCustomSubType = !selectedSchemaType || type === 'other'
  const selectedAttributes = type ? gearTypeAttributes(schema, type) : []
  const hasSlots = (selectedSchemaType?.slots?.length ?? 0) > 0

  const sportOptions = useMemo(
    () => optionsIncludingCurrentRows(baseSportOptions, defaultPredicateRows),
    [baseSportOptions, defaultPredicateRows],
  )
  const duplicateDefaultRowIds = useMemo(
    () => duplicatePredicateRowIds(defaultPredicateRows),
    [defaultPredicateRows],
  )
  const hasDuplicateDefaultPredicates = duplicateDefaultRowIds.size > 0

  function setAttribute(key: string, value: string) {
    setAttributes(prev => ({ ...prev, [key]: value }))
  }

  function handleComponentSave(data: Partial<GearComponent>, existingId?: string) {
    if (existingId) {
      setComponents(prev =>
        prev.map(c => (c.id === existingId ? { ...c, ...data } : c)),
      )
    } else {
      // Generate a local placeholder id until backend assigns a real one.
      const localId = `local-${Date.now()}`
      const component: GearComponent = {
        ...data,
        id: localId,
        gear_id: gear?.id ?? '',
        name: data.name ?? data.type ?? '',
        type: data.type ?? '',
        slot_key: data.slot_key ?? null,
        total_distance_m: data.total_distance_m ?? 0,
        total_elevation_gain_m: data.total_elevation_gain_m ?? 0,
        total_elevation_loss_m: data.total_elevation_loss_m ?? 0,
        total_moving_time_s: data.total_moving_time_s ?? 0,
        total_time_s: data.total_time_s ?? 0,
        total_activities: data.total_activities ?? 0,
        is_retired: data.is_retired ?? false,
        status: data.status ?? 'active',
        attributes: data.attributes ?? null,
      }
      setComponents(prev => [
        ...prev,
        component,
      ])
    }
  }

  // Persists the reorder of free (non-slotted) components into form state so
  // it round-trips through Submit. Slotted components keep their slot-driven
  // order; only the unslotted tail is reordered by the DnD handler.
  function handleFreeComponentReorder(orderedIds: string[]) {
    setComponents(prev => {
      const slotted = prev.filter(c => !!c.slot_key)
      const freeById = new Map(prev.filter(c => !c.slot_key).map(c => [c.id, c]))
      const reorderedFree: GearComponent[] = []
      for (const id of orderedIds) {
        const match = freeById.get(id)
        if (match) reorderedFree.push(match)
      }
      // Append any free components the caller didn't include (defence against
      // an out-of-sync caller) so we never silently drop data.
      for (const c of freeById.values()) {
        if (!orderedIds.includes(c.id)) reorderedFree.push(c)
      }
      return [...slotted, ...reorderedFree]
    })
  }

  function handleTypeChange(nextType: string) {
    setType(nextType)
    if (schema.gearTypes.some(gt => gt.key === nextType)) {
      setSubType('')
    }
  }

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
    if (hasDuplicateDefaultPredicates) return
    const defaultSportPredicates = predicatesFromRows(defaultPredicateRows)

    const allowedAttributeKeys = new Set(selectedAttributes.map(attr => attr.key))
    const payloadAttributes = Object.fromEntries(
      Object.entries(attributes)
        .filter(([key, value]) => allowedAttributeKeys.has(key) && value !== ''),
    )

    onSave({
      name: name.trim(),
      type,
      sub_type: showCustomSubType ? subType.trim() || null : null,
      brand: brand.trim() || null,
      model: model.trim() || null,
      purchased_at: purchasedAt || null,
      applicable_sport_predicates: defaultSportPredicates,
      notes: notes.trim() || null,
      attributes: payloadAttributes,
      weight_g: weightG ? parseInt(weightG, 10) : null,
      components: hasSlots ? components : undefined,
    })
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {/* Name */}
      <div>
        <label htmlFor="gear-name" className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
          Name *
        </label>
        <input
          id="gear-name"
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
          className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
          placeholder={NAME_PLACEHOLDER[scope]}
        />
      </div>

      {/* Type — schema-driven searchable combobox */}
      <SearchableOptionSelect
        label="Typ"
        value={type}
        options={typeOptions}
        placeholder="Typ suchen..."
        listboxLabel="Typ-Vorschläge"
        onChange={handleTypeChange}
      />

      {/* Custom sub-type for schema-unknown types */}
      {showCustomSubType && (
        <div>
          <label htmlFor="gear-sub-type" className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            Untertyp
          </label>
          <input
            id="gear-sub-type"
            type="text"
            value={subType}
            onChange={(e) => setSubType(e.target.value)}
            className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
            placeholder={type === 'other' ? 'z.B. Trainingsmatte, Helm, Werkzeugset' : 'z.B. MTB, Rennrad, Trail'}
          />
        </div>
      )}

      {/* Brand & Model */}
      <div className="grid grid-cols-2 gap-4">
        <div>
          <label htmlFor="gear-brand" className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            Marke
          </label>
          <input
            id="gear-brand"
            type="text"
            value={brand}
            onChange={(e) => setBrand(e.target.value)}
            className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
          />
        </div>
        <div>
          <label htmlFor="gear-model" className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            Modell
          </label>
          <input
            id="gear-model"
            type="text"
            value={model}
            onChange={(e) => setModel(e.target.value)}
            className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
          />
        </div>
      </div>

      {/* Purchased at & Weight */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div>
          <label htmlFor="gear-purchased-at" className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            Kaufdatum
          </label>
          <input
            id="gear-purchased-at"
            type="date"
            value={purchasedAt}
            onChange={(e) => setPurchasedAt(e.target.value)}
            className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
          />
        </div>
        <div>
          <label htmlFor="gear-weight" className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            Gewicht (g)
          </label>
          <input
            id="gear-weight"
            type="number"
            min={0}
            value={weightG}
            onChange={(e) => setWeightG(e.target.value)}
            className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
            placeholder={WEIGHT_PLACEHOLDER[scope]}
          />
        </div>
      </div>

      {/* Schema-driven attribute fields (top-level, always inline) */}
      {selectedAttributes.length > 0 && (
        <GearAttributeFields
          schema={schema}
          attributes={selectedAttributes}
          values={attributes}
          recommended={selectedSchemaType?.recommended?.attributes}
          onChange={setAttribute}
        />
      )}

      {/* Slot-aware component list — only for gear types that define slots (e.g. bikes) */}
      {hasSlots && selectedSchemaType && (
        <div className="space-y-2">
          <p className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
            Komponenten
          </p>
          <ComponentSlotList
            gearType={selectedSchemaType}
            schema={schema}
            components={components}
            onComponentSave={handleComponentSave}
            onFreeComponentReorder={handleFreeComponentReorder}
          />
        </div>
      )}

      {/* Sport predicate assignment (Plan-1 predicate-based, GearApplicableModifierKeys) */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          Standard für Sportarten
        </label>
        <div className="space-y-2">
          {defaultPredicateRows.map((row, index) => {
            const duplicated = duplicateDefaultRowIds.has(row.id)
            return (
              <div key={row.id} className="space-y-1">
                <div className="grid grid-cols-1 lg:grid-cols-[minmax(0,2fr)_repeat(3,minmax(7rem,0.8fr))_auto] gap-2 items-end">
                  <label className="block">
                    <span className="block text-xs text-neutral-500 dark:text-neutral-400 mb-1">
                      Sportart
                    </span>
                    <select
                      aria-label={`Standard ${index + 1} Sportart`}
                      value={row.sportKey}
                      onChange={(e) => updateDefaultPredicate(index, { sportKey: e.target.value })}
                      aria-invalid={duplicated}
                      className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
                    >
                      {sportOptions.map((option) => (
                        <option key={option.value} value={option.value}>{option.label}</option>
                      ))}
                    </select>
                  </label>
                  {GEAR_DEFAULT_MODIFIER_KEYS.map((key) => (
                    <ModifierSelect
                      key={key}
                      label={modifierLabel(key)}
                      ariaLabel={`Standard ${index + 1} ${modifierLabel(key)}`}
                      value={row.modifiers[key] ?? 'any'}
                      onChange={(value) => updateDefaultPredicate(index, {
                        modifiers: { ...row.modifiers, [key]: value },
                      })}
                    />
                  ))}
                  <button
                    type="button"
                    aria-label={`Standard ${index + 1} entfernen`}
                    onClick={() => removeDefaultPredicate(index)}
                    className="h-10 px-3 rounded-lg text-sm text-neutral-600 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 cursor-pointer"
                  >
                    Entfernen
                  </button>
                </div>
                {duplicated && (
                  <p className="text-xs text-rose-600 dark:text-rose-400">
                    Doppelter Standard: gleiche Sportart und Modifier bereits vorhanden.
                  </p>
                )}
              </div>
            )
          })}
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
        <label htmlFor="gear-notes" className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
          Notizen
        </label>
        <textarea
          id="gear-notes"
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
          disabled={isSaving || !name.trim() || !type || hasDuplicateDefaultPredicates}
          className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
        >
          {isSaving ? 'Speichert...' : 'Speichern'}
        </button>
      </div>
    </form>
  )
}

// ── Schema-driven attribute fields ───────────────────────────────────────────
//
// Renders the gear-level attribute fields with:
//   - visibility filtering via isAttributeVisible
//   - recommended values surfaced at the top of dropdowns with an "empfohlen" tag
//   - "+ Custom-Wert" free-text entry for constrained fields
//   - mullet toggle for wheel-size (collapses wheel_size_front / wheel_size_rear
//     into a single toggle + two pickers when activated)
//   - kombi side-by-side layout for shock eye-to-eye + stroke fields

const MULLET_WHEEL_PAIR = {
  front: 'bike.wheel_size_front',
  rear: 'bike.wheel_size_rear',
  unified: 'bike.wheel_size',
} as const

interface GearAttributeFieldsProps {
  schema: ReturnType<typeof getActiveSchema>
  attributes: Attribute[]
  values: Record<string, string>
  recommended?: RecommendedAttribute[]
  onChange: (key: string, value: string) => void
}

function GearAttributeFields({
  schema,
  attributes,
  values,
  recommended,
  onChange,
}: GearAttributeFieldsProps) {
  // Mullet toggle state
  const [mulletEnabled, setMulletEnabled] = useState(false)

  // Build eval context for visibility: self = current gear attributes
  const evalCtx: EvalContext = useMemo(
    () => ({ self: values, gear: values }),
    [values],
  )

  // Build a recommended map: attr.key → recommended values[]
  const recommendedMap = useMemo(() => {
    const m = new Map<string, (string | number | boolean)[]>()
    for (const ra of recommended ?? []) {
      if (ra.values && ra.values.length > 0) m.set(ra.key, ra.values)
    }
    return m
  }, [recommended])

  // Determine which attributes to skip entirely vs. show (visibility + mullet)
  const skippedKeys = useMemo(() => {
    const s = new Set<string>()
    if (mulletEnabled) {
      s.add(MULLET_WHEEL_PAIR.unified)
    } else {
      s.add(MULLET_WHEEL_PAIR.front)
      s.add(MULLET_WHEEL_PAIR.rear)
    }
    return s
  }, [mulletEnabled])

  // Whether the current gear type supports mullet (i.e. has both _front and _rear)
  const hasMulletPair = useMemo(
    () =>
      attributes.some(a => a.key === MULLET_WHEEL_PAIR.front) &&
      attributes.some(a => a.key === MULLET_WHEEL_PAIR.rear),
    [attributes],
  )

  // Build kombi-group membership map from schema: group key → attributes in order.
  // Attributes without a kombiGroup are not in this map.
  const kombiGroups = useMemo(() => {
    const map = new Map<string, Attribute[]>()
    for (const attr of attributes) {
      if (!attr.kombiGroup) continue
      const existing = map.get(attr.kombiGroup)
      if (existing) existing.push(attr)
      else map.set(attr.kombiGroup, [attr])
    }
    return map
  }, [attributes])

  // Track which kombi groups have already been rendered so we only emit them once.
  const renderedKombiGroups = new Set<string>()

  const rows: React.ReactNode[] = []

  for (const attr of attributes) {
    if (skippedKeys.has(attr.key)) continue

    // Visibility check (same_item mode for gear-level attributes)
    if (!isAttributeVisible(attr, evalCtx, { mode: 'same_item' })) continue

    // Mullet toggle — rendered alongside the unified wheel-size field
    if (attr.key === MULLET_WHEEL_PAIR.unified && hasMulletPair) {
      rows.push(
        <div key="mullet-group" className="col-span-full grid grid-cols-1 sm:grid-cols-2 gap-4 items-end">
          <GearAttributeInput
            schema={schema}
            attr={attr}
            values={values}
            recommendedValues={recommendedMap.get(attr.key)}
            onChange={onChange}
          />
          <div className="flex items-center gap-2 pb-1">
            <input
              id="mullet-toggle"
              type="checkbox"
              checked={mulletEnabled}
              onChange={(e) => {
                setMulletEnabled(e.target.checked)
                // Clear mullet fields when disabling
                if (!e.target.checked) {
                  onChange(MULLET_WHEEL_PAIR.front, '')
                  onChange(MULLET_WHEEL_PAIR.rear, '')
                } else {
                  // Clear unified field when enabling
                  onChange(MULLET_WHEEL_PAIR.unified, '')
                }
              }}
              className="h-4 w-4 rounded border-neutral-300 text-accent-DEFAULT focus:ring-2 focus:ring-accent-DEFAULT"
            />
            <label htmlFor="mullet-toggle" className="text-sm text-neutral-600 dark:text-neutral-400 cursor-pointer">
              Mullet (unterschiedliche Laufradgrößen vorne/hinten)
            </label>
          </div>
        </div>,
      )
      continue
    }

    // Mullet expanded pickers (front + rear side-by-side)
    if (attr.key === MULLET_WHEEL_PAIR.front && mulletEnabled) {
      // Find rear attr
      const rearAttr = attributes.find(a => a.key === MULLET_WHEEL_PAIR.rear)
      rows.push(
        <div key="mullet-pickers" className="col-span-full grid grid-cols-2 gap-4">
          <GearAttributeInput
            schema={schema}
            attr={attr}
            values={values}
            recommendedValues={recommendedMap.get(attr.key)}
            onChange={onChange}
          />
          {rearAttr && (
            <GearAttributeInput
              schema={schema}
              attr={rearAttr}
              values={values}
              recommendedValues={recommendedMap.get(rearAttr.key)}
              onChange={onChange}
            />
          )}
        </div>,
      )
      continue
    }

    // Schema-driven kombi-group: attributes sharing the same kombiGroup render
    // side-by-side. The first member triggers the group render; subsequent members
    // are skipped via renderedKombiGroups.
    if (attr.kombiGroup) {
      if (renderedKombiGroups.has(attr.kombiGroup)) continue
      renderedKombiGroups.add(attr.kombiGroup)

      const groupAttrs = (kombiGroups.get(attr.kombiGroup) ?? []).filter(
        a => isAttributeVisible(a, evalCtx, { mode: 'same_item' }),
      )

      // The generator enforces exactly 2; log a warning when that invariant is
      // violated at runtime (schema downgrade or generator bug).
      if (groupAttrs.length !== 2) {
        console.warn(
          `kombi_group "${attr.kombiGroup}" has ${groupAttrs.length} visible members; expected 2. Rendering as plain fields.`,
        )
        for (const ga of groupAttrs) {
          rows.push(
            <GearAttributeInput
              key={ga.key}
              schema={schema}
              attr={ga}
              values={values}
              recommendedValues={recommendedMap.get(ga.key)}
              onChange={onChange}
            />,
          )
        }
        continue
      }

      const [attrA, attrB] = groupAttrs
      // Build a compact readout for numeric kombi pairs (e.g. shock dimensions).
      const valA = values[attrA.key]
      const valB = values[attrB.key]
      const kombiReadout = valA && valB ? `${valA}×${valB} mm` : null

      rows.push(
        <div key={`kombi-${attr.kombiGroup}`} className="col-span-full space-y-1">
          <div className="grid grid-cols-2 gap-4">
            <GearAttributeInput
              schema={schema}
              attr={attrA}
              values={values}
              recommendedValues={recommendedMap.get(attrA.key)}
              onChange={onChange}
            />
            <GearAttributeInput
              schema={schema}
              attr={attrB}
              values={values}
              recommendedValues={recommendedMap.get(attrB.key)}
              onChange={onChange}
            />
          </div>
          {kombiReadout && (
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              Einbaumaß: <span className="font-medium text-neutral-700 dark:text-neutral-300">{kombiReadout}</span>
            </p>
          )}
        </div>,
      )
      continue
    }

    rows.push(
      <GearAttributeInput
        key={attr.key}
        schema={schema}
        attr={attr}
        values={values}
        recommendedValues={recommendedMap.get(attr.key)}
        onChange={onChange}
      />,
    )
  }

  if (rows.length === 0) return null

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
      {rows}
    </div>
  )
}

// ── Single gear attribute input ───────────────────────────────────────────────

const CUSTOM_VALUE_SENTINEL = '__custom__'

interface GearAttributeInputProps {
  schema: ReturnType<typeof getActiveSchema>
  attr: Attribute
  values: Record<string, string>
  recommendedValues?: (string | number | boolean)[]
  onChange: (key: string, value: string) => void
}

function GearAttributeInput({
  schema,
  attr,
  values,
  recommendedValues,
  onChange,
}: GearAttributeInputProps) {
  const [customMode, setCustomMode] = useState(false)
  const currentValue = values[attr.key] ?? ''
  const inputId = `gear-attr-${attr.key}`

  const opts = optionSetOptions(schema, attr.optionSetKey)
  const hasOptions = opts.length > 0

  // Detect if current value is custom (not in opts list)
  const isCurrentCustom =
    hasOptions && currentValue !== '' && !opts.some(o => o.key === currentValue)

  // Recommended option keys for quick lookup
  const recommendedSet = useMemo(
    () => new Set((recommendedValues ?? []).map(v => String(v))),
    [recommendedValues],
  )

  // Split opts into recommended-first, then divider, then rest
  const sortedOpts = useMemo(() => {
    if (!recommendedValues || recommendedValues.length === 0 || opts.length === 0) return null
    const rec = opts.filter(o => recommendedSet.has(o.key))
    const rest = opts.filter(o => !recommendedSet.has(o.key))
    return { rec, rest }
  }, [opts, recommendedValues, recommendedSet])

  // Show inline custom input when user picks the sentinel or current value is already custom
  const showCustomInput = customMode || isCurrentCustom

  function handleSelectChange(newVal: string) {
    if (newVal === CUSTOM_VALUE_SENTINEL) {
      setCustomMode(true)
      // Don't clear existing value yet — let user type in the text input
    } else {
      setCustomMode(false)
      onChange(attr.key, newVal)
    }
  }

  if (hasOptions) {
    return (
      <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300">
        {attr.labelDe}
        {showCustomInput ? (
          <div className="mt-1 flex gap-2">
            <input
              id={inputId}
              type="text"
              value={currentValue}
              onChange={(e) => onChange(attr.key, e.target.value)}
              placeholder="Eigener Wert..."
              className="flex-1 border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent text-sm"
            />
            <button
              type="button"
              onClick={() => {
                setCustomMode(false)
                onChange(attr.key, '')
              }}
              className="px-2 py-1 text-xs text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 cursor-pointer"
            >
              ✕
            </button>
          </div>
        ) : (
          <select
            id={inputId}
            value={currentValue}
            onChange={(e) => handleSelectChange(e.target.value)}
            className="mt-1 w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
          >
            <option value="">Bitte wählen...</option>
            {sortedOpts ? (
              <>
                {sortedOpts.rec.map(opt => (
                  <option key={opt.key} value={opt.key}>
                    ★ {opt.labelDe} (empfohlen)
                  </option>
                ))}
                {sortedOpts.rest.length > 0 && sortedOpts.rec.length > 0 && (
                  <option disabled value="">──────────</option>
                )}
                {sortedOpts.rest.map(opt => (
                  <option key={opt.key} value={opt.key}>{opt.labelDe}</option>
                ))}
              </>
            ) : (
              opts.map(opt => (
                <option key={opt.key} value={opt.key}>{opt.labelDe}</option>
              ))
            )}
            <option disabled value="">──────────</option>
            <option value={CUSTOM_VALUE_SENTINEL}>+ Custom-Wert</option>
          </select>
        )}
      </label>
    )
  }

  // No options: text / number input
  return (
    <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300">
      {attr.labelDe}
      <input
        id={inputId}
        type={attr.valueType === 'number' ? 'number' : 'text'}
        value={currentValue}
        onChange={(e) => onChange(attr.key, e.target.value)}
        className="mt-1 w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
      />
    </label>
  )
}

// ── Schema-driven searchable type select ──────────────────────────────────────

function SearchableOptionSelect({
  label,
  value,
  options,
  placeholder,
  listboxLabel,
  onChange,
}: {
  label: string
  value: string
  options: GearTypeOption[]
  placeholder: string
  listboxLabel: string
  onChange: (value: string) => void
}) {
  const inputId = `${label.toLowerCase()}-combobox`
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const selected = options.find(opt => opt.value === value)

  useEffect(() => {
    if (selected) setQuery(selected.label)
  }, [selected])

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase()
    if (!needle || selected?.label === query) return options
    return options.filter(opt => opt.label.toLowerCase().includes(needle))
  }, [options, query, selected])

  function choose(opt: GearTypeOption) {
    onChange(opt.value)
    setQuery(opt.label)
    setOpen(false)
  }

  return (
    <div
      className="relative"
      onBlur={(e) => {
        const nextFocus = e.relatedTarget as Node | null
        if (!nextFocus || !e.currentTarget.contains(nextFocus)) setOpen(false)
      }}
    >
      <label htmlFor={inputId} className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
        {label} *
      </label>
      <input
        id={inputId}
        role="combobox"
        aria-label={label}
        aria-expanded={open}
        aria-controls={`${inputId}-listbox`}
        aria-autocomplete="list"
        value={query}
        onFocus={() => setOpen(true)}
        onClick={() => setOpen(true)}
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            setOpen(false)
            e.currentTarget.blur()
          }
        }}
        onChange={(e) => {
          setQuery(e.target.value)
          setOpen(true)
          if (value) onChange('')
        }}
        placeholder={placeholder}
        className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
      />
      {open && (
        <div
          id={`${inputId}-listbox`}
          role="listbox"
          aria-label={listboxLabel}
          className="absolute z-20 mt-1 max-h-64 w-full overflow-y-auto rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-lg"
        >
          {filtered.length > 0 ? (
            filtered.map(opt => (
              <button
                key={opt.value}
                type="button"
                role="option"
                aria-selected={opt.value === value}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => choose(opt)}
                className="block w-full px-3 py-2 text-left text-sm text-neutral-800 dark:text-neutral-100 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer"
              >
                {opt.label}
              </button>
            ))
          ) : (
            <div className="px-3 py-2 text-sm text-neutral-400">Keine Treffer</div>
          )}
        </div>
      )}
    </div>
  )
}

// ── Modifier select for predicate rows ────────────────────────────────────────

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

// ── Predicate row helpers ─────────────────────────────────────────────────────

function predicateRowsFromGear(gear: Gear | null | undefined): GearPredicateRow[] {
  const predicates = gear?.applicable_sport_predicates ?? []
  return predicates.map((predicate, index) => predicateRowFromPredicate(predicate, index))
}

function predicateRowFromPredicate(predicate: GearDefaultSportPredicate, index: number): GearPredicateRow {
  const modifiers = predicate.modifiers ?? {}
  return {
    id: `default-${index}`,
    sportKey: sportTupleKey(predicate.sport_type, predicate.sub_type ?? undefined),
    modifiers: Object.fromEntries(
      GEAR_DEFAULT_MODIFIER_KEYS.map((key) => [key, modifierChoice(modifiers[key as keyof GearDefaultModifiers])]),
    ),
  }
}

function newPredicateRow(index: number, sportKey: string): GearPredicateRow {
  return {
    id: `default-${Date.now()}-${index}`,
    sportKey,
    modifiers: Object.fromEntries(GEAR_DEFAULT_MODIFIER_KEYS.map((key) => [key, 'any'])),
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
    const predicate = predicateFromRow(row)
    if (!predicate) continue
    const key = stablePredicateKey(predicate)
    if (seen.has(key)) continue
    seen.add(key)
    predicates.push(predicate)
  }
  return predicates
}

function duplicatePredicateRowIds(rows: GearPredicateRow[]): Set<string> {
  const firstByKey = new Map<string, string>()
  const duplicates = new Set<string>()
  for (const row of rows) {
    const predicate = predicateFromRow(row)
    if (!predicate) continue
    const key = stablePredicateKey(predicate)
    const first = firstByKey.get(key)
    if (first) {
      duplicates.add(first)
      duplicates.add(row.id)
    } else {
      firstByKey.set(key, row.id)
    }
  }
  return duplicates
}

function predicateFromRow(row: GearPredicateRow): GearDefaultSportPredicate | null {
  const tuple = splitSportTupleKey(row.sportKey)
  if (!tuple.sportType) return null
  const predicate: GearDefaultSportPredicate = { sport_type: tuple.sportType }
  if (tuple.subType) predicate.sub_type = tuple.subType
  const modifiers = modifiersFromRow(row)
  if (modifiers) predicate.modifiers = modifiers
  return predicate
}

function modifiersFromRow(row: GearPredicateRow): GearDefaultModifiers | null {
  const modifiers: GearDefaultModifiers = {}
  for (const key of GEAR_DEFAULT_MODIFIER_KEYS) {
    assignModifier(modifiers, key as keyof GearDefaultModifiers, row.modifiers[key] ?? 'any')
  }
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

function stablePredicateKey(predicate: GearDefaultSportPredicate): string {
  const modifiers = predicate.modifiers ?? {}
  const keyParts = GEAR_DEFAULT_MODIFIER_KEYS.map((key) => modifierKeyPart(key, modifiers[key as keyof GearDefaultModifiers]))
  return [
    predicate.sport_type,
    predicate.sub_type ?? '',
    ...keyParts,
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
