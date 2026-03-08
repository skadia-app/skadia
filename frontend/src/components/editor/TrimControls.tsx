interface TrimControlsProps {
  totalPoints: number
  trimStart: number
  trimEnd: number
  onTrimStartChange: (idx: number) => void
  onTrimEndChange: (idx: number) => void
  onSetFromMap: (which: 'start' | 'end') => void
  activeMapSelect: 'start' | 'end' | null
}

export function TrimControls({
  totalPoints, trimStart, trimEnd,
  onTrimStartChange, onTrimEndChange,
  onSetFromMap, activeMapSelect,
}: TrimControlsProps) {
  const trimmedCount = trimEnd - trimStart + 1
  const trimmedPct = totalPoints > 0
    ? ((trimmedCount / totalPoints) * 100).toFixed(1)
    : '0.0'

  const startPct = totalPoints > 0
    ? ((trimStart / (totalPoints - 1)) * 100).toFixed(0)
    : '0'
  const endPct = totalPoints > 0
    ? ((trimEnd / (totalPoints - 1)) * 100).toFixed(0)
    : '100'

  return (
    <div className="space-y-3 p-4 border border-neutral-200 dark:border-neutral-700 rounded-lg bg-neutral-50 dark:bg-neutral-900">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
          Trim / Crop
        </h3>
        {(trimStart > 0 || trimEnd < totalPoints - 1) && (
          <span className="text-xs text-amber-600 dark:text-amber-400 font-medium">
            Anfang/Ende abgeschnitten
          </span>
        )}
      </div>

      {/* Start-Slider */}
      <div className="space-y-1">
        <div className="flex items-center justify-between">
          <label className="text-xs text-neutral-500 dark:text-neutral-400">
            Start ({startPct}%)
          </label>
          <button
            onClick={() => onSetFromMap('start')}
            className={`text-xs px-2 py-0.5 rounded border transition-colors ${
              activeMapSelect === 'start'
                ? 'bg-blue-100 border-blue-400 text-blue-700 dark:bg-blue-900/40 dark:border-blue-500 dark:text-blue-300'
                : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'
            }`}
          >
            {activeMapSelect === 'start' ? 'Auf Karte klicken...' : 'Auf Karte setzen'}
          </button>
        </div>
        <input
          type="range"
          min={0}
          max={totalPoints - 1}
          value={trimStart}
          onChange={e => {
            const v = Number(e.target.value)
            if (v <= trimEnd) onTrimStartChange(v)
          }}
          className="w-full accent-green-500"
        />
      </div>

      {/* End-Slider */}
      <div className="space-y-1">
        <div className="flex items-center justify-between">
          <label className="text-xs text-neutral-500 dark:text-neutral-400">
            Ende ({endPct}%)
          </label>
          <button
            onClick={() => onSetFromMap('end')}
            className={`text-xs px-2 py-0.5 rounded border transition-colors ${
              activeMapSelect === 'end'
                ? 'bg-blue-100 border-blue-400 text-blue-700 dark:bg-blue-900/40 dark:border-blue-500 dark:text-blue-300'
                : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'
            }`}
          >
            {activeMapSelect === 'end' ? 'Auf Karte klicken...' : 'Auf Karte setzen'}
          </button>
        </div>
        <input
          type="range"
          min={0}
          max={totalPoints - 1}
          value={trimEnd}
          onChange={e => {
            const v = Number(e.target.value)
            if (v >= trimStart) onTrimEndChange(v)
          }}
          className="w-full accent-red-500"
        />
      </div>

      {/* Summary */}
      <div className="flex items-center justify-between text-xs text-neutral-500 dark:text-neutral-400 pt-1 border-t border-neutral-200 dark:border-neutral-700">
        <span>
          {trimmedCount} / {totalPoints} Punkte behalten ({trimmedPct}%)
        </span>
        {(trimStart > 0 || trimEnd < totalPoints - 1) && (
          <button
            onClick={() => {
              onTrimStartChange(0)
              onTrimEndChange(totalPoints - 1)
            }}
            className="text-xs text-neutral-400 dark:text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 transition-colors underline"
          >
            Zurücksetzen
          </button>
        )}
      </div>
    </div>
  )
}
