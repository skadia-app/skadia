import type { GearComponent } from '@/api/types'

interface GearComponentsTabProps {
  components: GearComponent[]
}

interface TreeNode {
  component: GearComponent
  depth: number
}

// Flatten component tree: roots first, then children indented by depth.
function buildTree(components: GearComponent[]): TreeNode[] {
  const childrenByParent = new Map<string | null, GearComponent[]>()
  for (const comp of components) {
    const parentId = comp.parent_component_id ?? null
    const list = childrenByParent.get(parentId)
    if (list) {
      list.push(comp)
    } else {
      childrenByParent.set(parentId, [comp])
    }
  }

  const result: TreeNode[] = []

  function walk(parentId: string | null, depth: number) {
    const children = childrenByParent.get(parentId) ?? []
    for (const comp of children) {
      result.push({ component: comp, depth })
      walk(comp.id, depth + 1)
    }
  }

  walk(null, 0)

  // Any components not reachable from null (orphans) are appended at depth 0.
  const rendered = new Set(result.map((n) => n.component.id))
  for (const comp of components) {
    if (!rendered.has(comp.id)) {
      result.push({ component: comp, depth: 0 })
    }
  }

  return result
}

function formatComponentDistance(meters: number): string {
  return `${Math.round(Math.max(0, meters) / 1000)} km`
}

function formatComponentDuration(seconds: number): string {
  const minutes = Math.round(Math.max(0, seconds) / 60)
  if (minutes < 60) return `${minutes} min`
  return `${Math.round(minutes / 60)} h`
}

export default function GearComponentsTab({ components }: GearComponentsTabProps) {
  if (components.length === 0) {
    return (
      <p className="text-sm text-neutral-500 dark:text-neutral-400">
        Keine Komponenten vorhanden.
      </p>
    )
  }

  const tree = buildTree(components)

  return (
    <div className="space-y-2">
      {tree.map(({ component: comp, depth }) => (
        <div
          key={comp.id}
          data-testid={`component-${comp.id}`}
          data-depth={String(depth)}
          style={{ marginLeft: `${depth * 1.5}rem` }}
          className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-3"
        >
          <div className="flex items-center justify-between gap-2">
            <div className="min-w-0">
              <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate block">
                {comp.name}
              </span>
              {(comp.brand || comp.type) && (
                <span className="text-xs text-neutral-500 dark:text-neutral-400">
                  {[comp.brand, comp.type].filter(Boolean).join(' · ')}
                </span>
              )}
            </div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400 shrink-0 text-right">
              <div>{formatComponentDistance(comp.total_distance_m)}</div>
              <div>{formatComponentDuration(comp.total_time_s)}</div>
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}
