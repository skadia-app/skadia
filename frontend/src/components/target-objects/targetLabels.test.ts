import { describe, expect, it, vi } from 'vitest'
import {
  collectionKindLabel,
  memberKindLabel,
  observationTypeLabel,
  placeKindLabel,
  regionKindLabel,
  reportStatusLabel,
  reportTypeLabel,
  targetVisibilityLabel,
} from './targetLabels'

const t = vi.fn((key: string) => key)

describe('target object and collection labels', () => {
  it('uses catalog keys for known enum values', () => {
    expect(targetVisibilityLabel('private', t)).toBe('targetObjects.visibility.private')
    expect(placeKindLabel('summit', t)).toBe('targetObjects.placeKinds.summit')
    expect(regionKindLabel('protected_area', t)).toBe('targetObjects.regionKinds.protected_area')
    expect(collectionKindLabel('route_set', t)).toBe('targetCollections.kinds.route_set')
    expect(memberKindLabel('privacy_zone', t)).toBe('targetCollectionDetail.memberKinds.privacy_zone')
    expect(reportTypeLabel('condition', t)).toBe('targetOverlays.reports.types.condition')
    expect(reportStatusLabel('open', t)).toBe('targetOverlays.reports.status.open')
    expect(observationTypeLabel('matched_traversal', t)).toBe('targetOverlays.observations.types.matched_traversal')
  })

  it('uses category-specific unknown labels for unknown backend codes', () => {
    expect(targetVisibilityLabel('partner_only', t)).toBe('targetObjects.visibility.unknown')
    expect(placeKindLabel('trailhead_v2', t)).toBe('targetObjects.placeKinds.unknown')
    expect(regionKindLabel('district_v2', t)).toBe('targetObjects.regionKinds.unknown')
    expect(collectionKindLabel('campaign_set', t)).toBe('targetCollections.kinds.unknown')
    expect(memberKindLabel('external_dataset', t)).toBe('targetCollectionDetail.memberKinds.unknown')
    expect(reportTypeLabel('new_report_code', t)).toBe('targetOverlays.reports.types.unknown')
    expect(reportStatusLabel('custom_status_code', t)).toBe('targetOverlays.reports.status.unknown')
    expect(observationTypeLabel('sensor_ping', t)).toBe('targetOverlays.observations.types.unknown')
  })
})
