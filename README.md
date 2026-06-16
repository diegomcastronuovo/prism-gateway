# PrismGateway

**Open-source LLM gateway with semantic routing, semantic caching, and real cost control.**

Route AI traffic intelligently across any provider — OpenAI, Anthropic, Gemini, xAI, Bedrock, Cohere, DeepSeek, Kimi, or any OpenAI-compatible endpoint. PrismGateway goes beyond simple proxying: it understands *what* the request is about and routes accordingly.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev)

---

## Why PrismGateway?

Most LLM gateways route blindly — round-robin or static rules. PrismGateway routes by **meaning**.

| Feature | PrismGateway | LiteLLM | OpenRouter |
|---------|:---:|:---:|:---:|
| Multi-provider routing | ✅ | ✅ | ✅ |
| Semantic routing (by query intent) | ✅ | ❌ | ❌ |
| Semantic caching (pgvector) | ✅ | partial | ❌ |
| Tool routing (semantic) | ✅ | ❌ | ❌ |
| PII-aware routing | ✅ | ❌ | ❌ |
| Real budget enforcement (block/degrade) | ✅ | partial | ❌ |
| Distributed circuit breaker (Redis) | ✅ | ❌ | ❌ |
| Auto model benchmarking | ✅ | ❌ | ❌ |
| Multi-tenant isolation | ✅ | ✅ | ❌ |
| Self-hosted | ✅ | ✅ | ❌ |

---

## Key Features

### Semantic Routing
Route requests to different model groups based on the *meaning* of the query — not just keywords or headers. Define semantic anchors and let the gateway find the best match using vector similarity.

```
"Explain compound interest" → finance-group (claude-3-opus)
"Generate a Python script"  → code-group   (gpt-4o)
"Translate this to French"  → general-group (gemini-1.5-flash)
```

### Semantic Caching
Cache LLM responses by semantic similarity, not exact match. If a user asks the same question in a different way, they get the cached answer instantly — with zero provider cost.

### Tool Routing
Automatically detect when a request matches a registered tool and short-circuit to it before hitting any LLM provider.

### PII-Aware Routing
Detect sensitive data via webhook and automatically reroute to a designated PII-safe model — or block the request entirely.

### Real Cost Control
Set monthly budgets per tenant with hard and soft thresholds. When a tenant approaches their limit, the gateway automatically degrades to cheaper models. At the limit, requests are blocked. No surprises on your bill.

### Distributed Circuit Breaker
Redis-backed circuit breaker with 3-state FSM (CLOSED → OPEN → HALF_OPEN). Failures on one provider automatically redirect traffic — across all gateway instances.

### Auto Model Benchmarking
The gateway periodically benchmarks every configured model and records latency, cost, and reliability. Routing decisions use real-world data, not assumptions.

---

## Quick Start

### Prerequisites
- Docker and Docker Compose
- API keys for at least one provider

### 1. Clone and configure

```bash
git clone https://github.com/diegomcastronuovo/prism-gateway.git
cd prism-gateway

cp config.example.yaml config.yaml
cp .env.example .env
```

Edit `.env` with your provider API keys. Edit `config.yaml` to define your tenants, models, and routing strategy.

### 2. Start

```bash
docker compose up -d
```

The gateway starts on port `8080`. Postgres and Redis are included.

### 3. Verify

```bash
curl http://localhost:8080/healthz
```

### 4. Send your first request

```bash
  curl http://localhost:8080/v1/chat/completions \
  -H "X-API-Key: Bearer your-api-key" \
  -H "X-Tenant-ID: default" \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

The response is OpenAI-compatible. Any SDK or tool that works with OpenAI works with PrismGateway.

---

## Configuration

PrismGateway uses two files:

| File | Purpose |
|------|---------|
| `config.yaml` | Tenants, models, routing rules, budgets, rate limits |
| `.env` | Provider API keys (never committed) |

See [`config.example.yaml`](config.example.yaml) for a fully annotated example.

### Supported Providers

| Provider | Chat | Embeddings |
|----------|:----:|:----------:|
| OpenAI | ✅ | ✅ |
| Anthropic | ✅ | ❌ |
| Google Gemini | ✅ | ✅ |
| xAI (Grok) | ✅ | ❌ |
| AWS Bedrock | ✅ | ❌ |
| Cohere | ❌ | ✅ |
| DeepSeek | ✅ | ❌ |
| Kimi (Moonshot) | ✅ | ❌ |
| HTTP local (Ollama, vLLM) | ✅ | ✅ |
| Mock (testing) | ✅ | ✅ |

### Routing Strategies

| Strategy | Description |
|----------|-------------|
| `round_robin` | Distribute requests evenly across models |
| `latency_based` | Route to the fastest model (EWMA-weighted) |
| `cost_based` | Route to the cheapest model for the request |
| `header_based` | Route based on a custom request header |
| `smart` | Multi-stage: semantic → cost → latency combined |

---

## Architecture

```
Client Request
      │
      ▼
┌─────────────────────────────────────┐
│           PrismGateway              │
│                                     │
│  Auth → Rate Limit → Tool Routing   │
│       → Semantic Routing            │
│       → PII Check                   │
│       → Model Selection             │
│       → Circuit Breaker             │
│       → Provider Call               │
│       → Semantic Cache Write        │
└─────────────────────────────────────┘
      │
      ▼
┌──────────┐  ┌──────────┐  ┌──────────┐
│  OpenAI  │  │Anthropic │  │  Gemini  │  ...
└──────────┘  └──────────┘  └──────────┘
```

**Infrastructure:**
- Go 1.22+ backend — single binary, low memory footprint
- PostgreSQL + pgvector — tenant config, usage, semantic index
- Redis — circuit breaker state, rate limiting, distributed cache

---

## API

PrismGateway exposes an OpenAI-compatible API. Drop it in as a replacement for the OpenAI SDK by changing the base URL.

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="your-api-key",
    default_headers={"X-Tenant-ID": "default"}
)

response = client.chat.completions.create(
    model="auto",
    messages=[{"role": "user", "content": "Hello!"}]
)
```

**Endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/chat/completions` | Chat completions (streaming supported) |
| `GET` | `/v1/models` | List available models for the tenant |
| `POST` | `/v1/embeddings` | Generate embeddings |
| `GET` | `/healthz` | Health check |

**Routing headers (optional):**

| Header | Description |
|--------|-------------|
| `X-Tenant-ID` | Tenant identifier (required if not in JWT) |
| `X-Route-Group` | Force a specific route group |
| `X-Model` | Force a specific model |

---

## Building from Source

```bash
git clone https://github.com/diegomcastronuovo/prism-gateway.git
cd prism-gateway

# Run migrations
make migrate

# Start in development mode
make dev

# Run tests
make test

# Build binary
make build
```

---

## Enterprise

PrismGateway Enterprise adds organizational governance on top of the open-source core:

- **DecisionOps** — workflow-based routing with automatic degradation policies and conversation lifecycle management
- **FinOps dashboards** — CFO Board, cost anomaly detection, cross-tenant spend analysis
- **Compliance module** — audit trails, data retention policies, regulatory reporting
- **MRM** — Model Risk Management with maker-checker approval workflows
- **Advanced alerting** — Slack and webhook notifications for budget events
- **Admin UI** — full management dashboard with RBAC for finance and audit roles

**→ [arkana.ar](http://arkana.ar) for Enterprise plans and pricing.**

---

## Contributing

Contributions are welcome. Please open an issue before submitting a large pull request so we can discuss the approach.

1. Fork the repo
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Commit your changes
4. Open a pull request

---

## License

PrismGateway is MIT licensed. See [LICENSE](LICENSE).
