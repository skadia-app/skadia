// Known developer field units for display annotation.
// Keys are English (i18n rule 4). Labels translated in i18n sprint.
export const KNOWN_DEVELOPER_FIELD_UNITS: Record<string, { unit: string; label: string }> = {
  aeroscore:          { unit: '',      label: 'Aeroscore' },
  body_angle:         { unit: '°',    label: 'Body Angle' },
  core_temperature:   { unit: '°C',   label: 'Core Temperature' },
  skin_temperature:   { unit: '°C',   label: 'Skin Temperature' },
  heat_strain_index:  { unit: '',      label: 'Heat Strain Index' },
  running_smoothness: { unit: '%',     label: 'Running Smoothness' },
  glucose:            { unit: 'mg/dL', label: 'Glucose' },
  radar_current:      { unit: '',      label: 'Vehicles' },
  radar_total:        { unit: '',      label: 'Vehicles Total' },
}

/** Returns the display unit for a developer field, preferring the local mapping
 *  over the backend-provided unit string. */
export function getDevFieldUnit(fieldKey: string, backendUnit?: string): string {
  return KNOWN_DEVELOPER_FIELD_UNITS[fieldKey]?.unit ?? backendUnit ?? ''
}

export function getDevFieldLabel(fieldKey: string, backendName?: string): string {
  return KNOWN_DEVELOPER_FIELD_UNITS[fieldKey]?.label ?? backendName ?? fieldKey
}
