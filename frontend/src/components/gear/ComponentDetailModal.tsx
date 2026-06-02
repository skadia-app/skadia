import { useState, useEffect, useCallback, useRef } from 'react'
import type { GearSchema, ComponentType, GearTypeSlot, GearType, Attribute } from '@/lib/generatedGearSchema'
import type { GearComponent } from '@/api/types'
import { gearSchemaLabel, optionSetOptions } from '@/lib/gearSchemaHelpers'
import RecursiveSubfieldRenderer from '@/components/gear/RecursiveSubfieldRenderer'
import { deepEqual } from '@/lib/deepEqual'
import { isAttributeVisible, type EvalContext } from '@/lib/gearConditionEval'
import { supportedLanguageFrom } from '@/i18n/localizedString'
import { useTranslation } from 'react-i18next'

export interface ComponentDetailModalProps {
  /** The slot this component belongs to, or null for free components */
  slot: GearTypeSlot | null
  /** Existing component data, or null for a new component */
  component: GearComponent | null
  schema: GearSchema
  /**
   * Optional: the key of the gear this component belongs to. When set, the
   * preset-gear-type selector is narrowed to that gear type and its
   * descendants (via parentKey chain). When omitted, all gear types are
   * offered.
   */
  sourceGearTypeKey?: string
  /**
   * Current gear-level attribute values for cross-item visibility evaluation
   * (applies_to_gear_when). Pass {} or omit to disable cross-item filtering
   * (e.g. in tests that don't need it).
   */
  gearAttributes?: Record<string, unknown>
  onSave: (data: Partial<GearComponent>) => void
  onCancel: () => void
}

// gearTypeDescendants returns the set of gearType keys equal to or descended
// from rootKey via the parentKey chain. When rootKey is empty, the full set of
// keys is returned (no narrowing).
function gearTypeDescendants(gearTypes: GearType[], rootKey: string): Set<string> {
  if (!rootKey) {
    return new Set(gearTypes.map((g) => g.key))
  }
  const result = new Set<string>([rootKey])
  // Iterative closure — handles arbitrary depth without recursion. Worst case
  // O(N * depth); typical schema has <50 gear types and depth <4.
  let added = true
  while (added) {
    added = false
    for (const g of gearTypes) {
      if (result.has(g.key)) continue
      if (g.parentKey && result.has(g.parentKey)) {
        result.add(g.key)
        added = true
      }
    }
  }
  return result
}

export default function ComponentDetailModal({
  slot,
  component,
  schema,
  sourceGearTypeKey,
  gearAttributes = {},
  onSave,
  onCancel,
}: ComponentDetailModalProps) {
  const { t, i18n } = useTranslation()
  const language = supportedLanguageFrom(i18n.language)
  const strings = {
    componentType: t('gearZones.componentDetail.componentType'),
    noAttributes: t('gearZones.componentDetail.noAttributes'),
    selectType: t('gearZones.componentDetail.selectType'),
    presetGearType: t('gearZones.componentDetail.presetGearType'),
    presetGearTypeNone: t('gearZones.componentDetail.presetGearTypeNone'),
    presetGearTypeHelp: t('gearZones.componentDetail.presetGearTypeHelp'),
    unsavedChangesConfirm: t('gearZones.componentDetail.unsavedChangesConfirm'),
    customFieldsHeader: t('gearZones.componentDetail.customFieldsHeader'),
    customFieldsHint: t('gearZones.componentDetail.customFieldsHint'),
    addCustomField: t('gearZones.componentDetail.addCustomField'),
    customFieldName: t('gearZones.componentDetail.customFieldName'),
    customFieldValue: t('gearZones.componentDetail.customFieldValue'),
    customFieldSave: t('gearZones.componentDetail.customFieldSave'),
    customFieldCancel: t('gearZones.componentDetail.customFieldCancel'),
    deleteCustomField: (field: string) => t('gearZones.componentDetail.deleteCustomField', { field }),
    defaultComponentTitle: t('gearZones.componentDetail.defaultComponentTitle'),
    close: t('gearZones.componentDetail.close'),
    save: t('gearZones.componentDetail.save'),
    cancel: t('gearZones.componentDetail.cancel'),
  }
  const [componentTypeKey, setComponentTypeKey] = useState(
    component?.type ?? slot?.componentType ?? '',
  )
  const [attributes, setAttributes] = useState<Record<string, unknown>>(
    () => (typeof component?.attributes === 'object' && component.attributes ? component.attributes : {}),
  )
  const [presetGearTypeKey, setPresetGearTypeKey] = useState<string>(
    component?.preset_gear_type_key ?? '',
  )

  // Custom field add-row state
  const [customFieldName, setCustomFieldName] = useState('')
  const [customFieldValue, setCustomFieldValue] = useState('')
  const [showCustomFieldForm, setShowCustomFieldForm] = useState(false)

  // Snapshot of initial values to detect unsaved changes
  const initialTypeRef = useRef(component?.type ?? slot?.componentType ?? '')
  const initialAttributesRef = useRef(
    typeof component?.attributes === 'object' && component.attributes
      ? component.attributes
      : {} as Record<string, unknown>,
  )
  const initialPresetRef = useRef(component?.preset_gear_type_key ?? '')

  function isDirty(): boolean {
    if (componentTypeKey !== initialTypeRef.current) return true
    if (presetGearTypeKey !== initialPresetRef.current) return true
    // Key-order-independent comparison: a serialized JSON.stringify diff
    // would flag { a: 1, b: 2 } vs { b: 2, a: 1 } as dirty even though the
    // form is unchanged. deepEqual also treats `undefined` values as missing
    // keys, so deleting an attribute (setting it to undefined) reads as
    // dirty against an initial value, not against an absent key.
    return !deepEqual(attributes, initialAttributesRef.current)
  }

  // Prompts the user if there are unsaved changes, then calls onCancel if confirmed.
  const confirmCloseIfDirty = useCallback(() => {
    if (isDirty() && !window.confirm(strings.unsavedChangesConfirm)) return
    onCancel()
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [onCancel, componentTypeKey, attributes, presetGearTypeKey, strings])

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') confirmCloseIfDirty()
    },
    [confirmCloseIfDirty],
  )

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [handleKeyDown])

  // Find the resolved ComponentType from schema
  const componentTypes = schema.componentTypes ?? []
  const resolvedType: ComponentType | undefined = componentTypes.find(
    (ct) => ct.key === componentTypeKey,
  )

  // Resolve the subfields/attributes for the selected component type
  const attrByKey = new Map(schema.attributes.map((a) => [a.key, a]))
  const schemaAttributeKeys = new Set(resolvedType?.attributeKeys ?? [])
  const allComponentAttributes: Attribute[] = (resolvedType?.attributeKeys ?? [])
    .map((key) => attrByKey.get(key))
    .filter(Boolean)
    .map((a) => a!)

  // Build eval context for cross-item visibility
  const evalCtx: EvalContext = {
    self: attributes as Record<string, unknown>,
    gear: gearAttributes,
    component: attributes as Record<string, unknown>,
  }

  // Apply cross-item visibility filtering
  const componentAttributes = allComponentAttributes.filter((attr) =>
    isAttributeVisible(attr, evalCtx, { mode: 'cross_item' }),
  )

  // Custom fields: attribute keys in `attributes` that are NOT in the schema
  const customFields = Object.entries(attributes).filter(
    ([key]) => !schemaAttributeKeys.has(key),
  )

  function resolveOptions(optionSetKey: string | undefined) {
    return optionSetOptions(schema, optionSetKey)
  }

  function handleAttrChange(key: string, value: unknown) {
    setAttributes((prev) => ({ ...prev, [key]: value }))
  }

  function handleCustomFieldSave() {
    const trimmedKey = customFieldName.trim()
    const rawValue = customFieldValue.trim()
    if (!trimmedKey) return

    // Type-detect: number > boolean > string
    let parsed: unknown = rawValue
    if (rawValue === 'true') parsed = true
    else if (rawValue === 'false') parsed = false
    else if (rawValue !== '' && !Number.isNaN(Number(rawValue))) parsed = Number(rawValue)

    setAttributes((prev) => ({ ...prev, [trimmedKey]: parsed }))
    setCustomFieldName('')
    setCustomFieldValue('')
    setShowCustomFieldForm(false)
  }

  function handleCustomFieldDelete(key: string) {
    setAttributes((prev) => {
      const next = { ...prev }
      delete next[key]
      return next
    })
  }

  // Eligible preset gear types: the source gear's type and its descendants.
  // The empty option ("Keine Voreinstellung") clears the lock.
  const presetEligibleKeys = gearTypeDescendants(schema.gearTypes ?? [], sourceGearTypeKey ?? '')
  const presetOptions = (schema.gearTypes ?? []).filter((g) => presetEligibleKeys.has(g.key))

  function handleSave() {
    onSave({
      type: componentTypeKey,
      slot_key: slot?.key ?? null,
      attributes,
      // null clears the preset; the api/gear.ts wrapper translates null to ""
      // for the backend's three-state pointer semantics (omit=unchanged,
      // ""=clear, "key"=set). A non-empty value locks the component to that
      // gear type.
      preset_gear_type_key: presetGearTypeKey === '' ? null : presetGearTypeKey,
    })
  }

  const title = slot
    ? gearSchemaLabel(slot, language)
    : (component?.name ?? strings.defaultComponentTitle)

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={title}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={(e) => {
        // Close on backdrop click — with dirty-check
        if (e.target === e.currentTarget) confirmCloseIfDirty()
      }}
    >
      <div className="w-full max-w-lg rounded-xl bg-white dark:bg-neutral-900 shadow-2xl overflow-hidden max-h-[90vh] flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-neutral-200 dark:border-neutral-700">
          <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">{title}</h2>
          <button
            type="button"
            aria-label={strings.close}
            onClick={confirmCloseIfDirty}
            className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer"
          >
            ×
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-4">
          {/* Component type selector (shown when no fixed type from slot) */}
          {!slot && (
            <div className="flex flex-col gap-1">
              <label
                htmlFor="component-type-select"
                className="text-sm font-medium text-neutral-700 dark:text-neutral-300"
              >
                {strings.componentType}
              </label>
              <select
                id="component-type-select"
                value={componentTypeKey}
                onChange={(e) => {
                  setComponentTypeKey(e.target.value)
                  setAttributes({})
                }}
                className="w-full rounded border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-900 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
              >
                <option value="">{strings.selectType}</option>
                {componentTypes.map((ct) => (
                  <option key={ct.key} value={ct.key}>
                    {gearSchemaLabel(ct, language)}
                  </option>
                ))}
              </select>
            </div>
          )}

          {/* Preset gear-type lock (optional) — restricts where this component
              can be transferred. Hidden when no gear types are available in the
              schema to keep the form clean in minimal test schemas. */}
          {presetOptions.length > 0 && (
            <div className="flex flex-col gap-1">
              <label
                htmlFor="component-preset-gear-type-select"
                className="text-sm font-medium text-neutral-700 dark:text-neutral-300"
              >
                {strings.presetGearType}
              </label>
              <select
                id="component-preset-gear-type-select"
                value={presetGearTypeKey}
                onChange={(e) => setPresetGearTypeKey(e.target.value)}
                className="w-full rounded border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-900 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
              >
                <option value="">{strings.presetGearTypeNone}</option>
                {presetOptions.map((g) => (
                  <option key={g.key} value={g.key}>
                    {gearSchemaLabel(g, language)}
                  </option>
                ))}
              </select>
              <p className="text-xs text-neutral-500 dark:text-neutral-400">
                {strings.presetGearTypeHelp}
              </p>
            </div>
          )}

          {/* Schema-driven attributes with cross-item visibility */}
          {componentAttributes.length > 0 ? (
            <RecursiveSubfieldRenderer
              subfields={componentAttributes}
              values={attributes}
              prefix={componentTypeKey}
              depth={1}
              onChange={handleAttrChange}
              resolveOptions={resolveOptions}
            />
          ) : componentTypeKey ? (
            <p className="text-sm text-neutral-500 dark:text-neutral-400">
              {strings.noAttributes}
            </p>
          ) : null}

          {/* Custom fields section */}
          {componentTypeKey && (
            <div className="space-y-2 pt-2 border-t border-neutral-100 dark:border-neutral-800">
              <div className="flex items-center justify-between">
                <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
                  {strings.customFieldsHeader}
                </p>
              </div>
              {customFields.length > 0 && (
                <div className="rounded-lg bg-neutral-50 dark:bg-neutral-800/50 p-3 space-y-2">
                  <p className="text-xs text-neutral-400 dark:text-neutral-500">{strings.customFieldsHint}</p>
                  {customFields.map(([key, val]) => (
                    <div key={key} className="flex items-start gap-2">
                      <div className="flex-1 min-w-0">
                        <p className="text-xs font-medium text-neutral-600 dark:text-neutral-400 truncate">{key}</p>
                        <input
                          type="text"
                          value={val === null || val === undefined ? '' : String(val)}
                          onChange={(e) => handleAttrChange(key, e.target.value || undefined)}
                          className="mt-0.5 w-full rounded border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-900 px-2 py-1 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
                        />
                      </div>
                      <button
                        type="button"
                        aria-label={strings.deleteCustomField(key)}
                        onClick={() => handleCustomFieldDelete(key)}
                        className="mt-4 text-neutral-400 hover:text-rose-500 dark:hover:text-rose-400 cursor-pointer text-sm leading-none"
                      >
                        ✕
                      </button>
                    </div>
                  ))}
                </div>
              )}
              {showCustomFieldForm ? (
                <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-3 space-y-2 bg-neutral-50 dark:bg-neutral-800/50">
                  <input
                    type="text"
                    value={customFieldName}
                    onChange={(e) => setCustomFieldName(e.target.value)}
                    placeholder={strings.customFieldName}
                    className="w-full rounded border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-900 px-2 py-1.5 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
                  />
                  <input
                    type="text"
                    value={customFieldValue}
                    onChange={(e) => setCustomFieldValue(e.target.value)}
                    placeholder={strings.customFieldValue}
                    className="w-full rounded border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-900 px-2 py-1.5 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
                  />
                  <div className="flex gap-2">
                    <button
                      type="button"
                      onClick={handleCustomFieldSave}
                      disabled={!customFieldName.trim()}
                      className="px-3 py-1.5 text-xs font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed rounded cursor-pointer"
                    >
                      {strings.customFieldSave}
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        setShowCustomFieldForm(false)
                        setCustomFieldName('')
                        setCustomFieldValue('')
                      }}
                      className="px-3 py-1.5 text-xs font-medium text-neutral-600 dark:text-neutral-400 hover:text-neutral-800 dark:hover:text-neutral-200 cursor-pointer"
                    >
                      {strings.customFieldCancel}
                    </button>
                  </div>
                </div>
              ) : (
                <button
                  type="button"
                  onClick={() => setShowCustomFieldForm(true)}
                  className="text-sm font-medium text-accent-600 dark:text-orange-400 hover:underline cursor-pointer"
                >
                  {strings.addCustomField}
                </button>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex justify-end gap-3 px-5 py-4 border-t border-neutral-200 dark:border-neutral-700">
          <button
            type="button"
            onClick={confirmCloseIfDirty}
            className="px-4 py-2 rounded-lg text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 cursor-pointer"
          >
            {strings.cancel}
          </button>
          <button
            type="button"
            disabled={!componentTypeKey}
            onClick={handleSave}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
          >
            {strings.save}
          </button>
        </div>
      </div>
    </div>
  )
}
