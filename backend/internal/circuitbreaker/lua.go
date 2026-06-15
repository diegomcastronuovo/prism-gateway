package circuitbreaker

// luaCBAllow is the Lua script for the Allow gate check.
//
// KEYS[1] = {provider}:state  (Redis hash: state, open_until, inflight, successes)
// ARGV[1] = now               (Unix seconds, int)
// ARGV[2] = half_open_max_inflight (int)
//
// Returns: {allowed(0/1), state_string, is_probe(0/1)}
const luaCBAllow = `
local state_key = KEYS[1]
local now = tonumber(ARGV[1])
local max_inflight = tonumber(ARGV[2])

local data = redis.call('HMGET', state_key, 'state', 'open_until', 'inflight')
local state = data[1] or 'closed'
local open_until = tonumber(data[2]) or 0
local inflight = tonumber(data[3]) or 0

if state == 'open' then
    if now < open_until then
        return {0, 'open', 0}
    end
    -- Cooldown expired: transition to half_open
    redis.call('HMSET', state_key, 'state', 'half_open', 'inflight', 0, 'successes', 0)
    redis.call('PERSIST', state_key)
    state = 'half_open'
    inflight = 0
end

if state == 'half_open' then
    if inflight >= max_inflight then
        return {0, 'half_open', 0}
    end
    redis.call('HINCRBY', state_key, 'inflight', 1)
    return {1, 'half_open', 1}
end

-- closed (or nil/unknown) -> allow
return {1, 'closed', 0}
`

// luaCBReport is the Lua script for recording the outcome of an upstream call.
//
// KEYS[1] = {provider}:state           (state hash)
// KEYS[2] = {provider}:win:{bucket_ts} (current sliding-window bucket)
// ARGV[1]  = now               (Unix seconds, int)
// ARGV[2]  = outcome           (0=success, 1=failure)
// ARGV[3]  = is_probe          (0/1)
// ARGV[4]  = cooldown          (seconds, int)
// ARGV[5]  = successes_to_close (int)
// ARGV[6]  = bucket_ttl        (seconds, int)
// ARGV[7]  = window_buckets    (number of buckets spanning the full window, int)
// ARGV[8]  = bucket_size       (seconds per bucket, int)
// ARGV[9]  = min_requests      (int)
// ARGV[10] = failure_threshold (float, e.g. "0.5")
// ARGV[11] = base_key          (prefix + "{" + provider + "}", for building window keys)
//
// Returns: {transitioned(0/1), from_state, to_state, reason}
const luaCBReport = `
local state_key = KEYS[1]
local win_key   = KEYS[2]
local now       = tonumber(ARGV[1])
local outcome   = tonumber(ARGV[2])  -- 0=success, 1=failure
local is_probe  = tonumber(ARGV[3])
local cooldown  = tonumber(ARGV[4])
local successes_to_close = tonumber(ARGV[5])
local bucket_ttl    = tonumber(ARGV[6])
local window_buckets = tonumber(ARGV[7])
local bucket_size   = tonumber(ARGV[8])
local min_requests  = tonumber(ARGV[9])
local failure_threshold = tonumber(ARGV[10])
local base_key = ARGV[11]

local state_data = redis.call('HMGET', state_key, 'state')
local state = state_data[1] or 'closed'

-- 1. Decrement inflight if this was a probe
if is_probe == 1 then
    redis.call('HINCRBY', state_key, 'inflight', -1)
end

-- 2. Update sliding-window bucket
if outcome == 0 then
    redis.call('HINCRBY', win_key, 'ok', 1)
else
    redis.call('HINCRBY', win_key, 'fail', 1)
end
redis.call('EXPIRE', win_key, bucket_ttl)

-- 3. Half-open state transitions
if state == 'half_open' then
    if outcome == 0 then
        local successes = redis.call('HINCRBY', state_key, 'successes', 1)
        if successes >= successes_to_close then
            redis.call('DEL', state_key)
            return {1, 'half_open', 'closed', 'probe_success'}
        end
        return {0, 'half_open', 'half_open', ''}
    else
        local open_until = now + cooldown
        redis.call('HMSET', state_key, 'state', 'open', 'open_until', open_until, 'inflight', 0, 'successes', 0)
        return {1, 'half_open', 'open', 'half_open_failure'}
    end
end

-- 4. Closed state: check failure rate on failures only
if state == 'closed' and outcome == 1 then
    local current_bucket_ts = math.floor(now / bucket_size) * bucket_size
    local total_ok   = 0
    local total_fail = 0
    for i = 0, window_buckets - 1 do
        local ts   = current_bucket_ts - (i * bucket_size)
        local bkey = base_key .. ':win:' .. ts
        local bdata = redis.call('HMGET', bkey, 'ok', 'fail')
        total_ok   = total_ok   + (tonumber(bdata[1]) or 0)
        total_fail = total_fail + (tonumber(bdata[2]) or 0)
    end
    local total = total_ok + total_fail
    if total >= min_requests then
        local fail_ratio = total_fail / total
        if fail_ratio >= failure_threshold then
            local open_until = now + cooldown
            redis.call('HMSET', state_key, 'state', 'open', 'open_until', open_until, 'inflight', 0, 'successes', 0)
            return {1, 'closed', 'open', 'failure_rate'}
        end
    end
end

-- 5. No transition
return {0, state, state, ''}
`
