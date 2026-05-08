import type { Activity } from '@/api/types'

export interface NetTotals {
  distanceM: number
  elevationGainM: number
  elevationLossM: number
}

/**
 * Returns activity totals with the lift portion subtracted, unless the
 * user has opted out via exclude_lift_subtraction. Lift portion is
 * direction-agnostic: an upward gondola contributes lift_ascent_m,
 * a downhill cable car contributes lift_descent_m, a bidirectional
 * aerial tramway both. Negative differences clamp to zero (defensive).
 */
export function getNetActivityTotals(activity: Activity): NetTotals {
  const exclude = activity.exclude_lift_subtraction ?? false
  const liftDist = activity.lift_distance_m ?? 0
  const liftAsc = activity.lift_ascent_m ?? 0
  const liftDesc = activity.lift_descent_m ?? 0
  const distanceM = (activity.distance_m ?? 0) - (exclude ? 0 : liftDist)
  const elevationGainM = (activity.elevation_gain_m ?? 0) - (exclude ? 0 : liftAsc)
  const elevationLossM = (activity.elevation_loss_m ?? 0) - (exclude ? 0 : liftDesc)
  return {
    distanceM: Math.max(0, distanceM),
    elevationGainM: Math.max(0, elevationGainM),
    elevationLossM: Math.max(0, elevationLossM),
  }
}
