export type ElevClassId = 'all' | 'elevation_1000' | 'elevation_2000' | 'elevation_3000' | 'elevation_4000_plus'
export type ElevClass = { id: ElevClassId; labelKey: string; min: number | null; max: number | null }

export const ELEV_CLASSES: ElevClass[] = [
  { id: 'all', labelKey: 'summitCollection.elevationClasses.Alle', min: null, max: null },
  { id: 'elevation_1000', labelKey: 'summitCollection.elevationClasses.1000er', min: 1000, max: 1999 },
  { id: 'elevation_2000', labelKey: 'summitCollection.elevationClasses.2000er', min: 2000, max: 2999 },
  { id: 'elevation_3000', labelKey: 'summitCollection.elevationClasses.3000er', min: 3000, max: 3999 },
  { id: 'elevation_4000_plus', labelKey: 'summitCollection.elevationClasses.4000er+', min: 4000, max: null },
]

export function findElevationClass(value: string | null): ElevClass {
  if (!value) return ELEV_CLASSES[0]
  return ELEV_CLASSES.find(cls => cls.id === value || cls.labelKey.endsWith(`.${value}`)) ?? ELEV_CLASSES[0]
}
