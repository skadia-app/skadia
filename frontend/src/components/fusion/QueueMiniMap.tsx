import { useEffect, useRef } from 'react'

export interface MiniMapTrack {
  points: Array<{ lat: number; lng: number }>
  color: string
  label?: string
}

interface QueueMiniMapProps {
  tracks: MiniMapTrack[]
  height?: number
  className?: string
}

// Mercator projection: lng → x, lat → y (pixel space)
function mercatorX(lng: number): number {
  return lng
}

function mercatorY(lat: number): number {
  // Simple linear approximation is fine for small areas
  return -lat
}

function drawTracks(canvas: HTMLCanvasElement, tracks: MiniMapTrack[]) {
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const dpr = window.devicePixelRatio || 1
  const w = canvas.offsetWidth
  const h = canvas.offsetHeight
  if (w === 0 || h === 0) return

  canvas.width = w * dpr
  canvas.height = h * dpr
  ctx.scale(dpr, dpr)

  // Clear with light gray background
  ctx.fillStyle = '#f1f5f1'
  ctx.fillRect(0, 0, w, h)

  // Collect all points
  const allPoints = tracks.flatMap(t => t.points)
  if (allPoints.length === 0) return

  // Compute bounds in projected space
  const xs = allPoints.map(p => mercatorX(p.lng))
  const ys = allPoints.map(p => mercatorY(p.lat))
  const minX = Math.min(...xs)
  const maxX = Math.max(...xs)
  const minY = Math.min(...ys)
  const maxY = Math.max(...ys)

  const pad = 12
  const scaleW = maxX === minX ? 1 : (w - pad * 2) / (maxX - minX)
  const scaleH = maxY === minY ? 1 : (h - pad * 2) / (maxY - minY)
  const scale = Math.min(scaleW, scaleH)

  // Center the drawing
  const drawW = (maxX - minX) * scale
  const drawH = (maxY - minY) * scale
  const offsetX = (w - drawW) / 2
  const offsetY = (h - drawH) / 2

  function project(lat: number, lng: number): [number, number] {
    const px = (mercatorX(lng) - minX) * scale + offsetX
    const py = (mercatorY(lat) - minY) * scale + offsetY
    return [px, py]
  }

  // Draw each track
  tracks.forEach(track => {
    if (track.points.length < 2) return
    ctx.beginPath()
    ctx.strokeStyle = track.color
    ctx.lineWidth = 2
    ctx.lineCap = 'round'
    ctx.lineJoin = 'round'
    const [x0, y0] = project(track.points[0].lat, track.points[0].lng)
    ctx.moveTo(x0, y0)
    for (let i = 1; i < track.points.length; i++) {
      const [xi, yi] = project(track.points[i].lat, track.points[i].lng)
      ctx.lineTo(xi, yi)
    }
    ctx.stroke()
  })

  // Legend (bottom right)
  const tracksWithLabel = tracks.filter(t => t.label)
  if (tracksWithLabel.length > 0) {
    const legendPad = 6
    const dotR = 5
    const lineH = 16
    const fontSize = 10
    ctx.font = `${fontSize}px -apple-system, BlinkMacSystemFont, sans-serif`

    // Measure widths
    const items = tracksWithLabel.map(t => ({
      label: t.label!,
      color: t.color,
      width: ctx.measureText(t.label!).width + dotR * 2 + 6,
    }))

    // Lay out items in a row at bottom-right
    const totalW = items.reduce((acc, it) => acc + it.width + 8, 0) - 8
    const legendX = w - totalW - legendPad - 4
    const legendY = h - lineH - legendPad

    // Background
    ctx.fillStyle = 'rgba(255,255,255,0.82)'
    ctx.beginPath()
    ctx.roundRect(legendX - 4, legendY - 2, totalW + 8, lineH + 4, 4)
    ctx.fill()

    // Draw dots + labels
    let curX = legendX
    for (const item of items) {
      ctx.beginPath()
      ctx.arc(curX + dotR, legendY + lineH / 2 - 1, dotR, 0, Math.PI * 2)
      ctx.fillStyle = item.color
      ctx.fill()
      ctx.fillStyle = '#374151'
      ctx.fillText(item.label, curX + dotR * 2 + 4, legendY + lineH / 2 + fontSize / 2 - 1)
      curX += item.width + 8
    }
  }
}

export default function QueueMiniMap({ tracks, height = 150, className = '' }: QueueMiniMapProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)

  // Use ResizeObserver so the canvas draws correctly after layout is complete.
  // clientWidth/offsetWidth is 0 during the initial paint before the browser
  // has performed layout, so we must wait for the first resize notification.
  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return

    const observer = new ResizeObserver(() => {
      drawTracks(canvas, tracks)
    })
    observer.observe(canvas)

    // Also draw immediately in case the ResizeObserver fires synchronously only
    // on the first observation (Chrome behavior) — this is a no-op if width=0.
    drawTracks(canvas, tracks)

    return () => observer.disconnect()
  }, [tracks, height])

  return (
    <canvas
      ref={canvasRef}
      style={{ height, display: 'block', width: '100%' }}
      className={`rounded-lg ${className}`}
    />
  )
}
