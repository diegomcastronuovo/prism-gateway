import { LucideIcon } from 'lucide-react'
import Link from 'next/link'
import { Card, CardContent } from '@/components/ui/card'
import { cn } from '@/lib/utils/cn'
import { ReactNode } from 'react'

interface StatCardProps {
  title: string
  value: string | number
  description?: string | ReactNode
  icon?: LucideIcon
  trend?: {
    value: number
    isPositive: boolean
  }
  className?: string
  href?: string
}

export function StatCard({
  title,
  value,
  description,
  icon: Icon,
  trend,
  className,
  href,
}: StatCardProps) {
  const content = (
    <CardContent className="p-6 min-h-[152px]">
      <div className="flex h-full flex-col gap-2 items-stretch">
        <p className="text-sm font-medium text-muted-foreground text-center leading-tight flex items-center justify-center h-10">{title}</p>
        <div className="flex items-baseline justify-center gap-2">
          <p className="text-3xl font-bold tabular-nums">{value}</p>
          {trend && (
            <span
              className={cn(
                'text-sm font-medium',
                trend.isPositive ? 'text-green-600' : 'text-red-600'
              )}
            >
              {trend.isPositive ? '+' : ''}
              {trend.value}%
            </span>
          )}
        </div>
        {description && (
          <p className="text-xs text-muted-foreground text-center">{description}</p>
        )}
        {Icon && (
          <div className="mt-3 flex justify-center">
            <div className="rounded-full bg-primary/10 p-3">
              <Icon className="h-5 w-5 text-primary" />
            </div>
          </div>
        )}
      </div>
    </CardContent>
  )

  if (href) {
    return (
      <Link href={href} className="block">
        <Card
          className={cn(
            'transition-all hover:shadow-md hover:border-primary/50 cursor-pointer',
            className
          )}
        >
          {content}
        </Card>
      </Link>
    )
  }

  return <Card className={cn('', className)}>{content}</Card>
}
