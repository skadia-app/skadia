import { describe, expect, it } from 'vitest'

import { cyclesLabel, fieldLabel, fieldUnit, formatFieldValue, formatSpeedValue, swimStrokeLabel } from '../sportFieldLabels'

describe('sportFieldLabels i18n', () => {
  it('localizes representative sport-specific field labels without changing units or keys', () => {
    expect(fieldLabel('cadence', 'cycling')).toBe('Trittfrequenz')
    expect(fieldLabel('cadence', 'cycling', 'en')).toBe('Cadence')
    expect(fieldLabel('speed', 'running', 'en')).toBe('Pace')
    expect(fieldLabel('power', 'running', 'en')).toBe('Running power')
    expect(cyclesLabel('swimming', 'en')).toBe('Strokes')
  })

  it('localizes fixed labels and swim strokes', () => {
    expect(fieldLabel('temperature', 'cycling', 'en')).toBe('Temperature')
    expect(fieldLabel('torqueEffectivenessL', 'cycling', 'fr')).toBe('Efficacité du couple gauche')
    expect(swimStrokeLabel('breaststroke', 'it')).toBe('Rana')
    expect(swimStrokeLabel('unknown_stroke', 'es')).toBe('unknown_stroke')
  })

  it('formats user-facing speed and pace with optional user units', () => {
    const imperial = { units: 'imperial' as const, locale: 'en-US' }

    expect(formatSpeedValue(10, 'cycling', 'speed', imperial)).toBe('22.4 mph')
    expect(formatSpeedValue(1609.344 / 600, 'running', 'pace', imperial)).toBe('10:00 min/mi')
    expect(fieldUnit('speed', 'cycling', imperial)).toBe('mph')
    expect(fieldUnit('speed', 'running', imperial)).toBe('min/mi')
  })

  it('formats altitude, step length, travel range, and depth with optional user units', () => {
    const imperial = { units: 'imperial' as const, locale: 'en-US' }

    expect(fieldUnit('altitude', 'cycling', imperial)).toBe('ft')
    expect(fieldUnit('stepLength', 'running', imperial)).toBe('ft')
    expect(fieldUnit('travelRange', 'cycling', imperial)).toBe('mi')
    expect(fieldUnit('depth', 'diving', imperial)).toBe('ft')

    expect(formatFieldValue('altitude', 100, 'cycling', imperial)).toBe('328 ft')
    expect(formatFieldValue('stepLength', 1250, 'running', imperial)).toBe('4.1 ft')
    expect(formatFieldValue('travelRange', 16.09344, 'cycling', imperial)).toBe('10.0 mi')
    expect(formatFieldValue('depth', 12.3, 'diving', imperial)).toBe('40.4 ft')
  })

  it('keeps fixed training and sensor units unchanged when user units are supplied', () => {
    const imperial = { units: 'imperial' as const, locale: 'en-US' }

    expect(formatFieldValue('power', 250, 'cycling', imperial)).toBe('250 W')
    expect(formatFieldValue('heartRate', 151, 'running', imperial)).toBe('151 bpm')
    expect(formatFieldValue('batteryLevel', 98.4, 'cycling', imperial)).toBe('98.4 %')
  })

  it('formats activity temperatures with optional user units', () => {
    const imperial = { units: 'imperial' as const, locale: 'en-US' }

    expect(fieldUnit('temperature', 'cycling', imperial)).toBe('°F')
    expect(fieldUnit('coreTemperature', 'running', imperial)).toBe('°F')
    expect(formatFieldValue('temperature', 20, 'cycling', imperial)).toBe('68 °F')
    expect(formatFieldValue('coreTemperature', 37, 'running', imperial)).toBe('99 °F')
  })
})
