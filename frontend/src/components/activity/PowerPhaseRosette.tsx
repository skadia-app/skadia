import { useRef, useEffect } from 'react'
import type { Activity } from '@/api/types'

interface Props {
  activity: Activity
  size?: number
}

const COLORS = {
  leftFull: '#34d399',   // emerald-400
  leftPeak: '#10b981',   // emerald-500
  rightFull: '#60a5fa',  // blue-400
  rightPeak: '#3b82f6',  // blue-500
  ring: '#404040',       // neutral-700
  ringLight: '#e5e5e5',  // neutral-200
  text: '#a3a3a3',       // neutral-400
  textDark: '#737373',   // neutral-500
  label: '#d4d4d4',      // neutral-300
  labelDark: '#525252',  // neutral-600
}

/** Convert cycling degrees (0°=TDC, CW) to canvas radians (0=3 o'clock, CW) */
function toRad(deg: number): number {
  return ((deg - 90) * Math.PI) / 180
}

function drawArc(
  ctx: CanvasRenderingContext2D,
  cx: number, cy: number,
  innerR: number, outerR: number,
  startDeg: number, endDeg: number,
  color: string, alpha = 1.0,
) {
  let startRad = toRad(startDeg)
  let endRad = toRad(endDeg)
  if (endRad < startRad) endRad += 2 * Math.PI

  ctx.beginPath()
  ctx.arc(cx, cy, outerR, startRad, endRad)
  ctx.arc(cx, cy, innerR, endRad, startRad, true)
  ctx.closePath()
  ctx.fillStyle = color
  ctx.globalAlpha = alpha
  ctx.fill()
  ctx.globalAlpha = 1.0
}

function drawSingleWheel(
  ctx: CanvasRenderingContext2D,
  cx: number, cy: number, maxR: number,
  start: number, end: number,
  peakStart: number | null | undefined, peakEnd: number | null | undefined,
  fullColor: string, peakColor: string,
  ringColor: string, textColor: string, labelColor: string,
  label: string, balance: string | null,
) {
  const outerR = maxR * 0.92
  const innerR = maxR * 0.62
  const ringWidth = outerR - innerR

  ctx.beginPath()
  ctx.arc(cx, cy, outerR, 0, 2 * Math.PI)
  ctx.arc(cx, cy, innerR, 2 * Math.PI, 0, true)
  ctx.closePath()
  ctx.fillStyle = ringColor
  ctx.fill()

  drawArc(ctx, cx, cy, innerR, outerR, start, end, fullColor, 0.85)

  if (peakStart != null && peakEnd != null) {
    const peakInset = ringWidth * 0.1
    drawArc(ctx, cx, cy, innerR + peakInset, outerR - peakInset, peakStart, peakEnd, peakColor, 1.0)
  }

  ctx.fillStyle = labelColor
  for (const deg of [0, 90, 180, 270]) {
    const r = toRad(deg)
    ctx.beginPath()
    ctx.arc(cx + Math.cos(r) * (innerR * 0.85), cy + Math.sin(r) * (innerR * 0.85), 2, 0, 2 * Math.PI)
    ctx.fill()
  }

  ctx.fillStyle = labelColor
  ctx.font = '10px system-ui, sans-serif'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillText('TDC', cx, cy - innerR * 0.7)
  ctx.fillText('BDC', cx, cy + innerR * 0.7)

  ctx.fillStyle = textColor
  ctx.font = 'bold 13px system-ui, sans-serif'
  ctx.fillText(label, cx, balance ? cy - 8 : cy)

  if (balance) {
    ctx.font = '12px system-ui, sans-serif'
    ctx.fillText(balance, cx, cy + 10)
  }
}

export function PowerPhaseRosette({ activity, size = 160 }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null)

  const hasLeft = activity.avg_left_power_phase_start_deg != null && activity.avg_left_power_phase_end_deg != null
  const hasRight = activity.avg_right_power_phase_start_deg != null && activity.avg_right_power_phase_end_deg != null

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas || (!hasLeft && !hasRight)) return

    const draw = () => {
      const isDark = document.documentElement.classList.contains('dark')
      const ringColor = isDark ? COLORS.ring : COLORS.ringLight
      const textColor = isDark ? COLORS.text : COLORS.textDark
      const labelColor = isDark ? COLORS.label : COLORS.labelDark

      const cols = (hasLeft ? 1 : 0) + (hasRight ? 1 : 0)
      const gap = 24
      const totalW = cols * size + (cols > 1 ? gap : 0)
      const totalH = size + 60

      const dpr = window.devicePixelRatio || 1
      canvas.width = totalW * dpr
      canvas.height = totalH * dpr
      canvas.style.width = `${totalW}px`
      canvas.style.height = `${totalH}px`

      const ctx = canvas.getContext('2d')
      if (!ctx) return
      ctx.scale(dpr, dpr)
      ctx.clearRect(0, 0, totalW, totalH)

      const maxR = size / 2 - 4
      const balance = activity.left_right_balance
      let colIdx = 0

      if (hasLeft) {
        const cx = size / 2
        const cy = size / 2
        const balStr = balance != null ? `${balance.toFixed(0)}%` : null
        drawSingleWheel(ctx, cx, cy, maxR,
          activity.avg_left_power_phase_start_deg!, activity.avg_left_power_phase_end_deg!,
          activity.avg_left_power_phase_peak_start_deg, activity.avg_left_power_phase_peak_end_deg,
          COLORS.leftFull, COLORS.leftPeak, ringColor, textColor, labelColor,
          'LINKS', balStr,
        )
        ctx.fillStyle = textColor
        ctx.font = '10px system-ui, sans-serif'
        ctx.textAlign = 'center'
        ctx.fillText(
          `PP ${activity.avg_left_power_phase_start_deg!.toFixed(0)}°–${activity.avg_left_power_phase_end_deg!.toFixed(0)}°`,
          cx, size + 14,
        )
        if (
          activity.avg_left_power_phase_peak_start_deg != null &&
          activity.avg_left_power_phase_peak_end_deg != null
        ) {
          ctx.fillText(
            `Peak ${activity.avg_left_power_phase_peak_start_deg.toFixed(0)}°–${activity.avg_left_power_phase_peak_end_deg.toFixed(0)}°`,
            cx, size + 30,
          )
        }
        colIdx++
      }

      if (hasRight) {
        const cx = colIdx * (size + gap) + size / 2
        const cy = size / 2
        const balStr = balance != null ? `${(100 - balance).toFixed(0)}%` : null
        drawSingleWheel(ctx, cx, cy, maxR,
          activity.avg_right_power_phase_start_deg!, activity.avg_right_power_phase_end_deg!,
          activity.avg_right_power_phase_peak_start_deg, activity.avg_right_power_phase_peak_end_deg,
          COLORS.rightFull, COLORS.rightPeak, ringColor, textColor, labelColor,
          'RECHTS', balStr,
        )
        ctx.fillStyle = textColor
        ctx.font = '10px system-ui, sans-serif'
        ctx.textAlign = 'center'
        const rcx = colIdx * (size + gap) + size / 2
        ctx.fillText(
          `PP ${activity.avg_right_power_phase_start_deg!.toFixed(0)}°–${activity.avg_right_power_phase_end_deg!.toFixed(0)}°`,
          rcx, size + 14,
        )
        if (
          activity.avg_right_power_phase_peak_start_deg != null &&
          activity.avg_right_power_phase_peak_end_deg != null
        ) {
          ctx.fillText(
            `Peak ${activity.avg_right_power_phase_peak_start_deg.toFixed(0)}°–${activity.avg_right_power_phase_peak_end_deg.toFixed(0)}°`,
            rcx, size + 30,
          )
        }
      }
    }

    draw()

    const observer = new MutationObserver(() => draw())
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    })

    return () => observer.disconnect()
  }, [activity, size, hasLeft, hasRight])

  if (!hasLeft && !hasRight) return null

  return (
    <div className="flex flex-col items-center gap-2">
      <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
        Power Phase
      </h3>
      <canvas ref={canvasRef} />
      <div className="flex gap-4 text-xs text-neutral-500">
        {hasLeft && (
          <div className="flex items-center gap-1.5">
            <span className="inline-block w-3 h-3 rounded-sm" style={{ background: COLORS.leftPeak }} />
            <span>L Power Phase</span>
          </div>
        )}
        {hasRight && (
          <div className="flex items-center gap-1.5">
            <span className="inline-block w-3 h-3 rounded-sm" style={{ background: COLORS.rightPeak }} />
            <span>R Power Phase</span>
          </div>
        )}
      </div>
    </div>
  )
}
