'use client'

import { useEffect, useMemo, useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  type ExternalPiiConfig,
  type TenantConfig,
  useUpdateTenantPiiConfig,
} from '../api/use-tenants'
import { Badge } from '@/components/ui/badge'
import { useToast } from '@/hooks/use-toast'

interface EditPiiDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  tenantConfig: TenantConfig | null
}

interface ValidationErrors {
  request_url?: string
  response_url?: string
  timeout_ms?: string
  failure_policy?: string
}

const DEFAULT_PII: Required<Pick<ExternalPiiConfig, 'enabled' | 'request_url' | 'response_url' | 'timeout_ms' | 'failure_policy'>> & { api_key: string } = {
  enabled: false,
  request_url: '',
  response_url: '',
  timeout_ms: 3000,
  failure_policy: 'accept',
  api_key: '',
}

function isValidHttpUrl(value: string): boolean {
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

export function EditPiiDialog({ open, onOpenChange, tenantConfig }: EditPiiDialogProps) {
  const updatePiiConfig = useUpdateTenantPiiConfig()
  const { toast } = useToast()
  const [form, setForm] = useState(DEFAULT_PII)
  const [errors, setErrors] = useState<ValidationErrors>({})
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<
    | null
    | {
        request_endpoint?: { status?: string; status_code?: number; latency_ms?: number; error?: string }
        response_endpoint?: { status?: string; status_code?: number; latency_ms?: number; error?: string }
      }
  >(null)

  useEffect(() => {
    if (!open) return

    const current = tenantConfig?.config?.hooks?.global?.external_pii
    setForm({
      enabled: current?.enabled === true,
      request_url: current?.request_url || '',
      response_url: current?.response_url || '',
      timeout_ms: current?.timeout_ms ?? 3000,
      failure_policy: current?.failure_policy === 'deny' ? 'deny' : 'accept',
      api_key: current?.api_key || '',
    })
    setErrors({})
  }, [open, tenantConfig])

  const hasValidationErrors = useMemo(() => Object.keys(errors).length > 0, [errors])

  const validate = (): boolean => {
    const nextErrors: ValidationErrors = {}

    if (form.enabled) {
      if (!form.request_url || !isValidHttpUrl(form.request_url)) {
        nextErrors.request_url = 'Request URL must be a valid URL'
      }
      if (!form.response_url || !isValidHttpUrl(form.response_url)) {
        nextErrors.response_url = 'Response URL must be a valid URL'
      }
      if (form.timeout_ms < 500 || form.timeout_ms > 10000) {
        nextErrors.timeout_ms = 'Timeout must be between 500 and 10000 ms'
      }
      if (form.failure_policy !== 'accept' && form.failure_policy !== 'deny') {
        nextErrors.failure_policy = 'Failure policy must be accept or deny'
      }
    }

    setErrors(nextErrors)
    return Object.keys(nextErrors).length === 0
  }

  const onSubmit = async () => {
    if (!tenantConfig) return
    if (!validate()) return

    const patch: Record<string, unknown> = {
      hooks: {
        global: {
          external_pii: {
            enabled: form.enabled,
            request_url: form.request_url.trim(),
            response_url: form.response_url.trim(),
            timeout_ms: form.timeout_ms,
            failure_policy: form.failure_policy,
            ...(form.api_key && { api_key: form.api_key.trim() }),
          },
        },
      },
    }

    try {
      await updatePiiConfig.mutateAsync({
        tenantId: tenantConfig.tenant_id,
        version: tenantConfig.version,
        patch,
      })
      onOpenChange(false)
    } catch {
      // Error toast is handled by mutation hook
    }
  }

  // SPEC_65: validate for test regardless of enabled toggle
  const validateForTest = (): boolean => {
    const nextErrors: ValidationErrors = {}
    if (!form.request_url || !isValidHttpUrl(form.request_url)) {
      nextErrors.request_url = 'Request URL must be a valid URL'
    }
    if (!form.response_url || !isValidHttpUrl(form.response_url)) {
      nextErrors.response_url = 'Response URL must be a valid URL'
    }
    if (form.timeout_ms < 500 || form.timeout_ms > 10000) {
      nextErrors.timeout_ms = 'Timeout must be between 500 and 10000 ms'
    }
    setErrors(nextErrors)
    return Object.keys(nextErrors).length === 0
  }

  const onTestConnection = async () => {
    if (!tenantConfig) return
    // Clear previous result
    setTestResult(null)
    if (!validateForTest()) return
    try {
      setTesting(true)
      const res = await fetch(`/api/tenants/${tenantConfig.tenant_id}/pii/test-connection`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          request_url: form.request_url.trim(),
          response_url: form.response_url.trim(),
          timeout_ms: form.timeout_ms,
          api_key: form.api_key.trim(),
        }),
      })
      if (!res.ok) {
        toast({ title: 'Failed to test PII connection', variant: 'destructive' })
        setTesting(false)
        return
      }
      const data = await res.json()
      setTestResult(data)
    } catch {
      toast({ title: 'Could not reach the gateway', variant: 'destructive' })
    } finally {
      setTesting(false)
    }
  }

  const onFieldChange = (updater: (prev: typeof form) => typeof form) => {
    setForm((prev) => {
      const next = updater(prev)
      // Clear previous test results on change per SPEC_65
      if (
        next.request_url !== prev.request_url ||
        next.response_url !== prev.response_url ||
        next.timeout_ms !== prev.timeout_ms ||
        next.api_key !== prev.api_key
      ) {
        setTestResult(null)
      }
      return next
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Edit PII Protection</DialogTitle>
          <DialogDescription>
            Configure external PII webhook for {tenantConfig?.tenant_id}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-5 py-2">
          <div className="flex items-center justify-between rounded-lg border p-3">
            <div className="space-y-1">
              <Label htmlFor="external-pii-enabled" className="font-medium">Enable External PII Webhook</Label>
              <p className="text-xs text-muted-foreground">Enable webhook checks before request/response processing</p>
            </div>
            <Switch
              id="external-pii-enabled"
              checked={form.enabled}
              onCheckedChange={(checked: boolean) => setForm((prev) => ({ ...prev, enabled: checked }))}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="pii-request-url">Request URL</Label>
            <Input
              id="pii-request-url"
              placeholder="https://pii-service.company.com/request"
              value={form.request_url}
              onChange={(e) => onFieldChange((prev) => ({ ...prev, request_url: e.target.value }))}
            />
            <p className="text-xs text-muted-foreground">URL invoked before sending the request to the model.</p>
            {errors.request_url && <p className="text-xs text-destructive">{errors.request_url}</p>}
          </div>

          <div className="space-y-2">
            <Label htmlFor="pii-response-url">Response URL</Label>
            <Input
              id="pii-response-url"
              placeholder="https://pii-service.company.com/response"
              value={form.response_url}
              onChange={(e) => onFieldChange((prev) => ({ ...prev, response_url: e.target.value }))}
            />
            <p className="text-xs text-muted-foreground">URL invoked to validate the model response.</p>
            {errors.response_url && <p className="text-xs text-destructive">{errors.response_url}</p>}
          </div>

          <div className="space-y-2">
            <Label htmlFor="pii-timeout-ms">Timeout (ms)</Label>
            <Input
              id="pii-timeout-ms"
              type="number"
              min={500}
              max={10000}
              value={form.timeout_ms}
              onChange={(e) => onFieldChange((prev) => ({ ...prev, timeout_ms: Number(e.target.value) || 0 }))}
            />
            {errors.timeout_ms && <p className="text-xs text-destructive">{errors.timeout_ms}</p>}
          </div>

          <div className="space-y-2">
            <Label htmlFor="pii-api-key">API Key</Label>
            <Input
              id="pii-api-key"
              type="password"
              placeholder="sk-xxxxxxxxxxxx"
              value={form.api_key}
              onChange={(e) => onFieldChange((prev) => ({ ...prev, api_key: e.target.value }))}
            />
            <p className="text-xs text-muted-foreground">API key for authenticating requests to the webhook endpoint (optional).</p>
          </div>

          <div className="space-y-2">
            <Label>Failure Policy</Label>
            <Select
              value={form.failure_policy}
              onValueChange={(value: 'accept' | 'deny') => setForm((prev) => ({ ...prev, failure_policy: value }))}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select failure policy" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="accept">Accept (fail-open)</SelectItem>
                <SelectItem value="deny">Deny (fail-closed)</SelectItem>
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">Defines behavior if webhook is unreachable or times out.</p>
            {errors.failure_policy && <p className="text-xs text-destructive">{errors.failure_policy}</p>}
          </div>
        </div>

        {/* SPEC_65: Connection Test Result */}
        {testResult && (
          <div className="space-y-3 border rounded-lg p-3">
            <h4 className="text-sm font-medium">Connection Test Result</h4>
            <div className="grid gap-3">
              <div className="rounded border p-3 space-y-1">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium">Request endpoint</span>
                  <Badge
                    variant={
                      testResult.request_endpoint?.status === 'ok'
                        ? 'default'
                        : testResult.request_endpoint?.status === 'timeout'
                        ? 'secondary'
                        : 'destructive'
                    }
                  >
                    {(testResult.request_endpoint?.status || 'unknown').toUpperCase()}
                  </Badge>
                </div>
                <div className="text-xs grid grid-cols-2 gap-1">
                  {typeof testResult.request_endpoint?.status_code === 'number' && (
                    <div>HTTP: {testResult.request_endpoint?.status_code}</div>
                  )}
                  {typeof testResult.request_endpoint?.latency_ms === 'number' && (
                    <div>Latency: {testResult.request_endpoint?.latency_ms} ms</div>
                  )}
                  {testResult.request_endpoint?.error && (
                    <div className="col-span-2 text-destructive">Error: {testResult.request_endpoint?.error}</div>
                  )}
                </div>
              </div>

              <div className="rounded border p-3 space-y-1">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium">Response endpoint</span>
                  <Badge
                    variant={
                      testResult.response_endpoint?.status === 'ok'
                        ? 'default'
                        : testResult.response_endpoint?.status === 'timeout'
                        ? 'secondary'
                        : 'destructive'
                    }
                  >
                    {(testResult.response_endpoint?.status || 'unknown').toUpperCase()}
                  </Badge>
                </div>
                <div className="text-xs grid grid-cols-2 gap-1">
                  {typeof testResult.response_endpoint?.status_code === 'number' && (
                    <div>HTTP: {testResult.response_endpoint?.status_code}</div>
                  )}
                  {typeof testResult.response_endpoint?.latency_ms === 'number' && (
                    <div>Latency: {testResult.response_endpoint?.latency_ms} ms</div>
                  )}
                  {testResult.response_endpoint?.error && (
                    <div className="col-span-2 text-destructive">Error: {testResult.response_endpoint?.error}</div>
                  )}
                </div>
              </div>
            </div>
          </div>
        )}

        <DialogFooter>
          <Button type="button" variant="outline" onClick={onTestConnection} disabled={testing}>
            {testing ? 'Testing...' : 'Test Connection'}
          </Button>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={onSubmit} disabled={updatePiiConfig.isPending || hasValidationErrors}>
            {updatePiiConfig.isPending ? 'Saving...' : 'Save Changes'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
