import { RefreshCw } from 'lucide-react'
import { cn } from '@/lib/utils'

export function Spinner({ className }: { className?: string }) {
  return <RefreshCw className={cn('animate-spin h-4 w-4', className)} />
}
