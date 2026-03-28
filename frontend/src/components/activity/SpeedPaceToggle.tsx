// frontend/src/components/activity/SpeedPaceToggle.tsx

interface Props {
  mode: 'speed' | 'pace'
  onChange: (mode: 'speed' | 'pace') => void
}

export function SpeedPaceToggle({ mode, onChange }: Props) {
  return (
    <div className="inline-flex rounded-lg border border-neutral-200 dark:border-neutral-700 text-xs overflow-hidden">
      <button
        onClick={() => onChange('pace')}
        className={`px-3 py-1 cursor-pointer transition-colors ${
          mode === 'pace'
            ? 'bg-accent-DEFAULT text-white'
            : 'hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-400'
        }`}
      >
        Pace
      </button>
      <button
        onClick={() => onChange('speed')}
        className={`px-3 py-1 cursor-pointer transition-colors ${
          mode === 'speed'
            ? 'bg-accent-DEFAULT text-white'
            : 'hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-400'
        }`}
      >
        Geschwindigkeit
      </button>
    </div>
  )
}
