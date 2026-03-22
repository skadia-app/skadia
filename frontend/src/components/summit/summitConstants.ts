export type ElevClass = { label: string; min: number | null; max: number | null }

export const ELEV_CLASSES: ElevClass[] = [
  { label: 'Alle', min: null, max: null },
  { label: '1000er', min: 1000, max: 1999 },
  { label: '2000er', min: 2000, max: 2999 },
  { label: '3000er', min: 3000, max: 3999 },
  { label: '4000er+', min: 4000, max: null },
]
