'use client'

import { Palette } from 'lucide-react'
import { useThemeAccent } from '@/hooks/use-theme-accent'
import { ThemeAccent } from '@/lib/themes/theme-config'
import { cn } from '@/lib/utils/cn'

const accents: { value: ThemeAccent; label: string; color: string }[] = [
  { value: 'magenta', label: 'Magenta', color: 'bg-[#E6399B]' },
  { value: 'blue', label: 'Blue', color: 'bg-blue-500' },
  { value: 'violet', label: 'Violet', color: 'bg-violet-500' },
  { value: 'green', label: 'Green', color: 'bg-green-500' },
]

export function ThemeAccentSwitcher() {
  const { accent, setAccent } = useThemeAccent()

  return (
    <div className="flex items-center gap-2">
      <Palette className="h-4 w-4 text-muted-foreground" />
      <div className="flex gap-1">
        {accents.map((item) => (
          <button
            key={item.value}
            onClick={() => setAccent(item.value)}
            className={cn(
              'h-6 w-6 rounded-full border-2 transition-all',
              item.color,
              accent === item.value
                ? 'border-foreground scale-110'
                : 'border-transparent opacity-60 hover:opacity-100'
            )}
            title={item.label}
          />
        ))}
      </div>
    </div>
  )
}
