import { useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ModalShell } from '@/components/ui/ModalShell'
import { useToast } from '@/lib/toast'
import { fetchRecordingGroup, addGroupMember } from '@/api/recordingGroups'
import { ApiError } from '@/api/client'

interface FusionGroupContextDialogProps {
  /** ID of activity A */
  activityIdA: string
  /** Title of activity A (for display) */
  titleA: string
  /** ID of activity B */
  activityIdB: string
  /** Title of activity B (for display) */
  titleB: string
  /** Recording group ID for activity A, null if not in a group */
  groupIdA: string | null
  /** Recording group ID for activity B, null if not in a group */
  groupIdB: string | null
  onClose: () => void
}

/**
 * Shown before navigating to the fusion page when at least one of the two
 * Tier-2 GPS-similarity candidates is already a member of a recording group.
 *
 * Two main cases:
 *  - Single group: one activity in a group, the other is external.
 *    Offers pair-only OR group-mode fusion with the external activity.
 *  - Cross-group: both activities in different groups.
 *    Only pair-only fusion is offered, with an explanatory info text.
 */
export default function FusionGroupContextDialog({
  activityIdA,
  titleA,
  activityIdB,
  titleB,
  groupIdA,
  groupIdB,
  onClose,
}: FusionGroupContextDialogProps) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { t } = useTranslation()
  const { addToast } = useToast()
  const [isAdding, setIsAdding] = useState(false)

  // Normalize API strings: treat empty string as null (defense against
  // backend JSON-serialization drift where pgx *string may produce "").
  // All logic below uses these normalized values. (R4-11)
  const groupIdANormalized = groupIdA || null
  const groupIdBNormalized = groupIdB || null

  // Determine case
  const isCrossGroup = groupIdANormalized !== null && groupIdBNormalized !== null && groupIdANormalized !== groupIdBNormalized

  // For single-group case: which group is it, and which activity is external?
  const groupId = isCrossGroup ? null : (groupIdANormalized ?? groupIdBNormalized)
  // Guard with isCrossGroup so these variables are only used in the single-group
  // rendering branch. Without the guard, when both groupIdANormalized and
  // groupIdBNormalized are non-null the ternary silently picks "A is in group,
  // B is external" — semantically wrong even though the cross-group branch
  // doesn't render them.
  const inGroupActivityTitle = !isCrossGroup ? (groupIdANormalized ? titleA : titleB) : null
  const externalActivityId = !isCrossGroup ? (groupIdANormalized ? activityIdB : activityIdA) : null

  // Fetch group member count (only in single-group case)
  const { data: groupData } = useQuery({
    queryKey: ['recording-group', groupId],
    queryFn: () => fetchRecordingGroup(groupId!),
    enabled: !isCrossGroup && groupId !== null,
  })

  const otherMemberCount = groupData ? groupData.members.length - 1 : null

  // Pair-only mode: navigate without changing group membership.
  function handlePairOnly() {
    navigate(`/fusion?ids=${activityIdA},${activityIdB}`)
  }

  // Group mode: add the external activity, then navigate to group fusion.
  async function handleGroupMode() {
    if (!groupId || !externalActivityId) return
    setIsAdding(true)
    try {
      await addGroupMember(groupId, externalActivityId)
      // Invalidate before navigating so that if navigation fails (back-button,
      // route guard) the stale pair entry is already gone from the queue and
      // the new group membership is visible without a manual F5.
      await queryClient.invalidateQueries({ queryKey: ['recording-groups'] })
      await queryClient.invalidateQueries({ queryKey: ['fusion-queue'] })
      navigate(`/fusion?group=${groupId}`)
    } catch (err) {
      const status = err instanceof ApiError ? err.status : 0
      if (status === 409) {
        addToast({
          type: 'error',
          title: t('fusionGroupContext.toasts.alreadyInOtherGroupTitle'),
          message: t('fusionGroupContext.toasts.alreadyInOtherGroupMessage'),
          duration: 0,
        })
      } else {
        addToast({
          type: 'error',
          title: t('fusionGroupContext.toasts.extendGroupFailedTitle'),
          message: err instanceof Error ? err.message : t('common.unknownError'),
          duration: 0,
        })
      }
    } finally {
      setIsAdding(false)
    }
  }

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-md">
      <div className="space-y-4">
        {/* Title */}
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          {t('fusionGroupContext.title')}
        </h2>

        {isCrossGroup ? (
          /* ── Cross-group case ─────────────────────────────────────────────── */
          <>
            <div className="rounded-lg bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-800 px-4 py-3">
              <p className="text-sm text-amber-800 dark:text-amber-200">
                <Trans i18nKey="fusionGroupContext.crossGroupWarning" components={{ em: <em /> }} />
              </p>
            </div>
            <div className="text-sm text-neutral-600 dark:text-neutral-400 space-y-1">
              <p>
                <span className="font-medium text-neutral-800 dark:text-neutral-200">
                  {titleA || t('common.untitled')}
                </span>
                {' '}
                {t('fusionGroupContext.ownGroupArrow')}
              </p>
              <p>
                <span className="font-medium text-neutral-800 dark:text-neutral-200">
                  {titleB || t('common.untitled')}
                </span>
                {' '}
                {t('fusionGroupContext.ownGroupArrow')}
              </p>
            </div>
          </>
        ) : (
          /* ── Single-group case ────────────────────────────────────────────── */
          <>
            <p className="text-sm text-neutral-600 dark:text-neutral-400">
              <span className="font-medium text-neutral-800 dark:text-neutral-200">
                {inGroupActivityTitle || t('common.untitled')}
              </span>{' '}
              {t('fusionGroupContext.isPartOfGroup')}
              {otherMemberCount !== null && otherMemberCount > 0 && (
                <> {t('fusionGroupContext.withOtherRecordings', { count: otherMemberCount })}</>
              )}
              {t('fusionGroupContext.fusionQuestionSuffix')}
            </p>
          </>
        )}

        {/* Action buttons */}
        <div className="flex items-center gap-2 pt-1">
          {/* Cancel — left side */}
          <button
            onClick={onClose}
            disabled={isAdding}
            className="cursor-pointer mr-auto px-3 py-2 text-sm border border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 rounded-lg hover:bg-neutral-50 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {t('common.cancel')}
          </button>

          {/* Pair-only (always shown) */}
          <button
            onClick={handlePairOnly}
            disabled={isAdding}
            className="cursor-pointer px-3 py-2 text-sm border border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 rounded-lg hover:bg-neutral-50 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {t('fusionGroupContext.pairOnly')}
          </button>

          {/* Group-mode (only in single-group case) */}
          {!isCrossGroup && groupId && (
            <button
              onClick={handleGroupMode}
              disabled={isAdding}
              className="cursor-pointer flex items-center gap-2 px-3 py-2 text-sm font-medium bg-orange-500 hover:bg-orange-600 text-white rounded-lg disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {isAdding && (
                <svg className="h-3.5 w-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
                </svg>
              )}
              {t('fusionGroupContext.wholeGroupPlus', {
                activity: (groupIdANormalized ? titleB : titleA) || t('fusionGroupContext.otherActivity'),
              })}
            </button>
          )}
        </div>
      </div>
    </ModalShell>
  )
}
