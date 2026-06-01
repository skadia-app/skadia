import { describe, expect, it } from 'vitest'
import { formatCalendarDayHeading } from './CalendarPage'

describe('CalendarPage date formatting', () => {
  it('formats day headings with the supplied locale order', () => {
    const date = new Date('2026-05-31T12:00:00Z')

    expect(formatCalendarDayHeading(date, 'en-US')).toBe('Sunday, May 31')
    expect(formatCalendarDayHeading(date, 'de-DE')).toBe('Sonntag, 31. Mai')
  })
})
