import { useParams, Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft } from 'lucide-react'
import { SegmentMinimap } from '@/components/SegmentDetail/SegmentMinimap'
import { SegmentActivityList } from '@/components/SegmentDetail/SegmentActivityList'
import { SegmentPerformanceChart } from '@/components/SegmentDetail/SegmentPerformanceChart'
import { FavoriteButton } from '@/components/SegmentDetail/FavoriteButton'

export default function SegmentDetailPage() {
  const { id } = useParams<{ id: string }>()
  const segmentId = parseInt(id!)

  const { data: segment, isLoading } = useQuery({
    queryKey: ['segment', segmentId],
    queryFn: () => fetch(`/api/v1/segments/${segmentId}`).then(r => r.json()),
  })

  if (isLoading) return <div className="p-6">Lädt...</div>
  if (!segment) return <div className="p-6">Segment nicht gefunden</div>

  return (
    <div className="flex flex-col gap-6 p-6">
      <div className="flex items-center justify-between">
        <Link to="/segments" className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground">
          <ArrowLeft size={16} /> Zurück zu Segmenten
        </Link>
        <FavoriteButton segmentId={segmentId} />
      </div>
      <h1 className="text-3xl font-bold">{segment.name}</h1>
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <SegmentMinimap segment={segment} />
        <SegmentActivityList segmentId={segmentId} />
      </div>
      <SegmentPerformanceChart segmentId={segmentId} />
    </div>
  )
}
