import { cn } from '@/lib/utils'

interface StatCardProps {
  label: string
  value: string | number
  sub?: string
  className?: string
}

export function StatCard({ label, value, sub, className }: StatCardProps) {
  return (
    <div className={cn('rounded-lg border bg-card p-4 text-center', className)}>
      <div className="text-2xl font-bold">{value}</div>
      <div className="text-sm text-muted-foreground">{label}</div>
      {sub && <div className="text-xs text-muted-foreground mt-1">{sub}</div>}
    </div>
  )
}
