import { describe, expect, it, vi } from 'vitest'
import { targetObjectKindLabel, targetObjectPermissionLabel, targetObjectVisibilityLabel } from './TargetObjectDetailPage'

const t = vi.fn((key: string) => key)

describe('target object detail fallback labels', () => {
  it('uses structured target object labels and hides unknown codes', () => {
    expect(targetObjectKindLabel('region', 'protected_area', t)).toBe('targetObjects.regionKinds.protected_area')
    expect(targetObjectKindLabel('place', 'summit', t)).toBe('targetObjects.placeKinds.summit')
    expect(targetObjectKindLabel('place', 'new_place_code', t)).toBe('targetObjects.placeKinds.unknown')
    expect(targetObjectKindLabel('region', 'new_region_code', t)).toBe('targetObjects.regionKinds.unknown')
    expect(targetObjectVisibilityLabel('public', t)).toBe('targetObjects.visibility.public')
    expect(targetObjectVisibilityLabel('custom_visibility_code', t)).toBe('targetObjects.visibility.unknown')
  })

  it('localizes effective permissions and hides unknown permission codes', () => {
    expect(targetObjectPermissionLabel('edit_geometry', t)).toBe('targetObjects.permissions.edit_geometry')
    expect(targetObjectPermissionLabel('hide_or_archive', t)).toBe('targetObjects.permissions.hide_or_archive')
    expect(targetObjectPermissionLabel('teleport_target', t)).toBe('targetObjects.permissions.unknown')
  })
})
