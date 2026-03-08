import { apiClient } from './client'
import type { Gear } from './types'

export interface PendingReviewGear extends Gear {
  review_status: 'pending_review' | 'confirmed' | 'rejected' | 'none'
  source_document_id?: string
  purchase_price?: number | null
  currency?: string | null
  weight_g?: number | null
}

export async function fetchPendingReviewGear(): Promise<PendingReviewGear[]> {
  return apiClient<PendingReviewGear[]>('/gear/pending-review')
}

export async function confirmGear(gearId: string): Promise<void> {
  await apiClient('/gear/' + gearId + '/review', {
    method: 'PATCH',
    body: { review_status: 'confirmed' },
  })
}

export async function rejectGear(gearId: string): Promise<void> {
  await apiClient('/gear/' + gearId + '/review', {
    method: 'PATCH',
    body: { review_status: 'rejected' },
  })
}

export async function updateAndConfirmGear(gearId: string, data: Record<string, unknown>): Promise<void> {
  await apiClient('/gear/' + gearId, {
    method: 'PUT',
    body: data,
  })
  await confirmGear(gearId)
}
