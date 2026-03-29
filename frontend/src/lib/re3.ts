// Looney, Hoogkamer, Kram (2025) — RE3 Graded Running Equation
// Reference: docs/research/2026-03-29-gap-re3-model-analysis.md

/**
 * RE3 metabolic rate (W/kg).
 * Source: Looney, Hoogkamer, Kram (2025), Eq. 4
 *
 * @param speed - Running speed in m/s (clamped to >= 1.0)
 * @param grade - Grade as decimal (0.10 = 10%; clamped to [-0.45, 0.82])
 */
export function re3MetabolicRate(speed: number, grade: number): number {
  grade = Math.max(-0.45, Math.min(0.82, grade))
  speed = Math.max(1.0, speed)
  const mLevel = 4.43 + 1.51 * speed + 0.37 * speed * speed
  const exp1 = Math.pow(1.056, 100.0 * grade + 43.0)
  const exp2 = Math.pow(1.133, 1.0 - exp1)
  const gradeTerm = 30.43 * speed * grade * (1.0 - exp2)
  return Math.max(1.0, mLevel + gradeTerm)
}

/**
 * Invert RE3 level equation: solve for speed given metabolic rate.
 * Uses the quadratic formula on the level-running part of RE3.
 *
 * @param mdot - Metabolic rate in W/kg
 */
export function re3InvertLevel(mdot: number): number {
  const a = 0.37, b = 1.51, c0 = 4.43
  const disc = b * b - 4 * a * (c0 - mdot)
  if (disc < 0) return 0
  const s = (-b + Math.sqrt(disc)) / (2 * a)
  return s > 0 ? s : 0
}

/**
 * Grade-adjusted speed (m/s) using exact quadratic inversion of RE3.
 * Returns the flat-equivalent speed that produces the same metabolic rate.
 *
 * @param speed - Actual running speed in m/s
 * @param grade - Grade as decimal (0.10 = 10%)
 */
export function re3GradeAdjustedSpeed(speed: number, grade: number): number {
  if (speed <= 0) return 0
  return re3InvertLevel(re3MetabolicRate(speed, grade))
}
