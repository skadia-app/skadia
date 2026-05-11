import type { Attribute } from '@/lib/generatedGearSchema'
import SearchableEnumInput from '@/components/gear/inputs/SearchableEnumInput'
import UnitNumberInput from '@/components/gear/inputs/UnitNumberInput'
import MultiEnumInput from '@/components/gear/inputs/MultiEnumInput'
import ConditionalField from '@/components/gear/inputs/ConditionalField'
import type { OptionValue } from '@/lib/generatedGearSchema'
import { isAttributeVisible, type EvalContext } from '@/lib/gearConditionEval'

export interface RecursiveSubfieldRendererProps {
  subfields: Attribute[]
  values: Record<string, unknown>
  /** Dot-prefix for generating unique input ids */
  prefix: string
  /** Recursion depth guard (max 5) */
  depth: number
  onChange: (key: string, value: unknown) => void
  /** Option resolver — maps optionSetKey to OptionValue[]. Pass from parent. */
  resolveOptions: (optionSetKey: string | undefined) => OptionValue[]
  /**
   * Eval context for visible_when / applies_to_gear_when conditions. When
   * provided, each subfield is tested via isAttributeVisible before rendering.
   * The self-scope is overridden per recursion level to the local values object;
   * gear and component cross-references are threaded through unchanged.
   */
  evalCtx?: EvalContext
}

export default function RecursiveSubfieldRenderer({
  subfields,
  values,
  prefix,
  depth,
  onChange,
  resolveOptions,
  evalCtx,
}: RecursiveSubfieldRendererProps) {
  if (depth > 5) return null

  return (
    <>
      {subfields.map((sf) => {
        const inputId = `${prefix}.${sf.key}`

        // Evaluate visible_when / applies_to_gear_when at every recursion level.
        // self is always the local values object so sibling-references resolve
        // correctly even when nested. Cross-item refs (gear.*, component.*) pass
        // through from the parent evalCtx unchanged.
        if (evalCtx) {
          const localCtx: EvalContext = {
            self: values,
            gear: evalCtx.gear,
            component: evalCtx.component,
          }
          if (!isAttributeVisible(sf, localCtx, { mode: 'same_item' })) return null
        }

        return (
          <ConditionalField key={sf.key} attribute={sf} siblingValues={values}>
            {sf.subfields && sf.subfields.length > 0 ? (
              <fieldset className="border border-neutral-200 dark:border-neutral-700 rounded p-3 space-y-2">
                <legend className="text-xs font-medium text-neutral-600 dark:text-neutral-400 px-1">
                  {sf.labelDe}
                </legend>
                <RecursiveSubfieldRenderer
                  subfields={sf.subfields}
                  values={
                    typeof values[sf.key] === 'object' && values[sf.key] !== null
                      ? (values[sf.key] as Record<string, unknown>)
                      : {}
                  }
                  prefix={inputId}
                  depth={depth + 1}
                  onChange={(childKey, childVal) => {
                    const parent =
                      typeof values[sf.key] === 'object' && values[sf.key] !== null
                        ? (values[sf.key] as Record<string, unknown>)
                        : {}
                    onChange(sf.key, { ...parent, [childKey]: childVal })
                  }}
                  resolveOptions={resolveOptions}
                  evalCtx={evalCtx ? { self: values, gear: evalCtx.gear, component: evalCtx.component } : undefined}
                />
              </fieldset>
            ) : (
              <div className="flex flex-col gap-1">
                <label
                  htmlFor={inputId}
                  className="text-xs font-medium text-neutral-600 dark:text-neutral-400"
                >
                  {sf.labelDe}
                  {sf.required && (
                    <span className="ml-1 text-rose-500" aria-hidden="true">*</span>
                  )}
                </label>
                {renderLeafInput(sf, inputId, values[sf.key], onChange, resolveOptions)}
              </div>
            )}
          </ConditionalField>
        )
      })}
    </>
  )
}

function renderLeafInput(
  sf: Attribute,
  inputId: string,
  currentValue: unknown,
  onChange: (key: string, value: unknown) => void,
  resolveOptions: (optionSetKey: string | undefined) => OptionValue[],
) {
  const baseClass =
    'rounded border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-900 px-2 py-1 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT'

  switch (sf.valueType) {
    case 'enum': {
      const opts = resolveOptions(sf.optionSetKey)
      return (
        <SearchableEnumInput
          id={inputId}
          value={typeof currentValue === 'string' ? currentValue : ''}
          options={opts}
          onChange={(v) => onChange(sf.key, v)}
          allowFreeText={false}
        />
      )
    }
    case 'search_enum': {
      const opts = resolveOptions(sf.optionSetKey)
      return (
        <SearchableEnumInput
          id={inputId}
          value={typeof currentValue === 'string' ? currentValue : ''}
          options={opts}
          onChange={(v) => onChange(sf.key, v)}
          allowFreeText
        />
      )
    }
    case 'enum_with_freetext': {
      const opts = resolveOptions(sf.optionSetKey)
      return (
        <SearchableEnumInput
          id={inputId}
          value={typeof currentValue === 'string' ? currentValue : ''}
          options={opts}
          onChange={(v) => onChange(sf.key, v)}
          allowFreeText
        />
      )
    }
    case 'multi_enum': {
      const opts = resolveOptions(sf.optionSetKey)
      const arrVal = Array.isArray(currentValue)
        ? (currentValue as string[])
        : []
      return (
        <MultiEnumInput
          id={inputId}
          value={arrVal}
          options={opts}
          onChange={(v) => onChange(sf.key, v)}
        />
      )
    }
    case 'unit_number': {
      const unitVal =
        currentValue !== null && typeof currentValue === 'object' &&
        'value' in (currentValue as object)
          ? (currentValue as { value: number | null; unit: string })
          : null
      return (
        <UnitNumberInput
          id={inputId}
          value={unitVal}
          allowedUnits={sf.allowedUnits ?? []}
          onChange={(v) => onChange(sf.key, v)}
        />
      )
    }
    case 'number':
      return (
        <input
          id={inputId}
          type="number"
          value={currentValue === null || currentValue === undefined ? '' : String(currentValue)}
          onChange={(e) => {
            const raw = e.target.value
            onChange(sf.key, raw === '' ? undefined : parseFloat(raw))
          }}
          className={baseClass}
        />
      )
    case 'boolean':
      return (
        <input
          id={inputId}
          type="checkbox"
          checked={currentValue === true}
          onChange={(e) => onChange(sf.key, e.target.checked)}
          className="h-4 w-4 rounded border-neutral-300 text-accent-DEFAULT focus:ring-2 focus:ring-accent-DEFAULT"
        />
      )
    case 'object':
      // Container without subfields shouldn't reach here — the fieldset branch
      // above renders subfields. If we hit this path, the container is empty;
      // render nothing rather than a misleading control.
      return null
    case 'date':
      return (
        <input
          id={inputId}
          type="date"
          value={typeof currentValue === 'string' ? currentValue : ''}
          onChange={(e) => onChange(sf.key, e.target.value || undefined)}
          className={baseClass}
        />
      )
    default:
      // string and fallback
      return (
        <input
          id={inputId}
          type="text"
          value={currentValue === null || currentValue === undefined ? '' : String(currentValue)}
          onChange={(e) => onChange(sf.key, e.target.value || undefined)}
          className={baseClass}
        />
      )
  }
}
