import { Badge } from '@/components/ui/badge'
import { Status } from '@/types/common'
import { cn } from '@/lib/utils/cn'

interface StatusBadgeProps {
  status: Status
  className?: string
}

const statusConfig: Record<Status, { label: string; className: string }> = {
  active: {
    label: 'Active',
    className: 'bg-green-500/10 text-green-700 dark:text-green-400 border-green-500/20',
  },
  inactive: {
    label: 'Inactive',
    className: 'bg-gray-500/10 text-gray-700 dark:text-gray-400 border-gray-500/20',
  },
  pending: {
    label: 'Pending',
    className: 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-400 border-yellow-500/20',
  },
  error: {
    label: 'Error',
    className: 'bg-red-500/10 text-red-700 dark:text-red-400 border-red-500/20',
  },
}

export function StatusBadge({ status, className }: StatusBadgeProps) {
  const config = statusConfig[status]
  
  return (
    <Badge variant="outline" className={cn(config.className, className)}>
      {config.label}
    </Badge>
  )
}
