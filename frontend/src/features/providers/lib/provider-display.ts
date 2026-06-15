import type { Provider } from '@/features/providers/api/use-providers'

export function isAwsBedrockProvider(p: Pick<Provider, 'id' | 'type'>): boolean {
  return p.type === 'aws_bedrock' || p.id === 'bedrock'
}

/** Display title for provider id (global config key). */
export function providerDisplayTitle(id: string): string {
  if (id === 'bedrock') return 'AWS Bedrock'
  if (!id) return 'Provider'
  return id.charAt(0).toUpperCase() + id.slice(1)
}

/** Detail panel / header title */
export function providerDetailHeading(id: string): string {
  if (id === 'bedrock') return 'AWS Bedrock Provider'
  return `${providerDisplayTitle(id)} Provider`
}

export function maskAwsAccessKeyId(value: string | undefined): string {
  if (!value || value.length === 0) return '—'
  if (value.length <= 8) return '••••••••'
  return `${value.slice(0, 4)}…${value.slice(-4)}`
}
