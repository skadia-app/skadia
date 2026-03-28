import { useMemo, useState } from 'react'
import type { StreamPoint } from '@/api/types'
import { sportColor } from '@/lib/sportTaxonomy'

interface Props {
  streams: StreamPoint[]
  sportType: string
  hoverIndex: number | null
  visible: boolean
  onClickScrollToMap: () => void
}

export function PiPMap({ streams, sportType, hoverIndex, visible, onClickScrollToMap }: Props) {
  const [dismissed, setDismissed] = useState(false)

  const coords = useMemo(() => {
    const pts: { lat: number; lng: number }[] = []
    for (const s of streams) {
      if (s.lat != null && s.lng != null) pts.push({ lat: s.lat, lng: s.lng })
    }
    // Downsample to max ~800 points for SVG performance
    if (pts.length <= 800) return pts
    const step = pts.length / 800
    const result: { lat: number; lng: number }[] = []
    for (let i = 0; i < pts.length; i = Math.round(i + step)) {
      result.push(pts[i])
    }
    if (result[result.length - 1] !== pts[pts.length - 1]) result.push(pts[pts.length - 1])
    return result
  }, [streams])

  if (dismissed || !visible || coords.length < 2) return null

  let minLat = Infinity, maxLat = -Infinity, minLng = Infinity, maxLng = -Infinity
  for (const c of coords) {
    if (c.lat < minLat) minLat = c.lat
    if (c.lat > maxLat) maxLat = c.lat
    if (c.lng < minLng) minLng = c.lng
    if (c.lng > maxLng) maxLng = c.lng
  }

  const padding = 10
  const width = 300
  const height = 220
  const innerW = width - padding * 2
  const innerH = height - padding * 2

  const latRange = maxLat - minLat || 0.001
  const lngRange = maxLng - minLng || 0.001
  const cosLat = Math.cos(((minLat + maxLat) / 2) * Math.PI / 180)
  const adjustedLngRange = lngRange * cosLat

  const scaleX = innerW / (adjustedLngRange || 0.001)
  const scaleY = innerH / latRange
  const scale = Math.min(scaleX, scaleY)
  const offX = padding + (innerW - adjustedLngRange * scale) / 2
  const offY = padding + (innerH - latRange * scale) / 2

  const toX = (lng: number) => offX + (lng - minLng) * cosLat * scale
  const toY = (lat: number) => offY + (maxLat - lat) * scale

  const polyline = coords.map(c => `${toX(c.lng)},${toY(c.lat)}`).join(' ')

  const hoverPt = hoverIndex != null && streams[hoverIndex]?.lat != null
    ? { x: toX(streams[hoverIndex].lng!), y: toY(streams[hoverIndex].lat!) }
    : null

  const color = sportColor(sportType)

  return (
    <div
      className="fixed bottom-5 right-5 z-[100] rounded-xl shadow-2xl border border-neutral-300 dark:border-neutral-700 overflow-hidden cursor-pointer transition-all duration-300 w-[200px] h-[150px] sm:w-[300px] sm:h-[220px]"
      style={{ opacity: visible ? 1 : 0, transform: visible ? 'scale(1)' : 'scale(0.9)' }}
      onClick={onClickScrollToMap}
      role="button"
      aria-label="Kartenvorschau — klicken zum Scrollen zur Karte"
      aria-hidden={!visible}
    >
      <button
        onClick={e => { e.stopPropagation(); setDismissed(true) }}
        className="absolute top-1.5 right-1.5 z-10 w-5 h-5 rounded-full bg-neutral-800/60 text-white text-xs flex items-center justify-center hover:bg-neutral-800/80 cursor-pointer"
        aria-label="Kartenvorschau schließen"
      >
        ×
      </button>
      <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-full bg-neutral-800 dark:bg-neutral-900">
        <polyline
          points={polyline}
          fill="none"
          stroke={color}
          strokeWidth="2.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        {hoverPt && (
          <circle cx={hoverPt.x} cy={hoverPt.y} r="4" fill="#fff" stroke={color} strokeWidth="2" />
        )}
      </svg>
    </div>
  )
}
