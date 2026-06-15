#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
API_KEY="${API_KEY:-}"
TENANT_ID="${TENANT_ID:-default}"

MODEL_OK_BODY="${MODEL_OK_BODY:-gpt-4o-mini}"
MODEL_CHEAP_ALT="${MODEL_CHEAP_ALT:-claude-sonnet-4-6}"
MODEL_PREMIUM="${MODEL_PREMIUM:-claude-sonnet-4-6}"
MODEL_FORBIDDEN="${MODEL_FORBIDDEN:-grok-4}"

ALLFAILED_HEADER_MODEL="${ALLFAILED_HEADER_MODEL:-claude-sonnet-4-6}"
ALLFAILED_ROUTE_GROUP="${ALLFAILED_ROUTE_GROUP:-}" # opcional

fail() { echo "❌ $*" >&2; exit 1; }
ok()   { echo "✅ $*"; }
note() { echo "ℹ️  $*"; }
need() { command -v "$1" >/dev/null 2>&1 || fail "Falta '$1' en PATH"; }

need curl; need awk; need sed; need grep

if [[ -z "${API_KEY}" ]]; then
  fail "Seteá API_KEY. Ej: API_KEY='rk_live_...' ./scripts/route_test.sh"
fi

# req [-H "K: V"]... -- '{"json"}'
# prints: "<code>|<x-selected-model>|<x-mock-response>|<body>"
req() {
  local -a hdrs=()

  while [[ "${1:-}" == "-H" ]]; do
    hdrs+=(-H "$2")
    shift 2
  done

  [[ "${1:-}" == "--" ]] || fail "Uso: req [-H ...] -- '{json}'"
  shift
  local data="${1:-{}}"

  local resp headers
  resp="$(mktemp)"
  headers="$(mktemp)"

  # Armamos args como array (evita unbound/quoting issues)
  local -a curl_args=(
    -sS
    -D "$headers"
    -o "$resp"
    -X POST "${BASE_URL}/v1/chat/completions"
    -H "Content-Type: application/json"
    -H "X-API-Key: ${API_KEY}"
    -H "X-Tenant-Id: ${TENANT_ID}"
  )

  # Solo agregamos headers extra si hay
  if ((${#hdrs[@]} > 0)); then
    curl_args+=("${hdrs[@]}")
  fi

  curl_args+=(-d "$data")

  curl "${curl_args[@]}" || true

  local code selected mock body
  code="$(awk 'BEGIN{c=""} /^HTTP\//{c=$2} END{print c}' "$headers" | tail -n1)"
  selected="$(awk -F': ' 'tolower($1)=="x-selected-model"{print $2}' "$headers" | tr -d '\r' | tail -n1)"
  mock="$(awk -F': ' 'tolower($1)=="x-mock-response"{print $2}' "$headers" | tr -d '\r' | tail -n1)"
  body="$(cat "$resp")"

  rm -f "$resp" "$headers"
  echo "${code}|${selected}|${mock}|${body}"
}

split_out() {
  local out="$1"
  CODE="${out%%|*}"
  local rest="${out#*|}"
  SEL="${rest%%|*}"
  rest="${rest#*|}"
  MOCK="${rest%%|*}"
  BODY="${rest#*|}"
}

assert_code() {
  local got="$1" want="$2" label="$3"
  if [[ "$got" != "$want" ]]; then
    echo "---- debug: ${label} ----" >&2
    echo "HTTP=${got:-<empty>}" >&2
    echo "X-Selected-Model=${SEL:-<empty>}" >&2
    echo "X-Mock-Response=${MOCK:-<empty>}" >&2
    echo "Body=${BODY:-<empty>}" >&2
    echo "-------------------------" >&2
    fail "${label}: expected HTTP ${want}, got ${got:-<empty>}"
  fi
  ok "${label}: HTTP ${want}"
}

assert_selected() {
  local got="$1" want="$2" label="$3"
  if [[ "$got" != "$want" ]]; then
    echo "---- debug: ${label} ----" >&2
    echo "HTTP=${CODE:-<empty>}" >&2
    echo "X-Selected-Model=${got:-<empty>}" >&2
    echo "X-Mock-Response=${MOCK:-<empty>}" >&2
    echo "Body=${BODY:-<empty>}" >&2
    echo "-------------------------" >&2
    fail "${label}: expected X-Selected-Model=${want}, got ${got:-<empty>}"
  fi
  ok "${label}: selected=${want}"
}

assert_body_has() {
  local hay="$1" needle="$2" label="$3"
  echo "$hay" | grep -q "$needle" || fail "${label}: body no contiene '${needle}'. Body=${hay}"
  ok "${label}: body contiene '${needle}'"
}

echo "=== Routing V2 route_test ==="
echo "BASE_URL=${BASE_URL}"
echo "TENANT_ID=${TENANT_ID}"
echo

# 0) Sanity auth OK
{
  out="$(req -- '{"messages":[{"role":"user","content":"sanity"}]}')"
  split_out "$out"
  assert_code "$CODE" "200" "sanity auth"
  note "sanity: selected=${SEL:-<empty>} mock=${MOCK:-<empty>}"
}

# 1) Precedence: header vs body
{
  out="$(req -H "X-Model: ${MODEL_CHEAP_ALT}" -- "{\"model\":\"${MODEL_OK_BODY}\",\"messages\":[{\"role\":\"user\",\"content\":\"precedence header vs body\"}]}")"
  split_out "$out"
  assert_code "$CODE" "200" "precedence header>body"
  if [[ "$SEL" == "${MODEL_CHEAP_ALT}" ]]; then
    ok "precedence: selected=${SEL} (header wins)"
  else
    note "precedence: selected=${SEL} (posible fallback por fallo upstream del header model)"
  fi
}

# 2) Route group conflict: premium + gpt-4o-mini => 400
{
  out="$(req -H "X-Route-Group: premium" -H "X-Model: ${MODEL_OK_BODY}" -- '{"messages":[{"role":"user","content":"group vs model conflict"}]}')"
  split_out "$out"
  assert_code "$CODE" "400" "route_group premium + model not in group"
  assert_body_has "$BODY" "not in route group" "route_group premium + wrong model"
}

# 3) Route group conflict: premium + body.model gpt-4o-mini => 400
{
  out="$(req -H "X-Route-Group: premium" -- "{\"model\":\"${MODEL_OK_BODY}\",\"messages\":[{\"role\":\"user\",\"content\":\"group vs body conflict\"}]}")"
  split_out "$out"
  assert_code "$CODE" "400" "route_group premium + body.model not in group"
  assert_body_has "$BODY" "not in route group" "route_group premium + wrong body.model"
}

# 4) Premium only => claude
{
  out="$(req -H "X-Route-Group: premium" -- '{"messages":[{"role":"user","content":"premium group only"}]}')"
  split_out "$out"
  assert_code "$CODE" "200" "route_group premium only"
  assert_selected "$SEL" "${MODEL_PREMIUM}" "route_group premium only"
}

# 5) Cheap + claude => 200
{
  out="$(req -H "X-Route-Group: cheap" -H "X-Model: ${MODEL_CHEAP_ALT}" -- '{"messages":[{"role":"user","content":"cheap + claude"}]}')"
  split_out "$out"
  assert_code "$CODE" "200" "route_group cheap + claude"
  assert_selected "$SEL" "${MODEL_CHEAP_ALT}" "route_group cheap + claude"
}

# 6) Unknown route group => 400
{
  out="$(req -H "X-Route-Group: does_not_exist" -- '{"messages":[{"role":"user","content":"unknown group"}]}')"
  split_out "$out"
  assert_code "$CODE" "400" "unknown route group"
  assert_body_has "$BODY" "not found" "unknown route group body"
}

# 7) Model not allowed => 403
{
  out="$(req -H "X-Model: ${MODEL_FORBIDDEN}" -- '{"messages":[{"role":"user","content":"model not allowed"}]}')"
  split_out "$out"
  assert_code "$CODE" "403" "model not allowed"
  assert_body_has "$BODY" "not allowed" "model not allowed body"
}

# 8) Deterministic all_attempts_failed repro (depende de mocks)
{
  local -a extra_headers=()
  extra_headers+=(-H "X-Model: ${ALLFAILED_HEADER_MODEL}")
  if [[ -n "${ALLFAILED_ROUTE_GROUP}" ]]; then
    extra_headers+=(-H "X-Route-Group: ${ALLFAILED_ROUTE_GROUP}")
  fi

  out="$(req "${extra_headers[@]}" -- '{"messages":[{"role":"user","content":"all_attempts_failed deterministic test"}]}')"
  split_out "$out"

  if [[ "$CODE" == "500" || "$CODE" == "502" ]]; then
    ok "all_attempts_failed repro: HTTP ${CODE}"
    note "selected=${SEL:-<empty>} mock=${MOCK:-<empty>}"
    note "Check DB:"
    cat <<'EOF'
docker exec -it router-postgres psql -U router -d router -c "
select request_id, attempt, model, status, error_type, decision_reason,
       (decision_snapshot is null) as snapshot_is_null
from request_log
order by ts desc
limit 6;"
EOF
  else
    echo "---- debug: all_attempts_failed repro ----" >&2
    echo "HTTP=${CODE}" >&2
    echo "X-Selected-Model=${SEL:-<empty>}" >&2
    echo "X-Mock-Response=${MOCK:-<empty>}" >&2
    echo "Body=${BODY:-<empty>}" >&2
    echo "----------------------------------------" >&2
    fail "all_attempts_failed repro: esperaba HTTP 500/502; obtuve ${CODE}"
  fi
}

echo
ok "Routing V2 route_test completed."