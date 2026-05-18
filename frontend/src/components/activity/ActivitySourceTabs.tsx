import { Link } from 'react-router-dom'
import type { RecordingGroup } from '@/api/types'
import { getSourceColor } from '@/lib/comparisonColors'
import { deviceLabel } from '@/lib/deviceUtils'

interface ActivitySourceTabsProps {
  recordingGroup?: RecordingGroup | null
  activeActivityId?: string
  onActiveActivityChange?: (activityId: string) => void
}

export default function ActivitySourceTabs({ recordingGroup, activeActivityId, onActiveActivityChange }: ActivitySourceTabsProps) {
  if (!recordingGroup || recordingGroup.members.length <= 1) return null

  return (
    <div className="flex min-w-0 items-center gap-4 overflow-x-auto border-b border-neutral-200 pb-px dark:border-neutral-800">
      {recordingGroup.members.map((member, i) => {
        const active = member.activity_id === activeActivityId
        const label = deviceLabel(member.recording_device_name ?? member.source_device, member.source, member.original_file_format) ?? `Quelle ${i + 1}`
        return (
          <button
            key={member.activity_id}
            type="button"
            onClick={() => onActiveActivityChange?.(member.activity_id)}
            className={`shrink-0 border-b-2 px-1 py-2 text-sm transition-colors ${
              active
                ? 'border-orange-500 text-orange-600 dark:text-orange-400 font-medium'
                : 'border-transparent text-neutral-500 dark:text-neutral-400 hover:text-neutral-800 dark:hover:text-neutral-200'
            }`}
          >
            <span className="mr-2 inline-block h-2 w-2 rounded-full align-middle" style={{ background: getSourceColor(i) }} />
            {label}
          </button>
        )
      })}
      <Link
        to={`/activities/group/${recordingGroup.id}`}
        className="ml-auto shrink-0 text-xs text-orange-600 dark:text-orange-400 hover:underline"
      >
        Vergleichsansicht →
      </Link>
    </div>
  )
}
