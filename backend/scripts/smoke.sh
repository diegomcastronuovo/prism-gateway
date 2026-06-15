#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
API_KEY="${API_KEY:-}"
TENANT_ID="${TENANT_ID:-}"

if [[ -z "${API_KEY}" ]]; then
  echo "Usage: API_KEY=<your-key> ./scripts/smoke.sh" >&2
  exit 1
fi

fail() { echo "❌ $*" >&2; exit 1; }
ok()   { echo "✅ $*"; }
note() { echo "ℹ️  $*"; }

# req:
#   req '{"json..."}'
#   req -H "X-Model: gpt-4o-mini" '{"json..."}'
# Prints: "<http_code>|<selected_model>|<body>"
req() {
  local -a hdrs
  hdrs=()

  # Parse -H "Header: value" ...
  while [[ "${1:-}" == "-H" ]]; do
    hdrs+=("$2")
    shift 2
  done

  local data="${1:-{}}"

  # Detect if caller provided explicit X-API-Key header
  local has_key="false"
  local h
  for h in ${hdrs[@]+"${hdrs[@]}"}; do
    if [[ "$h" == X-API-Key:* ]]; then
      has_key="true"
      break
    fi
  done

  local resp headers
  resp="$(mktemp)"
  headers="$(mktemp)"

  local -a curl_args
  curl_args=(
    -sS
    -D "$headers"
    -o "$resp"
    -X POST "${BASE_URL}/v1/chat/completions"
    -H "Content-Type: application/json"
  )

  # Default key unless overridden
  if [[ "$has_key" != "true" ]]; then
    curl_args+=(-H "X-API-Key: ${API_KEY}")
  fi

  # Optional tenant header ONLY if TENANT_ID is explicitly set (non-empty)
  if [[ -n "${TENANT_ID}" ]]; then
    curl_args+=(-H "X-Tenant-Id: ${TENANT_ID}")
  fi

  # Add extra headers from caller
  for h in ${hdrs[@]+"${hdrs[@]}"}; do
    curl_args+=(-H "$h")
  done

  curl_args+=(-d "$data")

  curl "${curl_args[@]}" || true

  local code selected body
  code="$(awk 'BEGIN{c=""} /^HTTP\//{c=$2} END{print c}' "$headers" | tail -n1)"
  selected="$(awk -F': ' 'tolower($1)=="x-selected-model"{print $2}' "$headers" | tr -d '\r' | tail -n1)"
  body="$(cat "$resp")"

  rm -f "$resp" "$headers"
  echo "${code}|${selected}|${body}"
}

split_out() {
  local out="$1"
  CODE="${out%%|*}"
  local rest="${out#*|}"
  SEL="${rest%%|*}"
  BODY="${rest#*|}"
}

debug_dump() {
  local label="$1"
  echo "---- debug: ${label} ----" >&2
  echo "HTTP=${CODE:-<empty>}" >&2
  echo "X-Selected-Model=${SEL:-<empty>}" >&2
  echo "Body=${BODY:-<empty>}" >&2
  echo "TENANT_ID=${TENANT_ID:-<empty>}" >&2
  echo "-------------------------" >&2
}

assert_code() {
  local got="$1" want="$2" label="$3"
  if [[ "$got" != "$want" ]]; then
    debug_dump "$label"
    fail "${label}: expected HTTP ${want}, got ${got:-<empty>}"
  fi
  ok "${label}: HTTP ${want}"
}

assert_selected() {
  local got="$1" want="$2" label="$3"
  if [[ "$got" != "$want" ]]; then
    debug_dump "$label"
    fail "${label}: expected X-Selected-Model=${want}, got ${got:-<empty>}"
  fi
  ok "${label}: selected=${want}"
}

echo "=== Router smoke tests ==="
echo "BASE_URL=${BASE_URL}"
echo "TENANT_ID=${TENANT_ID:-<empty>}"
echo

# Sanity: Good key must work
{
  out="$(req '{"messages":[{"role":"user","content":"sanity auth good key"}]}')"
  split_out "$out"
  assert_code "$CODE" "200" "auth valid key sanity"
  ok "auth valid key sanity: selected=${SEL:-<empty>}"
}

# 0) Invalid key should 401 (override header)
{
  out="$(req -H "X-API-Key: bad" '{"messages":[{"role":"user","content":"bad key"}]}')"
  split_out "$out"
  assert_code "$CODE" "401" "auth invalid key"
}

# 1) Body model chooses that model (if allowed)
{
  out="$(req '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"test body model"}]}')"
  split_out "$out"
  assert_code "$CODE" "200" "body model"
  assert_selected "$SEL" "gpt-4o-mini" "body model"
}

# 2) Header X-Model chooses that model (if allowed)
{
  out="$(req -H "X-Model: gpt-4o-mini" '{"messages":[{"role":"user","content":"test header model"}]}')"
  split_out "$out"
  assert_code "$CODE" "200" "header model"
  assert_selected "$SEL" "gpt-4o-mini" "header model"
}

# 3) Header vs Body precedence: header wins (but fallback may change selected model if header model fails upstream)
{
  out="$(req -H "X-Model: claude-sonnet-4-6" '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"test precedence"}]}')"
  split_out "$out"
  assert_code "$CODE" "200" "precedence header>body"
  if [[ "$SEL" == "claude-sonnet-4-6" ]]; then
    ok "precedence header>body: selected=claude-sonnet-4-6"
  else
    note "precedence header>body: selected=${SEL} (posible fallback si Claude falló upstream)"
  fi
}

# 4) Route group cheap + X-Model gpt-4o-mini
{
  out="$(req -H "X-Route-Group: cheap" -H "X-Model: gpt-4o-mini" '{"messages":[{"role":"user","content":"test group+model"}]}')"
  split_out "$out"
  assert_code "$CODE" "200" "route_group cheap + model"
  assert_selected "$SEL" "gpt-4o-mini" "route_group cheap + model"
}

# 5) Route group cheap without model (can be either)
{
  out="$(req -H "X-Route-Group: cheap" '{"messages":[{"role":"user","content":"test group only"}]}')"
  split_out "$out"
  assert_code "$CODE" "200" "route_group cheap only"
  if [[ "$SEL" != "gpt-4o-mini" && "$SEL" != "claude-sonnet-4-6" ]]; then
    debug_dump "route_group cheap only"
    fail "route_group cheap only: unexpected selected model: ${SEL}"
  fi
  ok "route_group cheap only: selected=${SEL}"
}

# 6) Optional debug failover headers (if supported)
{
  out="$(req -H "X-Model: gpt-4o-mini" -H "X-Debug-Fail-Model: gpt-4o-mini" -H "X-Debug-Fail-Status: 500" \
    '{"messages":[{"role":"user","content":"test debug failover"}]}')"
  split_out "$out"
  if [[ "$CODE" == "200" ]]; then
    ok "debug failover supported: selected=${SEL}"
  else
    ok "debug failover skipped/disabled (HTTP ${CODE})"
  fi
}

echo
ok "All smoke tests completed."