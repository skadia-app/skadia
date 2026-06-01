import { useState, useRef, useEffect, useMemo } from 'react'
import type { GearSchema, GearType, GearTypeSlot } from '@/lib/generatedGearSchema'
import type { GearComponent } from '@/api/types'
import ComponentDetailModal from '@/components/gear/ComponentDetailModal'
import { supportedLanguageFrom } from '@/i18n/localizedString'
import { gearSchemaLabel } from '@/lib/gearSchemaHelpers'
import { useTranslation } from 'react-i18next'

export interface ComponentSlotListProps {
  gearType: GearType
  schema: GearSchema
  components: GearComponent[]
  onComponentSave: (data: Partial<GearComponent>, existingId?: string) => void
  /** Called with the new ordered array of free-component IDs after a drag-and-drop reorder */
  onFreeComponentReorder?: (orderedIds: string[]) => void
}

export default function ComponentSlotList({
  gearType,
  schema,
  components,
  onComponentSave,
  onFreeComponentReorder,
}: ComponentSlotListProps) {
  const { t, i18n } = useTranslation()
  const language = supportedLanguageFrom(i18n.language)
  const strings = {
    addComponent: t('gearZones.componentSlotList.addComponent'),
    optional: t('gearZones.componentSlotList.optional'),
    noComponent: t('gearZones.componentSlotList.noComponent'),
    componentArrow: t('gearZones.componentSlotList.componentArrow'),
  }
  const [openSlot, setOpenSlot] = useState<GearTypeSlot | null>(null)
  const [openFreeComponent, setOpenFreeComponent] = useState(false)
  const [editingComponent, setEditingComponent] = useState<GearComponent | null>(null)

  // Free components are ones the user hasn't pinned to a schema slot.
  // useMemo (not useState) keeps the derivation pure on every render.
  const freeComponents = useMemo(
    () => components.filter((c) => !c.slot_key),
    [components],
  )
  const freeIdsKey = useMemo(
    () => freeComponents.map((c) => c.id).join(','),
    [freeComponents],
  )

  // freeOrder is the locally-tracked ordering of free-component IDs. It seeds
  // from the props' order and resets whenever the set of ids changes (set
  // identity, not array reference) — that way StrictMode's double-effect-fire
  // does not clobber a mid-drag reorder, and a save round-trip that returns
  // the same ids in a different order doesn't fight the user.
  const [freeOrder, setFreeOrder] = useState<string[]>(() => freeComponents.map((c) => c.id))
  useEffect(() => {
    setFreeOrder((prev) => {
      const prevKey = prev.join(',')
      if (prevKey === freeIdsKey) return prev
      return freeComponents.map((c) => c.id)
    })
    // freeIdsKey is the set-identity signal; freeComponents is the value source.
  }, [freeIdsKey, freeComponents])

  const dragSourceId = useRef<string | null>(null)

  const slots = gearType.slots ?? []

  // Build ordered free-component list from local order state
  const freeById = new Map(freeComponents.map((c) => [c.id, c]))
  const orderedFreeComponents = freeOrder
    .map((id) => freeById.get(id))
    .filter((c): c is GearComponent => !!c)

  // Find the component that matches a slot key
  function componentForSlot(slotKey: string): GearComponent | undefined {
    return components.find((c) => c.slot_key === slotKey)
  }

  // Brief one-line summary for a component (e.g. brand + model or just type)
  function componentSummary(comp: GearComponent): string {
    const parts = [comp.brand, comp.model].filter(Boolean)
    return parts.length > 0 ? parts.join(' ') : comp.name
  }

  function handleSlotClick(slot: GearTypeSlot) {
    const existing = componentForSlot(slot.key)
    setEditingComponent(existing ?? null)
    setOpenSlot(slot)
    setOpenFreeComponent(false)
  }

  function handleAddFree() {
    setEditingComponent(null)
    setOpenFreeComponent(true)
    setOpenSlot(null)
  }

  function handleSave(data: Partial<GearComponent>) {
    onComponentSave(data, editingComponent?.id)
    setOpenSlot(null)
    setOpenFreeComponent(false)
    setEditingComponent(null)
  }

  function handleCancel() {
    setOpenSlot(null)
    setOpenFreeComponent(false)
    setEditingComponent(null)
  }

  // ── Drag-and-drop handlers for free components ────────────────────────────

  function handleDragStart(id: string) {
    dragSourceId.current = id
  }

  function handleDragOver(e: React.DragEvent<HTMLButtonElement>) {
    // Allow drop
    e.preventDefault()
  }

  function handleDrop(targetId: string) {
    const sourceId = dragSourceId.current
    if (!sourceId || sourceId === targetId) return
    dragSourceId.current = null

    const next = [...freeOrder]
    const fromIdx = next.indexOf(sourceId)
    const toIdx = next.indexOf(targetId)
    if (fromIdx === -1 || toIdx === -1) return
    next.splice(fromIdx, 1)
    next.splice(toIdx, 0, sourceId)
    setFreeOrder(next)
    onFreeComponentReorder?.(next)
  }

  return (
    <div className="space-y-1">
      {slots.map((slot) => {
        const existing = componentForSlot(slot.key)
        return (
          <button
            key={slot.key}
            type="button"
            data-testid={`slot-${slot.key}`}
            onClick={() => handleSlotClick(slot)}
            className="w-full flex items-center justify-between gap-2 px-3 py-2 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 hover:bg-neutral-50 dark:hover:bg-neutral-800 text-left cursor-pointer transition-colors"
          >
            <span className="flex items-center gap-2 min-w-0">
              <span className="text-sm font-medium text-neutral-800 dark:text-neutral-100 truncate">
                {gearSchemaLabel(slot, language)}
              </span>
              {slot.optional && (
                <span className="text-xs text-neutral-400 dark:text-neutral-500 shrink-0">
                  {strings.optional}
                </span>
              )}
            </span>
            <span className="flex items-center gap-1 shrink-0 text-sm text-neutral-500 dark:text-neutral-400">
              {existing ? componentSummary(existing) : strings.noComponent}
              <span aria-hidden="true" className="text-neutral-300 dark:text-neutral-600">
                {strings.componentArrow}
              </span>
            </span>
          </button>
        )
      })}

      {/* Free components (not tied to a slot) — draggable for reordering */}
      {orderedFreeComponents.map((comp) => (
        <button
          key={comp.id}
          type="button"
          draggable
          data-testid={`free-component-${comp.id}`}
          onDragStart={() => handleDragStart(comp.id)}
          onDragOver={handleDragOver}
          onDrop={() => handleDrop(comp.id)}
          onClick={() => {
            setEditingComponent(comp)
            setOpenFreeComponent(true)
            setOpenSlot(null)
          }}
          className="w-full flex items-center justify-between gap-2 px-3 py-2 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 hover:bg-neutral-50 dark:hover:bg-neutral-800 text-left cursor-grab active:cursor-grabbing transition-colors"
        >
          <span className="text-sm font-medium text-neutral-800 dark:text-neutral-100 truncate">
            {comp.name}
          </span>
          <span aria-hidden="true" className="text-neutral-300 dark:text-neutral-600">
            {strings.componentArrow}
          </span>
        </button>
      ))}

      {/* Add custom component */}
      <button
        type="button"
        data-testid="add-component-button"
        onClick={handleAddFree}
        className="w-full px-3 py-2 text-sm font-medium text-accent-600 dark:text-orange-400 hover:underline text-left cursor-pointer"
      >
        {strings.addComponent}
      </button>

      {/* Slot modal */}
      {(openSlot || openFreeComponent) && (
        <ComponentDetailModal
          slot={openSlot}
          component={editingComponent}
          schema={schema}
          sourceGearTypeKey={gearType.key}
          onSave={handleSave}
          onCancel={handleCancel}
        />
      )}
    </div>
  )
}
