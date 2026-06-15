'use client'

import { useState, useCallback } from 'react'
import { PageHeader } from '@/components/layout/page-header'
import { SectionCard } from '@/components/shared/section-card'
import { Button } from '@/components/ui/button'
import { Copy, Check } from 'lucide-react'
import { cn } from '@/lib/utils/cn'

type TabId =
  | 'basic'
  | 'routing'
  | 'ml'
  | 'decision-ops'
  | 'openai-responses'
  | 'anthropic'
  | 'gemini'
  | 'summary'

const TABS: { id: TabId; label: string }[] = [
  { id: 'basic', label: 'Basic' },
  { id: 'routing', label: 'Routing' },
  { id: 'ml', label: 'ML Model' },
  { id: 'decision-ops', label: 'DecisionOps' },
  { id: 'openai-responses', label: 'OpenAI Responses' },
  { id: 'anthropic', label: 'Anthropic' },
  { id: 'gemini', label: 'Google Gemini' },
  { id: 'summary', label: 'Summary' },
]

function CodeBlock({ code }: { code: string }) {
  const [copied, setCopied] = useState(false)

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(code.trim())
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // clipboard unavailable — silent fail
    }
  }, [code])

  return (
    <div className="relative group">
      <pre className="rounded-lg bg-zinc-950 dark:bg-zinc-900 p-4 pr-20 text-sm font-mono text-zinc-100 overflow-x-auto leading-relaxed whitespace-pre">
        <code>{code.trim()}</code>
      </pre>
      <button
        type="button"
        onClick={handleCopy}
        className={cn(
          'absolute top-3 right-3 flex items-center gap-1.5 rounded px-2 py-1 text-xs font-medium transition-all',
          'bg-zinc-700 text-zinc-300 hover:bg-zinc-600 hover:text-white',
          copied && 'bg-green-700 text-green-100'
        )}
        aria-label="Copy code"
      >
        {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
        {copied ? 'Copied' : 'Copy'}
      </button>
    </div>
  )
}

function Section({ title, description, children }: { title: string; description?: string; children: React.ReactNode }) {
  return (
    <div className="space-y-2">
      <div>
        <h3 className="text-sm font-semibold text-foreground">{title}</h3>
        {description && <p className="text-xs text-muted-foreground mt-0.5">{description}</p>}
      </div>
      {children}
    </div>
  )
}

// ─── curl snippets ────────────────────────────────────────────────────────────

const CURL = {
  basicJwt: `\
curl -s http://localhost:5555/v1/chat/completions \\
  -H "Authorization: Bearer $TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "messages": [
      {"role": "user", "content": "Explain AI simply"}
    ]
  }'`,

  basicJwtStream: `\
curl -N -s http://localhost:5555/v1/chat/completions \\
  -H "Authorization: Bearer $TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "messages": [
      {"role": "user", "content": "Explain AI simply"}
    ]
  }'`,

  basicApiKey: `\
curl -s http://localhost:5555/v1/chat/completions \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "Content-Type: application/json" \\
  -d '{
    "messages": [
      {"role": "user", "content": "Explain AI simply"}
    ]
  }'`,

  basicApiKeyStream: `\
curl -N -s http://localhost:5555/v1/chat/completions \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "messages": [
      {"role": "user", "content": "Explain AI simply"}
    ]
  }'`,

  routingModel: `\
curl -s http://localhost:5555/v1/chat/completions \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "X-Model: claude-sonnet-4-6" \\
  -H "Content-Type: application/json" \\
  -d '{
    "messages": [
      {"role": "user", "content": "Write a short sentence about AI"}
    ]
  }'`,

  routingModelStream: `\
curl -N -s http://localhost:5555/v1/chat/completions \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "X-Model: claude-sonnet-4-6" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "messages": [
      {"role": "user", "content": "Write a short sentence about AI"}
    ]
  }'`,

  routingGroup: `\
curl -s http://localhost:5555/v1/chat/completions \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "X-Route-Group: cheap" \\
  -H "Content-Type: application/json" \\
  -d '{
    "messages": [
      {"role": "user", "content": "Write a short sentence about AI"}
    ]
  }'`,

  routingGroupStream: `\
curl -N -s http://localhost:5555/v1/chat/completions \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "X-Route-Group: cheap" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "messages": [
      {"role": "user", "content": "Write a short sentence about AI"}
    ]
  }'`,

  routingStream: `\
curl -N -s -X POST http://localhost:5555/v1/chat/completions \\
  -H "X-API-Key: rk_live_xxxx" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o-mini",
    "stream": true,
    "messages": [
      {"role": "user", "content": "Write a short sentence about AI"}
    ]
  }'`,

  ml: `\
curl -s http://localhost:5555/v1/chat/completions \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "X-Model-Type: ml" \\
  -H "X-Model-Name: <model-name-configured-in-platform>" \\
  -H "Content-Type: application/json" \\
  -d '{
    "input": {
      "features": {
        "amount": 1000,
        "country": "AR"
      }
    }
  }'`,

  decisionOpsSimple: `\
curl -s http://localhost:5555/v1/chat/completions \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "X-Workflow-Id: customer_support" \\
  -H "X-Conversation-Id: conv_abc123" \\
  -H "Content-Type: application/json" \\
  -d '{
    "messages": [
      {"role": "user", "content": "I need help with my invoice"}
    ]
  }'`,

  decisionOpsSimpleStream: `\
curl -N -s http://localhost:5555/v1/chat/completions \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "X-Workflow-Id: customer_support" \\
  -H "X-Conversation-Id: conv_abc123" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "messages": [
      {"role": "user", "content": "I need help with my invoice"}
    ]
  }'`,

  decisionOpsComplex: `\
curl -s http://localhost:5555/v1/chat/completions \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "X-Workflow-Id: customer_support" \\
  -H "X-Conversation-Id: conv_abc123" \\
  -H "X-Customer-Id: cust_9981" \\
  -H "X-Channel: whatsapp" \\
  -H "X-Interaction-Type: inbound" \\
  -H "X-Agent-Id: agent_007" \\
  -H "X-Department: support" \\
  -H "X-Ticket-Id: ticket_45521" \\
  -H "X-Customer-Segment: premium" \\
  -H "X-Language: es" \\
  -H "X-Intent: refund_request" \\
  -H "X-Experiment-Id: prompt_test_17" \\
  -H "X-Autonomy-Level: autonomous" \\
  -H "X-Policy-Id: premium_customer_policy" \\
  -H "X-Risk-Level: medium" \\
  -H "X-Revenue-Impact: 250.00" \\
  -H "X-Currency: USD" \\
  -H "Content-Type: application/json" \\
  -d '{
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful customer support agent. Reply in the customer language."
      },
      {
        "role": "user",
        "content": "I need help with my last invoice."
      }
    ]
  }'`,

  decisionOpsComplexStream: `\
curl -N -s http://localhost:5555/v1/chat/completions \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "X-Workflow-Id: customer_support" \\
  -H "X-Conversation-Id: conv_abc123" \\
  -H "X-Customer-Id: cust_9981" \\
  -H "X-Channel: whatsapp" \\
  -H "X-Interaction-Type: inbound" \\
  -H "X-Agent-Id: agent_007" \\
  -H "X-Department: support" \\
  -H "X-Ticket-Id: ticket_45521" \\
  -H "X-Customer-Segment: premium" \\
  -H "X-Language: es" \\
  -H "X-Intent: refund_request" \\
  -H "X-Experiment-Id: prompt_test_17" \\
  -H "X-Autonomy-Level: autonomous" \\
  -H "X-Policy-Id: premium_customer_policy" \\
  -H "X-Risk-Level: medium" \\
  -H "X-Revenue-Impact: 250.00" \\
  -H "X-Currency: USD" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful customer support agent. Reply in the customer language."
      },
      {
        "role": "user",
        "content": "I need help with my last invoice."
      }
    ]
  }'`,

  openaiResponses: `\
curl -s http://localhost:5555/v1/responses \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o",
    "input": [
      {"role": "user",      "content": "What is a transformer model?"},
      {"role": "assistant", "content": "A transformer is a neural network architecture..."},
      {"role": "user",      "content": "How does attention work in it?"}
    ],
    "max_output_tokens": 512,
    "temperature": 0.7
  }'`,

  openaiResponsesStream: `\
curl -N -s http://localhost:5555/v1/responses \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o",
    "stream": true,
    "input": [
      {"role": "user",      "content": "What is a transformer model?"},
      {"role": "assistant", "content": "A transformer is a neural network architecture..."},
      {"role": "user",      "content": "How does attention work in it?"}
    ],
    "max_output_tokens": 512,
    "temperature": 0.7
  }'`,

  anthropic: `\
curl -s http://localhost:5555/v1/messages \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "claude-sonnet-4-6",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Explain recursion with a simple analogy."}
    ]
  }'`,

  anthropicStream: `\
curl -N -s http://localhost:5555/v1/messages \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "claude-sonnet-4-6",
    "max_tokens": 1024,
    "stream": true,
    "messages": [
      {"role": "user", "content": "Explain recursion with a simple analogy."}
    ]
  }'`,

  gemini: `\
curl -s -X POST \\
  "http://localhost:5555/v1/models/gemini-1.5-pro:generateContent" \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "Content-Type: application/json" \\
  -d '{
    "contents": [
      {
        "role": "user",
        "parts": [{"text": "Explain what a large language model is."}]
      }
    ]
  }'`,

  geminiStream: `\
curl -N -s -X POST \\
  "http://localhost:5555/v1/models/gemini-1.5-pro:streamGenerateContent" \\
  -H "X-API-Key: rk_live_XXXX" \\
  -H "Content-Type: application/json" \\
  -d '{
    "contents": [
      {
        "role": "user",
        "parts": [{"text": "Explain what a large language model is."}]
      }
    ]
  }'`,
}

// ─── Tab content ─────────────────────────────────────────────────────────────

function TabBasic() {
  return (
    <div className="space-y-6">
      <Section title="JWT Authentication" description="Pass a Bearer token obtained from your identity provider (e.g. Keycloak).">
        <div className="space-y-3">
          <CodeBlock code={CURL.basicJwt} />
          <p className="text-xs text-muted-foreground pl-1">Streaming variant — responses arrive as Server-Sent Events:</p>
          <CodeBlock code={CURL.basicJwtStream} />
        </div>
      </Section>

      <Section title="API Key Authentication" description="Pass a gateway-issued API key via the X-API-Key header.">
        <div className="space-y-3">
          <CodeBlock code={CURL.basicApiKey} />
          <p className="text-xs text-muted-foreground pl-1">Streaming variant:</p>
          <CodeBlock code={CURL.basicApiKeyStream} />
        </div>
      </Section>
    </div>
  )
}

function TabRouting() {
  return (
    <div className="space-y-6">
      <Section
        title="Select a specific model"
        description="X-Model overrides tenant routing and pins the request to one model."
      >
        <div className="space-y-3">
          <CodeBlock code={CURL.routingModel} />
          <p className="text-xs text-muted-foreground pl-1">Streaming variant:</p>
          <CodeBlock code={CURL.routingModelStream} />
        </div>
      </Section>

      <Section
        title="Select a Route Group"
        description="X-Route-Group routes the request to a pre-configured group of models (e.g. cheap, fast, private)."
      >
        <div className="space-y-3">
          <CodeBlock code={CURL.routingGroup} />
          <p className="text-xs text-muted-foreground pl-1">Streaming variant:</p>
          <CodeBlock code={CURL.routingGroupStream} />
        </div>
      </Section>

      <Section
        title="Streaming via body parameter"
        description='Set "stream": true in the request body to receive tokens as they are generated. Use -N (no buffering) in curl.'
      >
        <CodeBlock code={CURL.routingStream} />
      </Section>
    </div>
  )
}

function TabML() {
  return (
    <div className="space-y-6">
      <Section
        title="Local ML Model inference"
        description="Route to an on-premise ML model registered in the platform. Use X-Model-Type: ml and X-Model-Name to identify the target. The body carries the feature vector in the input.features object."
      >
        <CodeBlock code={CURL.ml} />
      </Section>

      <div className="rounded-md border border-amber-200 bg-amber-50 dark:bg-amber-950/20 dark:border-amber-900 px-4 py-3 text-sm text-amber-800 dark:text-amber-300">
        <strong>Note:</strong> ML model responses are synchronous — streaming is not applicable for inference endpoints.
        Replace <code className="font-mono text-xs">amount</code> and <code className="font-mono text-xs">country</code>{' '}
        with the actual feature names expected by your model.
      </div>
    </div>
  )
}

function TabDecisionOps() {
  return (
    <div className="space-y-6">
      <Section
        title="Simple workflow & conversation tracking"
        description="Attach a workflow and conversation ID to every request. The gateway logs and traces the full conversation lifecycle."
      >
        <div className="space-y-3">
          <CodeBlock code={CURL.decisionOpsSimple} />
          <p className="text-xs text-muted-foreground pl-1">Streaming variant:</p>
          <CodeBlock code={CURL.decisionOpsSimpleStream} />
        </div>
      </Section>

      <Section
        title="Complex business context tracking"
        description="Attach full business metadata to every request. All headers are optional — add only what your workflow needs."
      >
        <div className="space-y-3">
          <CodeBlock code={CURL.decisionOpsComplex} />
          <p className="text-xs text-muted-foreground pl-1">Streaming variant:</p>
          <CodeBlock code={CURL.decisionOpsComplexStream} />
        </div>
      </Section>

      <div className="rounded-md border px-4 py-3 text-sm text-muted-foreground">
        <p className="font-medium text-foreground mb-1">Business context headers reference</p>
        <ul className="space-y-0.5 text-xs font-mono">
          {[
            ['X-Workflow-Id', 'Workflow identifier (required for DecisionOps)'],
            ['X-Conversation-Id', 'Conversation thread identifier'],
            ['X-Customer-Id', 'End customer identifier'],
            ['X-Channel', 'Interaction channel (web, whatsapp, email…)'],
            ['X-Interaction-Type', 'inbound / outbound'],
            ['X-Agent-Id', 'Human or bot agent identifier'],
            ['X-Department', 'Organizational unit'],
            ['X-Ticket-Id', 'Support ticket reference'],
            ['X-Customer-Segment', 'Customer tier (premium, standard…)'],
            ['X-Language', 'ISO 639-1 language code'],
            ['X-Intent', 'Detected or declared intent'],
            ['X-Experiment-Id', 'A/B test or prompt experiment'],
            ['X-Autonomy-Level', 'autonomous / supervised'],
            ['X-Policy-Id', 'Policy ruleset to apply'],
            ['X-Risk-Level', 'low / medium / high'],
            ['X-Revenue-Impact', 'Numeric value'],
            ['X-Currency', 'ISO 4217 currency code'],
          ].map(([header, desc]) => (
            <li key={header} className="flex gap-3">
              <span className="text-sky-600 dark:text-sky-400 w-52 shrink-0">{header}</span>
              <span className="text-muted-foreground">{desc}</span>
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}

function TabOpenAIResponses() {
  return (
    <div className="space-y-6">
      <Section
        title="OpenAI Responses API"
        description="Compatible with the OpenAI /v1/responses endpoint. Supports multi-turn input arrays and full conversation context."
      >
        <div className="space-y-3">
          <CodeBlock code={CURL.openaiResponses} />
          <p className="text-xs text-muted-foreground pl-1">Streaming variant:</p>
          <CodeBlock code={CURL.openaiResponsesStream} />
        </div>
      </Section>
    </div>
  )
}

function TabAnthropic() {
  return (
    <div className="space-y-6">
      <Section
        title="Anthropic Messages API"
        description="Native Anthropic wire format. Use this when your SDK targets the Anthropic API directly — no OpenAI translation layer."
      >
        <div className="space-y-3">
          <CodeBlock code={CURL.anthropic} />
          <p className="text-xs text-muted-foreground pl-1">Streaming variant — returns Anthropic SSE events:</p>
          <CodeBlock code={CURL.anthropicStream} />
        </div>
      </Section>
    </div>
  )
}

function TabGemini() {
  return (
    <div className="space-y-6">
      <Section
        title="Google Gemini — generateContent"
        description="Standard (non-streaming) Gemini content generation."
      >
        <CodeBlock code={CURL.gemini} />
      </Section>

      <Section
        title="Google Gemini — streamGenerateContent"
        description="Streaming variant. Uses a different endpoint path (:streamGenerateContent). Add -N to disable curl's output buffering."
      >
        <CodeBlock code={CURL.geminiStream} />
      </Section>
    </div>
  )
}

function TabSummary() {
  const endpoints = [
    { endpoint: 'POST /v1/chat/completions', format: 'OpenAI Chat', streaming: '"stream": true' },
    { endpoint: 'POST /v1/responses', format: 'OpenAI Responses', streaming: '"stream": true' },
    { endpoint: 'POST /v1/messages', format: 'Anthropic Messages', streaming: '"stream": true' },
    { endpoint: 'POST /v1/models/{model}:generateContent', format: 'Google Gemini', streaming: '— (use :streamGenerateContent)' },
    { endpoint: 'POST /v1/models/{model}:streamGenerateContent', format: 'Google Gemini', streaming: 'SSE (native)' },
    { endpoint: 'POST /v1/embeddings', format: 'OpenAI Embeddings', streaming: '—' },
  ]

  return (
    <div className="space-y-6">
      <Section title="All Endpoints">
        <div className="rounded-md border overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50">
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">Endpoint</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">Wire Format</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">Streaming</th>
              </tr>
            </thead>
            <tbody>
              {endpoints.map((row, i) => (
                <tr key={row.endpoint} className={cn('border-b last:border-0', i % 2 === 1 && 'bg-muted/20')}>
                  <td className="px-4 py-2.5 font-mono text-xs text-sky-600 dark:text-sky-400">{row.endpoint}</td>
                  <td className="px-4 py-2.5 text-muted-foreground">{row.format}</td>
                  <td className="px-4 py-2.5 font-mono text-xs text-muted-foreground">{row.streaming}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Section>

      <Section title="Shared across all endpoints">
        <div className="rounded-md border px-4 py-3 space-y-2 text-sm text-muted-foreground">
          {[
            ['Authentication', 'X-API-Key: rk_live_… or Authorization: Bearer <token>'],
            ['Routing', 'X-Model, X-Route-Group'],
            ['DecisionOps', 'X-Workflow-Id, X-Conversation-Id, and all business context headers'],
            ['PII hooks', 'Automatic — configured per tenant'],
            ['Rate limiting', 'Automatic — configured per tenant / API key / user'],
          ].map(([label, value]) => (
            <div key={label} className="flex gap-3">
              <span className="font-medium text-foreground w-36 shrink-0">{label}</span>
              <span className="font-mono text-xs">{value}</span>
            </div>
          ))}
        </div>
      </Section>
    </div>
  )
}

const TAB_CONTENT: Record<TabId, React.ReactNode> = {
  basic: <TabBasic />,
  routing: <TabRouting />,
  ml: <TabML />,
  'decision-ops': <TabDecisionOps />,
  'openai-responses': <TabOpenAIResponses />,
  anthropic: <TabAnthropic />,
  gemini: <TabGemini />,
  summary: <TabSummary />,
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function UsageDocPage() {
  const [activeTab, setActiveTab] = useState<TabId>('basic')

  return (
    <div>
      <PageHeader
        title="Usage Doc"
        description="cURL examples for every gateway endpoint — replace localhost:5555 with your gateway URL."
      />

      <div className="mt-4 flex flex-wrap gap-2">
        {TABS.map((tab) => (
          <Button
            key={tab.id}
            type="button"
            variant={activeTab === tab.id ? 'default' : 'outline'}
            onClick={() => setActiveTab(tab.id)}
          >
            {tab.label}
          </Button>
        ))}
      </div>

      <div className="mt-6">
        <SectionCard title={TABS.find((t) => t.id === activeTab)?.label ?? ''}>
          {TAB_CONTENT[activeTab]}
        </SectionCard>
      </div>
    </div>
  )
}
