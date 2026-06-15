'use client'

import { cn } from '@/lib/utils/cn'

interface ProviderMeta {
  label: string
  initial: string
  gradient: string   // CSS gradient for background
  ring: string       // border/ring color
}

const PROVIDER_META: Record<string, ProviderMeta> = {
  openai:    { label: 'OpenAI',    initial: 'O',  gradient: 'linear-gradient(135deg,#10a37f,#1dc586)', ring: '#10a37f' },
  anthropic: { label: 'Anthropic', initial: 'A',  gradient: 'linear-gradient(135deg,#cc785c,#e8a87c)', ring: '#cc785c' },
  gemini:    { label: 'Gemini',    initial: 'G',  gradient: 'linear-gradient(135deg,#4285f4,#34a0f5)', ring: '#4285f4' },
  xai:       { label: 'xAI',      initial: 'x',  gradient: 'linear-gradient(135deg,#1a1a1a,#3a3a3a)', ring: '#555'    },
  deepseek:  { label: 'DeepSeek', initial: 'D',  gradient: 'linear-gradient(135deg,#4d6bfe,#7b93ff)', ring: '#4d6bfe' },
  kimi:      { label: 'Kimi',     initial: 'K',  gradient: 'linear-gradient(135deg,#7c3aed,#a855f7)', ring: '#7c3aed' },
  bedrock:   { label: 'Bedrock',  initial: 'B',  gradient: 'linear-gradient(135deg,#ff9900,#ffb84d)', ring: '#ff9900' },
  local:     { label: 'Local',    initial: 'L',  gradient: 'linear-gradient(135deg,#6b7280,#9ca3af)', ring: '#6b7280' },
}

const SIZE = {
  sm: { box: 'h-7 w-7',   font: 'text-[14px] font-semibold' },
  md: { box: 'h-9 w-9',   font: 'text-[17px] font-semibold' },
  lg: { box: 'h-11 w-11', font: 'text-[20px] font-semibold' },
} as const

type IconSize = keyof typeof SIZE

interface ProviderIconProps {
  providerId: string
  size?: IconSize
  className?: string
}

export function ProviderIcon({ providerId, size = 'md', className }: ProviderIconProps) {
  const meta = PROVIDER_META[providerId.toLowerCase()] ?? {
    label: providerId,
    initial: providerId.charAt(0).toUpperCase(),
    gradient: 'linear-gradient(135deg,#94a3b8,#cbd5e1)',
    ring: '#94a3b8',
  }

  const { box, font } = SIZE[size]

  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center justify-center rounded-full leading-none select-none shadow-sm',
        box,
        className,
      )}
      style={{
        background: meta.gradient,
        color: '#fff',
        boxShadow: `0 0 0 2px ${meta.ring}22`,
      }}
      title={meta.label}
      aria-label={meta.label}
    >
      <span className={font}>{meta.initial}</span>
    </span>
  )
}

/** Human-readable display name for a provider id. */
export function providerLabel(id: string): string {
  return PROVIDER_META[id.toLowerCase()]?.label ?? (id.charAt(0).toUpperCase() + id.slice(1))
}
