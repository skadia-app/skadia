export const PROTECTED_TERMS = [
  'Skadia',
  'Garmin',
  'Strava',
  'Komoot',
  'OSM',
  'BRouter',
  'MapLibre',
  'Mountainbike',
  'MTB',
  'Power Phase',
  'Body Battery',
  'TSS',
  'FTP',
  'PMC',
  'HRV',
  'GPX',
  'FIT',
  'CSV',
  'ZIP',
  'TCX',
] as const

export type ProtectedTerm = typeof PROTECTED_TERMS[number]
