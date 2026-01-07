# Quota System Redesign Plan v2

## Overview

Redesign quota management using **Strategy Pattern** with **lock-free architecture** for provider-specific selection strategies.

## Current Architecture

### Existing Components (Keep)
```go
// manager.go - Single Selector interface
type Selector interface {
    Pick(ctx context.Context, provider, model string, opts Options, auths []*Auth) (*Auth, error)
}

// quota_manager.go - Sharded state management
type QuotaManager struct {
    shards [32]*quotaShard  // Each shard: sync.RWMutex + map[string]*AuthQuotaState
    sticky *StickyStore
}

// AuthQuotaState - Already uses atomics
type AuthQuotaState struct {
    ActiveRequests  atomic.Int64
    CooldownUntil   atomic.Int64
    TotalTokensUsed atomic.Int64
    LearnedLimit    atomic.Int64
    // ...
}
```

### Current Problems

1. **One-size-fits-all**: Same selection algorithm for ALL providers despite different quota models
2. **Lock contention**: Sharded RWMutex still causes contention at high concurrency
3. **No real quota API**: Only learns limits from 429 errors, doesn't use provider APIs
4. **Sync quota fetch**: Would block hot path if integrated naively

## Provider Quota Models

| Provider | Quota Type | Window | Data Source | Strategy Needed |
|----------|------------|--------|-------------|-----------------|
| Antigravity | Tokens | 5h | **Real API** (`remainingFraction`) | Highest Remaining |
| Claude | Tokens | 5h | Learned from 429 | Lowest Usage % |
| Copilot | Requests | 24h | Count requests | Request Counter |
| Gemini | RPM | 1 min | Rate limit | Token Bucket |

## Target Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      QuotaManager                           │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  strategies map[string]ProviderStrategy             │   │
│  │  shards [32]*quotaShard (unchanged)                 │   │
│  │  sticky *StickyStore (unchanged)                    │   │
│  └─────────────────────────────────────────────────────┘   │
│                          │                                  │
│                   Pick() delegates to                       │
│                          ▼                                  │
│  ┌──────────────────────────────────────────────────────┐  │
│  │            ProviderStrategy Interface                 │  │
│  │  Score(auth, state, config) int64  // Lock-free      │  │
│  │  OnQuotaHit(state, cooldown)                         │  │
│  │  RecordUsage(state, tokens)                          │  │
│  └──────────────────────────────────────────────────────┘  │
│                          │                                  │
│     ┌────────────────────┼────────────────────┐            │
│     ▼                    ▼                    ▼            │
│ ┌───────────┐      ┌──────────┐         ┌─────────┐       │
│ │Antigravity│      │ Claude   │         │ Gemini  │       │
│ │Strategy   │      │ Strategy │         │ Strategy│       │
│ │           │      │          │         │         │       │
│ │ Real API  │      │ Learned  │         │ Token   │       │
│ │ + atomic  │      │ Limits   │         │ Bucket  │       │
│ │ Pointer   │      │          │         │         │       │
│ └───────────┘      └──────────┘         └─────────┘       │
└─────────────────────────────────────────────────────────────┘
```

## Implementation Tasks

### Phase 1: Core Interfaces (1h)

**File**: `internal/provider/provider_strategy.go`

```go
// ProviderStrategy defines provider-specific selection logic.
// All methods MUST be lock-free and O(1).
type ProviderStrategy interface {
    // Score returns selection priority (lower = better).
    // Must be pure and lock-free - only read from atomic state.
    Score(auth *Auth, state *AuthQuotaState, config *ProviderQuotaConfig) int64
    
    // OnQuotaHit handles 429 errors with provider-specific logic.
    OnQuotaHit(state *AuthQuotaState, cooldown *time.Duration)
    
    // RecordUsage tracks tokens/requests for selection decisions.
    RecordUsage(state *AuthQuotaState, tokens int64)
}

// BackgroundRefresher is optionally implemented by strategies 
// that need to fetch real quota data from provider APIs.
type BackgroundRefresher interface {
    ProviderStrategy
    // StartRefresh begins background quota polling for this auth.
    // Returns a channel that receives updated quota snapshots.
    StartRefresh(ctx context.Context, auth *Auth) <-chan *RealQuotaSnapshot
}

// RealQuotaSnapshot holds data from real provider APIs.
type RealQuotaSnapshot struct {
    RemainingFraction float64   // 0.0-1.0 (from Antigravity API)
    RemainingTokens   int64     // Absolute remaining
    WindowResetAt     time.Time // When quota window resets
    FetchedAt         time.Time // When this was fetched
}

// DefaultStrategy is used for providers without specific strategy.
type DefaultStrategy struct{}

func (s *DefaultStrategy) Score(auth *Auth, state *AuthQuotaState, config *ProviderQuotaConfig) int64 {
    if state == nil {
        return 0
    }
    var priority int64
    priority += state.ActiveRequests.Load() * 1000
    
    limit := state.LearnedLimit.Load()
    if limit <= 0 {
        limit = config.EstimatedLimit
    }
    if limit > 0 {
        usagePercent := float64(state.TotalTokensUsed.Load()) / float64(limit)
        priority += int64(usagePercent * 500)
    }
    return priority
}

func (s *DefaultStrategy) OnQuotaHit(state *AuthQuotaState, cooldown *time.Duration) {
    // Use default cooldown logic
}

func (s *DefaultStrategy) RecordUsage(state *AuthQuotaState, tokens int64) {
    if state != nil && tokens > 0 {
        state.TotalTokensUsed.Add(tokens)
    }
}
```

### Phase 2: AuthQuotaState Enhancement (30m)

**File**: `internal/provider/quota_manager.go` (modify)

Add `atomic.Pointer` for lock-free real quota updates:

```go
type AuthQuotaState struct {
    // Existing atomic fields (unchanged)
    ActiveRequests  atomic.Int64
    CooldownUntil   atomic.Int64
    TotalTokensUsed atomic.Int64
    LastExhaustedAt atomic.Int64
    LearnedLimit    atomic.Int64
    LearnedCooldown atomic.Int64
    
    // NEW: Lock-free real quota from background refresh
    RealQuota atomic.Pointer[RealQuotaSnapshot]
}

// GetRealQuota returns the latest snapshot (nil if not available)
func (s *AuthQuotaState) GetRealQuota() *RealQuotaSnapshot {
    return s.RealQuota.Load()
}

// SetRealQuota atomically updates the real quota snapshot
func (s *AuthQuotaState) SetRealQuota(snapshot *RealQuotaSnapshot) {
    s.RealQuota.Store(snapshot)
}
```

### Phase 3: AntigravityStrategy (2h)

**File**: `internal/provider/strategy_antigravity.go`

Features:
- Uses REAL quota data from Antigravity API when available
- Falls back to learned limits if real quota is stale (>5 min)
- Implements `BackgroundRefresher` for async quota polling
- 5% jitter tolerance for load balancing

```go
type AntigravityStrategy struct{}

func (s *AntigravityStrategy) Score(auth *Auth, state *AuthQuotaState, _ *ProviderQuotaConfig) int64 {
    if state == nil {
        return 0
    }
    
    var priority int64
    priority += state.ActiveRequests.Load() * 1000
    
    // Prefer real quota when available (lock-free read)
    if real := state.GetRealQuota(); real != nil && time.Since(real.FetchedAt) < 5*time.Minute {
        // Lower remaining = higher priority (worse)
        priority += int64((1.0 - real.RemainingFraction) * 800)
        return priority
    }
    
    // Fallback to learned limits
    limit := state.LearnedLimit.Load()
    if limit > 0 {
        used := state.TotalTokensUsed.Load()
        priority += int64(float64(used) / float64(limit) * 500)
    }
    
    return priority
}

func (s *AntigravityStrategy) OnQuotaHit(state *AuthQuotaState, cooldown *time.Duration) {
    if state == nil {
        return
    }
    now := time.Now()
    state.SetLastExhaustedAt(now)
    
    // Update learned limit
    tokensUsed := state.TotalTokensUsed.Load()
    for {
        currentLimit := state.LearnedLimit.Load()
        if tokensUsed <= currentLimit {
            break
        }
        if state.LearnedLimit.CompareAndSwap(currentLimit, tokensUsed) {
            break
        }
    }
    
    // Set cooldown
    if cooldown != nil && *cooldown > 0 {
        state.SetCooldownUntil(now.Add(*cooldown))
    } else {
        state.SetCooldownUntil(now.Add(5 * time.Hour))
    }
    
    state.TotalTokensUsed.Store(0)
}

func (s *AntigravityStrategy) RecordUsage(state *AuthQuotaState, tokens int64) {
    if state != nil && tokens > 0 {
        state.TotalTokensUsed.Add(tokens)
    }
}

// Implements BackgroundRefresher for real quota polling
func (s *AntigravityStrategy) StartRefresh(ctx context.Context, auth *Auth) <-chan *RealQuotaSnapshot {
    ch := make(chan *RealQuotaSnapshot, 1)
    go func() {
        defer close(ch)
        
        // Initial jitter to avoid thundering herd
        jitter := time.Duration(rand.Float64() * float64(30*time.Second))
        time.Sleep(jitter)
        
        ticker := time.NewTicker(2 * time.Minute)
        defer ticker.Stop()
        
        // Fetch immediately on start
        if snapshot := fetchAntigravityQuota(ctx, auth); snapshot != nil {
            select {
            case ch <- snapshot:
            default:
            }
        }
        
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                if snapshot := fetchAntigravityQuota(ctx, auth); snapshot != nil {
                    select {
                    case ch <- snapshot:
                    default:
                    }
                }
            }
        }
    }()
    return ch
}
```

### Phase 4: ClaudeStrategy (1h)

**File**: `internal/provider/strategy_claude.go`

Features:
- Uses `LearnedLimit` and `TotalTokensUsed` from 429 responses
- All atomic operations (lock-free)
- Selection by lowest usage percentage

```go
type ClaudeStrategy struct{}

func (s *ClaudeStrategy) Score(auth *Auth, state *AuthQuotaState, config *ProviderQuotaConfig) int64 {
    if state == nil {
        return 0
    }
    
    var priority int64
    priority += state.ActiveRequests.Load() * 1000
    
    limit := state.LearnedLimit.Load()
    if limit <= 0 {
        limit = config.EstimatedLimit
    }
    if limit > 0 {
        usagePercent := float64(state.TotalTokensUsed.Load()) / float64(limit)
        priority += int64(usagePercent * 500)
    }
    
    return priority
}

func (s *ClaudeStrategy) OnQuotaHit(state *AuthQuotaState, cooldown *time.Duration) {
    // Same as AntigravityStrategy.OnQuotaHit
}

func (s *ClaudeStrategy) RecordUsage(state *AuthQuotaState, tokens int64) {
    if state != nil && tokens > 0 {
        state.TotalTokensUsed.Add(tokens)
    }
}
```

### Phase 5: CopilotStrategy (1h)

**File**: `internal/provider/strategy_copilot.go`

Features:
- Request counter (not token-based)
- 24h window with atomic CAS reset
- Lock-free via `atomic.Int64`

```go
type CopilotStrategy struct {
    counters sync.Map // map[authID]*RequestCounter
}

type RequestCounter struct {
    count     atomic.Int64
    windowEnd atomic.Int64 // Unix nano timestamp
}

func (s *CopilotStrategy) Score(auth *Auth, state *AuthQuotaState, config *ProviderQuotaConfig) int64 {
    counter := s.getOrCreateCounter(auth.ID, config)
    
    var priority int64
    if state != nil {
        priority += state.ActiveRequests.Load() * 1000
    }
    
    // Check if window needs reset
    now := time.Now().UnixNano()
    windowEnd := counter.windowEnd.Load()
    if now > windowEnd {
        // Window expired, reset counter
        newWindowEnd := time.Now().Add(config.WindowDuration).UnixNano()
        if counter.windowEnd.CompareAndSwap(windowEnd, newWindowEnd) {
            counter.count.Store(0)
        }
    }
    
    count := counter.count.Load()
    if config.EstimatedLimit > 0 {
        priority += int64(float64(count) / float64(config.EstimatedLimit) * 600)
    }
    
    return priority
}

func (s *CopilotStrategy) getOrCreateCounter(authID string, config *ProviderQuotaConfig) *RequestCounter {
    if v, ok := s.counters.Load(authID); ok {
        return v.(*RequestCounter)
    }
    counter := &RequestCounter{}
    counter.windowEnd.Store(time.Now().Add(config.WindowDuration).UnixNano())
    actual, _ := s.counters.LoadOrStore(authID, counter)
    return actual.(*RequestCounter)
}

func (s *CopilotStrategy) OnQuotaHit(state *AuthQuotaState, cooldown *time.Duration) {
    // Standard cooldown handling
}

func (s *CopilotStrategy) RecordUsage(state *AuthQuotaState, _ int64) {
    // Copilot counts requests, not tokens - handled in Score via counter
}
```

### Phase 6: GeminiStrategy (1.5h)

**File**: `internal/provider/strategy_gemini.go`

Features:
- Token bucket algorithm for RPM rate limiting
- Per-auth rate limiting
- Lock-free via atomic operations
- Configurable RPM limit (default: 60)

```go
type GeminiStrategy struct {
    buckets sync.Map // map[authID]*TokenBucket
}

type TokenBucket struct {
    tokens    atomic.Int64
    lastFill  atomic.Int64 // Unix nano timestamp
    capacity  int64
    fillRate  float64 // tokens per nanosecond
}

func (s *GeminiStrategy) Score(auth *Auth, state *AuthQuotaState, config *ProviderQuotaConfig) int64 {
    bucket := s.getOrCreateBucket(auth.ID, config)
    
    var priority int64
    if state != nil {
        priority += state.ActiveRequests.Load() * 1000
    }
    
    // Refill bucket based on elapsed time
    available := bucket.availableTokens()
    
    // Fewer available tokens = higher priority (worse)
    if bucket.capacity > 0 {
        priority += int64((1.0 - float64(available)/float64(bucket.capacity)) * 600)
    }
    
    return priority
}

func (b *TokenBucket) availableTokens() int64 {
    now := time.Now().UnixNano()
    last := b.lastFill.Load()
    elapsed := float64(now - last)
    
    current := b.tokens.Load()
    refilled := current + int64(elapsed*b.fillRate)
    if refilled > b.capacity {
        refilled = b.capacity
    }
    
    // Try to update lastFill (CAS for correctness)
    if b.lastFill.CompareAndSwap(last, now) {
        b.tokens.Store(refilled)
    }
    
    return refilled
}

func (s *GeminiStrategy) getOrCreateBucket(authID string, config *ProviderQuotaConfig) *TokenBucket {
    if v, ok := s.buckets.Load(authID); ok {
        return v.(*TokenBucket)
    }
    
    capacity := config.EstimatedLimit // e.g., 60 RPM
    fillRate := float64(capacity) / float64(time.Minute) // tokens per nanosecond
    
    bucket := &TokenBucket{
        capacity: capacity,
        fillRate: fillRate,
    }
    bucket.tokens.Store(capacity)
    bucket.lastFill.Store(time.Now().UnixNano())
    
    actual, _ := s.buckets.LoadOrStore(authID, bucket)
    return actual.(*TokenBucket)
}

func (s *GeminiStrategy) OnQuotaHit(state *AuthQuotaState, cooldown *time.Duration) {
    // Standard cooldown handling
}

func (s *GeminiStrategy) RecordUsage(state *AuthQuotaState, _ int64) {
    // Gemini counts requests via bucket, not tokens
}
```

### Phase 7: QuotaManager Integration (2h)

**File**: `internal/provider/quota_manager.go` (modify)

Changes:
1. Add `strategies map[string]ProviderStrategy` field
2. Register strategies in `NewQuotaManager()`
3. Update `Pick()` to use `selectWithStrategy()`
4. Add `RegisterAuth()` for background refresh lifecycle

```go
type QuotaManager struct {
    shards   [numQuotaShards]*quotaShard
    sticky   *StickyStore
    strategies map[string]ProviderStrategy  // NEW
    
    stopChan chan struct{}
    stopOnce sync.Once
    wg       sync.WaitGroup
    
    // Background refresh tracking
    refreshMu      sync.Mutex
    refreshCancels map[string]context.CancelFunc // authID -> cancel func
}

func NewQuotaManager() *QuotaManager {
    m := &QuotaManager{
        sticky:         NewStickyStore(),
        stopChan:       make(chan struct{}),
        strategies:     make(map[string]ProviderStrategy),
        refreshCancels: make(map[string]context.CancelFunc),
    }
    for i := range m.shards {
        m.shards[i] = &quotaShard{
            states: make(map[string]*AuthQuotaState),
        }
    }
    
    // Register provider-specific strategies
    m.strategies["antigravity"] = &AntigravityStrategy{}
    m.strategies["claude"] = &ClaudeStrategy{}
    m.strategies["copilot"] = &CopilotStrategy{}
    m.strategies["gemini"] = &GeminiStrategy{}
    
    return m
}

func (m *QuotaManager) getStrategy(provider string) ProviderStrategy {
    if s, ok := m.strategies[provider]; ok {
        return s
    }
    return &DefaultStrategy{}
}

func (m *QuotaManager) Pick(ctx context.Context, provider, model string, opts Options, auths []*Auth) (*Auth, error) {
    if len(auths) == 0 {
        return nil, &Error{Code: "auth_not_found", Message: "no auth candidates"}
    }

    now := time.Now()
    config := GetProviderQuotaConfig(provider)
    strategy := m.getStrategy(provider)

    available := m.filterAvailable(auths, model, now)
    if len(available) == 0 {
        return nil, m.buildRetryError(auths, now)
    }

    if len(available) == 1 {
        m.incrementActive(available[0].ID)
        return available[0], nil
    }

    // Sticky session check (unchanged)
    if config.StickyEnabled && !opts.ForceRotate {
        key := provider + ":" + model
        if authID, ok := m.sticky.Get(key); ok {
            for _, auth := range available {
                if auth.ID == authID {
                    m.incrementActive(auth.ID)
                    return auth, nil
                }
            }
        }
    }

    // Use strategy for scoring (lock-free)
    selected := m.selectWithStrategy(available, config, strategy)

    if config.StickyEnabled {
        m.sticky.Set(provider+":"+model, selected.ID)
    }

    m.incrementActive(selected.ID)
    return selected, nil
}

func (m *QuotaManager) selectWithStrategy(auths []*Auth, config *ProviderQuotaConfig, strategy ProviderStrategy) *Auth {
    type scored struct {
        auth     *Auth
        priority int64
    }

    candidates := make([]scored, 0, len(auths))
    for _, auth := range auths {
        state := m.getState(auth.ID)
        priority := strategy.Score(auth, state, config)
        candidates = append(candidates, scored{auth: auth, priority: priority})
    }

    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].priority < candidates[j].priority
    })

    // Top-N randomization for similar scores
    topN := 3
    if len(candidates) < topN {
        topN = len(candidates)
    }

    minPriority := candidates[0].priority
    similarCount := 0
    for i := 0; i < topN; i++ {
        if candidates[i].priority-minPriority < 100 {
            similarCount++
        }
    }

    if similarCount > 1 {
        return candidates[rand.Intn(similarCount)].auth
    }

    return candidates[0].auth
}

// RegisterAuth starts background refresh for providers that support it
func (m *QuotaManager) RegisterAuth(ctx context.Context, auth *Auth) {
    strategy := m.getStrategy(auth.Provider)
    
    refresher, ok := strategy.(BackgroundRefresher)
    if !ok {
        return // No background refresh for this provider
    }

    // Create cancellable context for this auth
    refreshCtx, cancel := context.WithCancel(ctx)
    
    m.refreshMu.Lock()
    // Cancel existing refresh if any
    if oldCancel, exists := m.refreshCancels[auth.ID]; exists {
        oldCancel()
    }
    m.refreshCancels[auth.ID] = cancel
    m.refreshMu.Unlock()

    // Start background refresh goroutine
    ch := refresher.StartRefresh(refreshCtx, auth)
    go func() {
        for snapshot := range ch {
            state := m.getOrCreateState(auth.ID)
            state.SetRealQuota(snapshot) // Lock-free atomic pointer swap
        }
    }()
}

// UnregisterAuth stops background refresh for an auth
func (m *QuotaManager) UnregisterAuth(authID string) {
    m.refreshMu.Lock()
    if cancel, exists := m.refreshCancels[authID]; exists {
        cancel()
        delete(m.refreshCancels, authID)
    }
    m.refreshMu.Unlock()
}
```

### Phase 8: Antigravity Quota Fetcher (2h)

**File**: `internal/provider/antigravity_quota_fetcher.go`

```go
package provider

import (
    "context"
    "net/http"
    "time"
    
    "github.com/nghyane/llm-mux/internal/json"
    "github.com/nghyane/llm-mux/internal/runtime/executor"
)

const (
    antigravityQuotaEndpoint = "https://cloudcode-pa.googleapis.com/v1internal:getQuotaInfo"
    quotaFetchTimeout        = 10 * time.Second
)

// fetchAntigravityQuota retrieves real quota data from Antigravity API
func fetchAntigravityQuota(ctx context.Context, auth *Auth) *RealQuotaSnapshot {
    if auth == nil {
        return nil
    }
    
    accessToken := executor.MetaStringValue(auth.Metadata, "access_token")
    if accessToken == "" {
        return nil
    }
    
    ctx, cancel := context.WithTimeout(ctx, quotaFetchTimeout)
    defer cancel()
    
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, antigravityQuotaEndpoint, nil)
    if err != nil {
        return nil
    }
    
    req.Header.Set("Authorization", "Bearer "+accessToken)
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil
    }
    
    var quotaResp struct {
        RemainingFraction float64 `json:"remainingFraction"`
        ResetTime         string  `json:"resetTime"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&quotaResp); err != nil {
        return nil
    }
    
    snapshot := &RealQuotaSnapshot{
        RemainingFraction: quotaResp.RemainingFraction,
        FetchedAt:         time.Now(),
    }
    
    if quotaResp.ResetTime != "" {
        if t, err := time.Parse(time.RFC3339, quotaResp.ResetTime); err == nil {
            snapshot.WindowResetAt = t
        }
    }
    
    return snapshot
}
```

### Phase 9: Cleanup Old Code (30m)

Remove from `quota_manager.go`:
- `calculatePriority()` method (replaced by strategy.Score)

Keep:
- `filterAvailable()` - still needed
- `buildRetryError()` - still needed
- `RecordRequestStart/End` - update to call strategy.RecordUsage
- `RecordQuotaHit` - update to call strategy.OnQuotaHit

### Phase 10: Tests (3h)

Files:
- `internal/provider/strategy_antigravity_test.go`
- `internal/provider/strategy_claude_test.go`
- `internal/provider/strategy_copilot_test.go`
- `internal/provider/strategy_gemini_test.go`
- `internal/provider/quota_manager_strategy_test.go`

Test cases:
- Strategy scoring with real quota data
- Strategy scoring with learned limits (fallback)
- Concurrent access (race detection)
- Token bucket refill correctness
- Request counter window reset
- Background refresh lifecycle

### Phase 11: Benchmarks (30m)

**File**: `internal/provider/quota_manager_bench_test.go`

```go
func BenchmarkPick_Antigravity_WithRealQuota(b *testing.B)
func BenchmarkPick_Antigravity_FallbackToLearned(b *testing.B)
func BenchmarkPick_Gemini_TokenBucket(b *testing.B)
func BenchmarkPick_Concurrent_1000(b *testing.B)
```

Compare: latency p50/p99, allocations, lock contention

## Lock-Free Design Summary

| Operation | Pattern | Contention |
|-----------|---------|------------|
| Read active requests | `atomic.Int64.Load()` | Zero |
| Read real quota | `atomic.Pointer.Load()` | Zero |
| Update real quota | `atomic.Pointer.Store()` | Zero (background only) |
| Read/update cooldown | `atomic.Int64` + CAS | Zero |
| Token bucket refill | `atomic.Int64` + CAS | Zero |
| Request counter | `atomic.Int64` + CAS | Zero |
| Shard lookup | `RLock` on 1/32 shards | ~3% (state creation only) |

## Data Flow

```
                    HOT PATH (Lock-free, O(N_auths))
                    ================================

  Pick(provider, model, auths)
           │
           ▼
  ┌─────────────────────┐
  │ filterAvailable()   │◄── Atomic reads: CooldownUntil, Disabled
  └─────────────────────┘
           │
           ▼
  ┌─────────────────────┐
  │ selectWithStrategy()│◄── strategy.Score() - ALL lock-free:
  │                     │    • ActiveRequests.Load()
  │                     │    • RealQuota.Load() ← atomic.Pointer
  │                     │    • TotalTokensUsed.Load()
  │                     │    • TokenBucket.availableTokens()
  └─────────────────────┘
           │
           ▼
       Return *Auth


                    BACKGROUND PATH (Async - Non-blocking)
                    ======================================

  Antigravity API ──► StartRefresh() ──► atomic.Pointer.Store()
       │                    │
       └── 2-minute ticker with jitter (per-auth goroutine)
```

## Files Summary

### Files to Create
| File | Description | Lines |
|------|-------------|-------|
| `internal/provider/provider_strategy.go` | Interfaces + DefaultStrategy | ~100 |
| `internal/provider/strategy_antigravity.go` | Real quota + background refresh | ~120 |
| `internal/provider/strategy_claude.go` | Learned limits scoring | ~60 |
| `internal/provider/strategy_copilot.go` | Request counter | ~80 |
| `internal/provider/strategy_gemini.go` | Token bucket | ~100 |
| `internal/provider/antigravity_quota_fetcher.go` | HTTP client for quota API | ~80 |

### Files to Modify
| File | Changes |
|------|---------|
| `internal/provider/quota_manager.go` | Add strategies map, selectWithStrategy(), RegisterAuth() |
| `internal/provider/types.go` | (Optional) No changes needed if RealQuotaSnapshot in strategy file |

### Files to Keep (Unchanged)
| File | Why |
|------|-----|
| `internal/provider/quota_group.go` | Quota group resolvers still needed |
| `internal/provider/quota_config.go` | Provider configs still needed |
| `internal/provider/selector.go` | RoundRobinSelector as fallback |

## Estimated Total Effort

| Phase | Task | Effort |
|-------|------|--------|
| 1 | Core interfaces | 1h |
| 2 | AuthQuotaState enhancement | 30m |
| 3 | AntigravityStrategy | 2h |
| 4 | ClaudeStrategy | 1h |
| 5 | CopilotStrategy | 1h |
| 6 | GeminiStrategy | 1.5h |
| 7 | QuotaManager integration | 2h |
| 8 | Antigravity quota fetcher | 2h |
| 9 | Cleanup old code | 30m |
| 10 | Tests | 3h |
| 11 | Benchmarks | 30m |
| **Total** | | **~15h (~2 days)** |

## Success Criteria

1. All existing tests pass
2. No mutex in hot path except sharded RLocks for state creation
3. Antigravity uses real quota from API when available
4. Each provider uses appropriate selection strategy
5. Benchmarks show reduced lock contention vs current
6. No goroutine leaks (verified with -race)
7. Background refresh handles auth deletion gracefully

## Watch Out For

1. **Goroutine leaks**: Background refresh goroutines must be cancelled on auth deletion. Use `UnregisterAuth()`.

2. **Stale real quota**: Set staleness threshold (5 minutes) - if `FetchedAt` too old, fall back to learned limits.

3. **API rate limiting**: Antigravity quota API may have rate limits. Use per-auth jitter on refresh intervals.

4. **Memory pressure**: `RealQuotaSnapshot` is ~32 bytes per auth. For 1000+ auths, consider pooling.

## Notes

- Antigravity is priority - it has real quota API
- Other providers use learned/estimated limits until they get APIs
- Design allows easy addition of new provider strategies
- All atomic operations use Go 1.19+ `atomic` types (not `sync/atomic` functions)
- Strategy pattern keeps selection logic isolated and testable
