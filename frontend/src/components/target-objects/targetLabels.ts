import type {
  TargetCollectionKind,
  TargetCollectionMemberKind,
  TargetPlaceKind,
  TargetRegionKind,
  TargetVisibility,
} from '@/api/targetObjects'
import type { TargetActivityObservation, TargetReportStatus, TargetReportType } from '@/api/targetOverlays'

type Translate = (key: string) => string

const TARGET_VISIBILITY_LABEL_KEYS: Record<TargetVisibility, string> = {
  private: 'targetObjects.visibility.private',
  shared: 'targetObjects.visibility.shared',
  public: 'targetObjects.visibility.public',
}

const PLACE_KIND_LABEL_KEYS: Record<TargetPlaceKind, string> = {
  custom: 'targetObjects.placeKinds.custom',
  private_place: 'targetObjects.placeKinds.private_place',
  poi: 'targetObjects.placeKinds.poi',
  entrance: 'targetObjects.placeKinds.entrance',
  parking: 'targetObjects.placeKinds.parking',
  summit: 'targetObjects.placeKinds.summit',
  pass: 'targetObjects.placeKinds.pass',
  water: 'targetObjects.placeKinds.water',
  settlement: 'targetObjects.placeKinds.settlement',
  amenity: 'targetObjects.placeKinds.amenity',
}

const REGION_KIND_LABEL_KEYS: Record<TargetRegionKind, string> = {
  custom: 'targetObjects.regionKinds.custom',
  private_area: 'targetObjects.regionKinds.private_area',
  public_pool: 'targetObjects.regionKinds.public_pool',
  venue_area: 'targetObjects.regionKinds.venue_area',
  privacy_zone_mirror: 'targetObjects.regionKinds.privacy_zone_mirror',
  admin_area: 'targetObjects.regionKinds.admin_area',
  protected_area: 'targetObjects.regionKinds.protected_area',
}

const COLLECTION_KIND_LABEL_KEYS: Record<TargetCollectionKind, string> = {
  private_map: 'targetCollections.kinds.private_map',
  public_pool: 'targetCollections.kinds.public_pool',
  venue_pool: 'targetCollections.kinds.venue_pool',
  route_set: 'targetCollections.kinds.route_set',
  course_set: 'targetCollections.kinds.course_set',
  rule_set: 'targetCollections.kinds.rule_set',
  privacy_set: 'targetCollections.kinds.privacy_set',
  custom: 'targetCollections.kinds.custom',
}

const MEMBER_KIND_LABEL_KEYS: Record<TargetCollectionMemberKind, string> = {
  course: 'targetCollectionDetail.memberKinds.course',
  route_publication: 'targetCollectionDetail.memberKinds.route_publication',
  place: 'targetCollectionDetail.memberKinds.place',
  region: 'targetCollectionDetail.memberKinds.region',
  collection: 'targetCollectionDetail.memberKinds.collection',
  privacy_zone: 'targetCollectionDetail.memberKinds.privacy_zone',
  tag_zone: 'targetCollectionDetail.memberKinds.tag_zone',
  external_ref: 'targetCollectionDetail.memberKinds.external_ref',
}

const REPORT_TYPE_LABEL_KEYS: Record<TargetReportType, string> = {
  condition: 'targetOverlays.reports.types.condition',
  issue: 'targetOverlays.reports.types.issue',
  warning: 'targetOverlays.reports.types.warning',
  trail_work: 'targetOverlays.reports.types.trail_work',
  closure: 'targetOverlays.reports.types.closure',
  metadata: 'targetOverlays.reports.types.metadata',
}

const REPORT_STATUS_LABEL_KEYS: Record<TargetReportStatus, string> = {
  open: 'targetOverlays.reports.status.open',
  resolved: 'targetOverlays.reports.status.resolved',
  rejected: 'targetOverlays.reports.status.rejected',
  superseded: 'targetOverlays.reports.status.superseded',
  informational: 'targetOverlays.reports.status.informational',
  redacted: 'targetOverlays.reports.status.redacted',
}

const OBSERVATION_TYPE_LABEL_KEYS: Record<TargetActivityObservation['observation_type'], string> = {
  matched_traversal: 'targetOverlays.observations.types.matched_traversal',
  visit: 'targetOverlays.observations.types.visit',
  dwell: 'targetOverlays.observations.types.dwell',
  check_in: 'targetOverlays.observations.types.check_in',
  completion: 'targetOverlays.observations.types.completion',
}

function labelForKnownValue<T extends string>(value: string, t: Translate, keys: Record<T, string>, unknownKey: string): string {
  const key = keys[value as T]
  return t(key ?? unknownKey)
}

export function targetVisibilityLabel(value: string, t: Translate): string {
  return labelForKnownValue(value, t, TARGET_VISIBILITY_LABEL_KEYS, 'targetObjects.visibility.unknown')
}

export function placeKindLabel(value: string, t: Translate): string {
  return labelForKnownValue(value, t, PLACE_KIND_LABEL_KEYS, 'targetObjects.placeKinds.unknown')
}

export function regionKindLabel(value: string, t: Translate): string {
  return labelForKnownValue(value, t, REGION_KIND_LABEL_KEYS, 'targetObjects.regionKinds.unknown')
}

export function collectionKindLabel(value: string, t: Translate): string {
  return labelForKnownValue(value, t, COLLECTION_KIND_LABEL_KEYS, 'targetCollections.kinds.unknown')
}

export function memberKindLabel(value: string, t: Translate): string {
  return labelForKnownValue(value, t, MEMBER_KIND_LABEL_KEYS, 'targetCollectionDetail.memberKinds.unknown')
}

export function reportTypeLabel(value: string, t: Translate): string {
  return labelForKnownValue(value, t, REPORT_TYPE_LABEL_KEYS, 'targetOverlays.reports.types.unknown')
}

export function reportStatusLabel(value: string, t: Translate): string {
  return labelForKnownValue(value, t, REPORT_STATUS_LABEL_KEYS, 'targetOverlays.reports.status.unknown')
}

export function observationTypeLabel(value: string, t: Translate): string {
  return labelForKnownValue(value, t, OBSERVATION_TYPE_LABEL_KEYS, 'targetOverlays.observations.types.unknown')
}
