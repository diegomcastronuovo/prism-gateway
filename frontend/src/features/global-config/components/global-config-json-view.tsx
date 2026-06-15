'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { ChevronDown, ChevronUp, Copy, Check } from 'lucide-react'

interface GlobalConfigJsonViewProps {
  config: Record<string, unknown>
}

export function GlobalConfigJsonView({ config }: GlobalConfigJsonViewProps) {
  const [showJson, setShowJson] = useState(false)
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    await navigator.clipboard.writeText(JSON.stringify(config, null, 2))
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setShowJson(!showJson)}
        >
          {showJson ? (
            <>
              <ChevronUp className="h-4 w-4 mr-2" />
              Hide Raw JSON
            </>
          ) : (
            <>
              <ChevronDown className="h-4 w-4 mr-2" />
              Show Raw JSON
            </>
          )}
        </Button>
        {showJson && (
          <Button variant="outline" size="sm" onClick={handleCopy}>
            {copied ? (
              <>
                <Check className="h-4 w-4 mr-2" />
                Copied
              </>
            ) : (
              <>
                <Copy className="h-4 w-4 mr-2" />
                Copy
              </>
            )}
          </Button>
        )}
      </div>
      {showJson && (
        <pre className="bg-muted p-4 rounded-lg overflow-auto text-xs max-h-96 border">
          {JSON.stringify(config, null, 2)}
        </pre>
      )}
    </div>
  )
}
