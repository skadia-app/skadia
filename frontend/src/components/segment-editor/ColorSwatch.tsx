// frontend/src/components/segment-editor/ColorSwatch.tsx
// Small color swatch for SAC/MTB difficulty next to select options.
interface ColorSwatchProps {
  color: 'blue' | 'red' | 'black'
  size?: number
}

export function ColorSwatch({ color, size = 24 }: ColorSwatchProps) {
  const bg = {
    blue: 'bg-blue-500',
    red: 'bg-red-500',
    black: 'bg-neutral-600',
  }[color]
  
  return (
    <span
      className={`inline-block rounded ${bg}`}
      style={{ width: size, height: size }}
      aria-hidden="true"
    />
  )
}
