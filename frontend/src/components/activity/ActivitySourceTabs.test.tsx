import '@testing-library/jest-dom/vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import type { RecordingGroup, RecordingGroupMember } from '@/api/types'
import ActivitySourceTabs from './ActivitySourceTabs'

function member(index: number): RecordingGroupMember {
  return {
    activity_id: `activity-${index}`,
    is_fused: false,
    fusion_source_count: 0,
    is_primary: index === 1,
    title: null,
    title_auto: null,
    started_at: '2026-05-18T08:00:00Z',
    sport_type: 'cycling',
    sub_type: null,
    is_electric: false,
    is_indoor: false,
    is_virtual: false,
    distance_m: 1000,
    elapsed_time_s: 600,
    field_richness: 0,
    source_device: `Device ${index}`,
    source: 'upload',
    original_file_format: 'fit',
    has_gps: true,
    elevation_gain_m: 10,
    elevation_loss_m: 10,
  }
}

function group(count: number): RecordingGroup {
  return {
    id: 'group-1',
    user_id: 'user-1',
    primary_activity_id: 'activity-1',
    sport_type: 'cycling',
    sub_type: null,
    is_electric: false,
    is_indoor: false,
    is_virtual: false,
    created_at: '2026-05-18T08:00:00Z',
    confirmed: false,
    members: Array.from({ length: count }, (_, i) => member(i + 1)),
  }
}

describe('ActivitySourceTabs', () => {
  it('keeps all grouped recordings directly selectable', () => {
    const onActiveActivityChange = vi.fn()

    render(
      <MemoryRouter>
        <ActivitySourceTabs
          recordingGroup={group(6)}
          activeActivityId="activity-1"
          onActiveActivityChange={onActiveActivityChange}
        />
      </MemoryRouter>,
    )

    for (let i = 1; i <= 6; i++) {
      expect(screen.getByRole('button', { name: `Device ${i} · FIT` })).toBeInTheDocument()
    }
    expect(screen.queryByText('+2 Quellen')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Device 6 · FIT' }))
    expect(onActiveActivityChange).toHaveBeenCalledWith('activity-6')
  })
})
