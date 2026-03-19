function luminance(hex: string): number {
  const toLinear = (c: number) => {
    const s = c / 255
    return s <= 0.04045 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4)
  }
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  if (isNaN(r) || isNaN(g) || isNaN(b)) return 1
  return 0.2126 * toLinear(r) + 0.7152 * toLinear(g) + 0.0722 * toLinear(b)
}

/**
 * Returns '#000000' or '#ffffff' for a tag background color.
 * Threshold 0.3 (above WCAG crossover ~0.179) so that saturated mid-tones
 * like vivid green or red get white text, which looks better visually.
 */
export function getTagTextColor(hex: string): '#000000' | '#ffffff' {
  return luminance(hex) > 0.3 ? '#000000' : '#ffffff'
}

/**
 * Full style object for a tag badge: background, text color, and a subtle
 * inset border that makes very light tags visible against white backgrounds.
 */
export function getTagBadgeStyle(hex: string): { backgroundColor: string; color: string; boxShadow: string } {
  return {
    backgroundColor: hex,
    color: getTagTextColor(hex),
    boxShadow: 'inset 0 0 0 1px rgba(0,0,0,0.12)',
  }
}
