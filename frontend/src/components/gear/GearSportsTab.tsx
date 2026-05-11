import { useState, useMemo } from 'react'
import type { Gear, GearDefaultSportPredicate, GearDefaultModifiers } from '@/api/types'
import { GearApplicableModifierKeys } from '@/lib/generatedGearApplicableModifiers'
import { modifierLabel, type SportModifierKey } from '@/lib/sportModifiers'
import { selectableSportTupleOptions, sportDetailLabel } from '@/lib/sportTaxonomy'
import { defaultSportsForSubcategory } from '@/lib/gearSchemaHelpers'
import { GENERATED_SPORT_MODIFIERS } from '@/lib/generatedSportModifiers'

// ── Types ─────────────────────────────────────────────────────────────────────

type ModifierChoice = 'any' | 'true' | 'false'

// Lookup map: modifier key → { recommendedFor, notRecommendedFor } lists.
// Built once from the generated modifier data so the hot render path is O(1).
const MODIFIER_HINT_MAP: Partial<Record<string, { recommendedFor: readonly string[]; notRecommendedFor: readonly string[] }>> =
  Object.fromEntries(
    GENERATED_SPORT_MODIFIERS
      .filter(m => 'recommendedFor' in m || 'notRecommendedFor' in m)
      .map(m => [
        m.key,
        {
          recommendedFor: ('recommendedFor' in m ? m.recommendedFor : []) as readonly string[],
          notRecommendedFor: ('notRecommendedFor' in m ? m.notRecommendedFor : []) as readonly string[],
        },
      ]),
  )

/**
 * Returns 'recommended' when gearType is in the modifier's recommendedFor list
 * (and the modifier value is 'true'), 'unusual' when in notRecommendedFor, or
 * null when no hint applies.
 */
function modifierHint(
  modifierKey: string,
  choice: ModifierChoice,
  gearType: string,
): 'recommended' | 'unusual' | null {
  // Hints only make sense when the modifier is explicitly set to 'true'.
  if (choice !== 'true') return null
  const hints = MODIFIER_HINT_MAP[modifierKey]
  if (!hints) return null
  if (hints.recommendedFor.includes(gearType)) return 'recommended'
  if (hints.notRecommendedFor.includes(gearType)) return 'unusual'
  return null
}

interface PredicateRow {
  id: string
  sportKey: string
  modifiers: Partial<Record<SportModifierKey, ModifierChoice>>
}

interface GearSportsTabProps {
  item: Gear
  onSave: (predicates: GearDefaultSportPredicate[]) => void
  /** Gear type key (e.g. "bike.mtb.enduro"). Used to render modifier hints. */
  gearType?: string
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function sportTupleKey(sportType: string, subType?: string | null): string {
  return subType ? `${sportType}:${subType}` : sportType
}

function splitSportTupleKey(key: string): { sportType: string; subType?: string } {
  const sep = key.indexOf(':')
  if (sep < 0) return { sportType: key }
  const sportType = key.slice(0, sep)
  const subType = key.slice(sep + 1)
  return subType ? { sportType, subType } : { sportType }
}

function rowFromPredicate(predicate: GearDefaultSportPredicate, index: number): PredicateRow {
  const modifiers = predicate.modifiers ?? {}
  const modifierChoices: Partial<Record<SportModifierKey, ModifierChoice>> = {}
  for (const key of GearApplicableModifierKeys) {
    const raw = (modifiers as Record<string, unknown>)[key]
    if (raw === true) modifierChoices[key] = 'true'
    else if (raw === false) modifierChoices[key] = 'false'
    else modifierChoices[key] = 'any'
  }
  return {
    id: `row-${index}`,
    sportKey: sportTupleKey(predicate.sport_type, predicate.sub_type),
    modifiers: modifierChoices,
  }
}

function predicateFromRow(row: PredicateRow): GearDefaultSportPredicate | null {
  const { sportType, subType } = splitSportTupleKey(row.sportKey)
  if (!sportType) return null
  const modifiersOut: GearDefaultModifiers = {}
  let hasModifier = false
  for (const key of GearApplicableModifierKeys) {
    const choice = row.modifiers[key]
    if (choice === 'true') {
      ;(modifiersOut as Record<string, boolean>)[key] = true
      hasModifier = true
    } else if (choice === 'false') {
      ;(modifiersOut as Record<string, boolean>)[key] = false
      hasModifier = true
    }
  }
  return {
    sport_type: sportType,
    sub_type: subType ?? null,
    modifiers: hasModifier ? modifiersOut : null,
  }
}

// Returns the subcategory attribute key for the item's type, or null if the
// type does not carry a subcategory attribute.
function subcategoryAttributeKey(typeKey: string): string | null {
  if (typeKey === 'helmet') return 'helmet.subcategory'
  if (typeKey === 'goggles') return 'goggles.subcategory'
  if (typeKey.startsWith('apparel.')) return 'apparel.subcategory'
  return null
}

function rowsFromGear(gear: Gear): PredicateRow[] {
  const stored = gear.applicable_sport_predicates ?? []
  if (stored.length > 0) {
    return stored.map((p, i) => rowFromPredicate(p, i))
  }

  // No user-set predicates yet — derive defaults from the subcategory attribute.
  const attrKey = subcategoryAttributeKey(gear.type ?? '')
  const subcategoryValue = attrKey
    ? String((gear.attributes as Record<string, unknown> | null | undefined)?.[attrKey] ?? '')
    : ''
  const defaults = defaultSportsForSubcategory(gear.type ?? '', subcategoryValue || undefined)
  return defaults.map((p, i) => rowFromPredicate(p, i))
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function GearSportsTab({ item, onSave, gearType }: GearSportsTabProps) {
  const [rows, setRows] = useState<PredicateRow[]>(() => rowsFromGear(item))
  // Resolve gear type: explicit prop wins; fall back to item.type.
  const resolvedGearType = gearType ?? item.type ?? ''

  const sportOptions = useMemo(
    () =>
      selectableSportTupleOptions().map((opt) => ({
        value: sportTupleKey(opt.value, opt.subType),
        label: opt.subType ? sportDetailLabel(opt.value, opt.subType) : opt.label,
      })),
    [],
  )

  function addRow() {
    const sportKey = sportOptions[0]?.value ?? 'generic'
    const modifiers: Partial<Record<SportModifierKey, ModifierChoice>> = {}
    for (const key of GearApplicableModifierKeys) {
      modifiers[key] = 'any'
    }
    setRows((prev) => [...prev, { id: `row-${prev.length}`, sportKey, modifiers }])
  }

  function updateRow(index: number, patch: Partial<PredicateRow>) {
    setRows((prev) =>
      prev.map((row, i) => (i === index ? { ...row, ...patch } : row)),
    )
  }

  function removeRow(index: number) {
    setRows((prev) => prev.filter((_, i) => i !== index))
  }

  function handleSave() {
    const predicates = rows
      .map((r) => predicateFromRow(r))
      .filter((p): p is GearDefaultSportPredicate => p !== null)
    onSave(predicates)
  }

  return (
    <div className="space-y-4">
      <h3 className="text-sm font-semibold text-neutral-800 dark:text-neutral-200">
        Standard-Sportarten
      </h3>

      {rows.length === 0 && (
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          Noch keine Sportart-Zuordnung festgelegt.
        </p>
      )}

      {rows.map((row, index) => (
        <div
          key={row.id}
          className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-4 space-y-3"
        >
          <div className="flex items-center justify-between gap-2">
            <label
              htmlFor={`sport-${index}`}
              className="text-xs font-medium text-neutral-600 dark:text-neutral-400 shrink-0"
            >
              {`Sportart ${index + 1}`}
            </label>
            <select
              id={`sport-${index}`}
              aria-label={`Sportart ${index + 1}`}
              value={row.sportKey}
              onChange={(e) => updateRow(index, { sportKey: e.target.value })}
              className="flex-1 rounded border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-900 px-2 py-1 text-sm text-neutral-900 dark:text-neutral-100"
            >
              {sportOptions.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {opt.label}
                </option>
              ))}
            </select>
            <button
              type="button"
              onClick={() => removeRow(index)}
              className="text-xs text-rose-500 hover:text-rose-700 cursor-pointer"
              aria-label={`Sportart ${index + 1} entfernen`}
            >
              Entfernen
            </button>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            {GearApplicableModifierKeys.map((key) => (
              <ModifierSelect
                key={key}
                modifierKey={key}
                value={row.modifiers[key] ?? 'any'}
                ariaLabel={`${modifierLabel(key)} ${index + 1}`}
                gearType={resolvedGearType}
                onChange={(choice) =>
                  updateRow(index, {
                    modifiers: { ...row.modifiers, [key]: choice },
                  })
                }
              />
            ))}
          </div>
        </div>
      ))}

      <div className="flex gap-2">
        <button
          type="button"
          onClick={addRow}
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
        >
          Sportart hinzufügen
        </button>
        <button
          type="button"
          onClick={handleSave}
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors cursor-pointer"
        >
          Speichern
        </button>
      </div>
    </div>
  )
}

// ── Modifier select ───────────────────────────────────────────────────────────

function ModifierSelect({
  modifierKey,
  value,
  ariaLabel,
  gearType,
  onChange,
}: {
  modifierKey: SportModifierKey
  value: ModifierChoice
  ariaLabel: string
  /** Gear type key used to derive recommendedFor / notRecommendedFor hints. */
  gearType: string
  onChange: (choice: ModifierChoice) => void
}) {
  const label = modifierLabel(modifierKey)
  const id = `modifier-${ariaLabel.replace(/\s+/g, '-').toLowerCase()}`
  const hint = modifierHint(modifierKey, value, gearType)
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-xs text-neutral-500 dark:text-neutral-400">
        {label}
      </label>
      <select
        id={id}
        aria-label={ariaLabel}
        value={value}
        onChange={(e) => onChange(e.target.value as ModifierChoice)}
        className="rounded border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-900 px-2 py-1 text-sm text-neutral-900 dark:text-neutral-100"
      >
        <option value="any">Beliebig</option>
        <option value="true">Ja</option>
        <option value="false">Nein</option>
      </select>
      {hint === 'recommended' && (
        <span className="text-xs text-green-600 dark:text-green-400">✓ empfohlen</span>
      )}
      {hint === 'unusual' && (
        <span className="text-xs text-amber-500 dark:text-amber-400">⚠ ungewöhnlich</span>
      )}
    </div>
  )
}
