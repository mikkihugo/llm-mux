# Architecture Optimization Plan - llm-mux

## Mục tiêu
1. Remove tất cả deprecated code
2. Consolidate streaming pipeline (giảm từ 4+ channels xuống 2)
3. Sử dụng Go standard patterns (errgroup, sync.Pool, context)
4. Giảm goroutine overhead per request

---

## Phase 1: Remove Deprecated Code (Low Risk)

### 1.1 Provider Layer
| File | Action | Priority |
|------|--------|----------|
| `provider/execution.go` | Remove `wrapStreamForStats` function | HIGH |
| `provider/manager.go` | Remove reference to `wrapStreamForStats` | HIGH |

### 1.2 CLI Legacy Flags
| File | Action | Priority |
|------|--------|----------|
| `cli/legacy.go` | Remove entire file - all flags deprecated | MEDIUM |
| `cli/root.go` | Remove legacy flag handling | MEDIUM |

### 1.3 Usage/Stats Layer
| File | Action | Priority |
|------|--------|----------|
| `usage/backend.go` | Remove deprecated `QueryStats` method | LOW |
| `usage/queries.go` | Remove deprecated query functions | LOW |
| `api/handlers/management/types.go` | Remove `UsageCounters`, `UsageAPIStats`, `UsageAPIModelStats` | LOW |

### 1.4 Translator Layer
| File | Action | Priority |
|------|--------|----------|
| `translator/ir/sse_fast.go` | Remove deprecated `formatResponsesSSE` | LOW |

---

## Phase 2: Consolidate Streaming Pipeline (Medium Risk)

### Current Flow (4 channels, 6+ goroutines per stream):
```
┌──────────────────┐     ┌──────────────────────┐     ┌──────────────────┐
│ RunSSEStream     │────▸│ executeStreamProvider │────▸│ wrapStreamChannel │────▸ Client
│ (executor)       │     │ (provider/manager)    │     │ (api/handler)     │
│ chan(64) + 2 go  │     │ chan(128) + 1 go      │     │ chan(32) + 1 go   │
└──────────────────┘     └──────────────────────┘     └──────────────────┘
```

### Target Flow (2 channels, 3 goroutines per stream):
```
┌──────────────────────────────────────────────────────────────────────────┐
│                        StreamPipeline (streamutil)                        │
│  ┌─────────────┐                                              ┌────────┐ │
│  │ LineScanner │──▸ Process ──▸ Translate ──▸ chan(128) ──▸ │ Client │ │
│  │ (shared     │    (inline)    (inline)                      │        │ │
│  │  watcher)   │                                              └────────┘ │
│  └─────────────┘                                                         │
│  + 1 goroutine (stream processor)                                        │
│  + 1 goroutine (shared idle watcher - amortized)                         │
└──────────────────────────────────────────────────────────────────────────┘
```

### 2.1 Integrate streamutil package ✅
| File | Action |
|------|--------|
| `runtime/executor/streaming_helpers.go` | Use `streamutil.LineScanner` instead of custom `StreamReader` |
| `runtime/executor/stream_reader.go` | Uses shared idle watcher from streamutil |
| All executors | Updated to use shared idle watcher |

### 2.2 Simplify wrapStreamChannel layer ✅
| File | Action |
|------|--------|
| `api/handlers/format/base.go` | Removed SSE error parsing (moved to provider layer), increased buffer to 128 |
| `api/handlers/format/openai/openai_handlers.go` | Direct stream consumption (unchanged) |

---

## Phase 3: Optimize Provider Manager (Medium Risk)

### 3.1 Async Result Processing (DONE)
- ✅ Created `async_result.go` with worker pool
- ✅ Updated `MarkResult` to use async worker
- ✅ Tests passing

### 3.2 Reduce Lock Contention in pickNext (DONE)
| Current | Target |
|---------|--------|
| Double lock pattern (RLock → Unlock → Lock) | Single atomic snapshot |
| Clone inside lock | Clone only filtered set before releasing lock |
| Selector call under RLock | Selector call after releasing RLock ✅ |

### 3.3 Remove Duplicate Stats Recording
| File | Action |
|------|--------|
| `provider/execution.go` | Consolidate stats in one place (DONE) |
| `provider/manager.go` | Remove redundant `recordProviderResult` calls |

---

## Phase 4: File Consolidation (Low Risk) ✅ COMPLETED

### 4.1 Merge Small Helper Files ✅
| Merge From | Merge Into | Status |
|------------|------------|--------|
| `executor/context_keys.go` (3 lines) | `executor/base_executor.go` | ✅ Done |
| `executor/event_buffer.go` (22 lines) | `executor/stream_translator.go` | ✅ Done |
| `executor/token_refresh.go` (28 lines) | `executor/oauth_helpers.go` | ✅ Done |
| `executor/stream_state.go` (52 lines) | `executor/stream_translator.go` | ✅ Done |

**Files deleted:** 4 files (~105 lines consolidated)

### 4.2 Package Restructure
| Current | Proposed |
|---------|----------|
| `internal/runtime/executor/` (40+ files) | Split into sub-packages |
| - | `internal/runtime/executor/core/` (base, helpers) |
| - | `internal/runtime/executor/providers/` (per-provider) |
| - | `internal/runtime/executor/stream/` (streaming utils) |

---

## Phase 5: Apply Go Standard Patterns

### 5.1 errgroup for Goroutine Management
```go
// Before: naked goroutines
go func() { ... }()

// After: managed with errgroup
g, ctx := errgroup.WithContext(parentCtx)
g.Go(func() error { return process(ctx) })
```

| File | Change |
|------|--------|
| `provider/execution.go` | Use errgroup for stream goroutine |
| `api/handlers/` | Use errgroup for request handling |

### 5.2 sync.Pool for Buffer Reuse
| Already Using | Need to Add |
|---------------|-------------|
| `streaming_helpers.go` (scanner buffer) | Chunk structs |
| - | JSON parse buffers |
| - | SSE format buffers |

### 5.3 Context-First APIs
| Current | Target |
|---------|--------|
| `func Execute(auth, req)` | `func Execute(ctx, auth, req)` |
| Context as afterthought | Context as first param everywhere |

---

## Implementation Order

```
Week 1: Phase 1 (Remove Deprecated) ✅ COMPLETED
├── Day 1: Remove wrapStreamForStats + fix tests ✅
├── Day 2: Remove CLI legacy flags ✅
└── Day 3: Remove deprecated usage/stats code ✅

Week 2: Phase 2 (Consolidate Streaming) ✅ COMPLETED
├── Day 1-2: Integrate streamutil into executors ✅
├── Day 3: Simplify wrapStreamChannel (removed SSE error parsing) ✅
└── Day 4-5: Testing + benchmarking ✅

Week 3: Phase 3 + 4 (Optimize + Consolidate) ✅ COMPLETED
├── Day 1-2: Fix pickNext lock pattern ✅
│   • Moved selector.Pick() call outside RLock
│   • Reduced lock hold time by cloning only after filtering
├── Day 3: Merge small files (optional - skipped)
└── Day 4-5: Package restructure (optional - skipped)

Week 4: Phase 5 (Standard Patterns) - OPTIONAL
├── Day 1-2: Apply errgroup (already done in streamutil)
├── Day 3: Expand sync.Pool usage (already done in streamutil)
└── Day 4-5: Final testing + documentation
```

---

## Success Metrics

| Metric | Before | Target |
|--------|--------|--------|
| Goroutines per stream | 6+ | 2-3 |
| Channels per stream | 4 | 1-2 |
| Lock hold time in MarkResult | ~1ms | ~0.1ms (async) |
| Memory per stream | ~8KB | ~4KB |
| P99 latency (idle) | TBD | -30% |

---

## Risk Assessment

| Phase | Risk | Mitigation |
|-------|------|------------|
| 1 (Deprecated) | Low | Well-isolated, already unused |
| 2 (Streaming) | Medium | Extensive testing, gradual rollout |
| 3 (Provider) | Medium | Feature flag for async result |
| 4 (Files) | Low | Pure refactoring, no logic change |
| 5 (Patterns) | Low | Additive changes |

---

## Files to Delete After Cleanup

```
internal/cli/legacy.go                       # ✅ Deleted - All deprecated flags
internal/runtime/executor/context_keys.go   # ✅ Deleted - Merged to base_executor.go
internal/runtime/executor/event_buffer.go   # ✅ Deleted - Merged to stream_translator.go
internal/runtime/executor/token_refresh.go  # ✅ Deleted - Merged to oauth_helpers.go
internal/runtime/executor/stream_state.go   # ✅ Deleted - Merged to stream_translator.go
```

## Files to Create

```
internal/streamutil/pipeline.go           # ✅ Created
internal/streamutil/idle_watcher.go       # ✅ Created  
internal/streamutil/buffers.go            # ✅ Created
internal/streamutil/reader.go             # ✅ Created
internal/streamutil/result_recorder.go    # ✅ Created
internal/provider/async_result.go         # ✅ Created
```
