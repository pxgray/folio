# Implementation Plan: Medium + Low Priority Improvements

## Changes Overview

Implementing 6 improvements across `internal/web/doc.go`, `internal/web/server.go`, and `internal/web/webhook.go`.

---

## 1. Refactor `serveRepo()` into smaller functions

**File:** `internal/web/doc.go`

**Current:** `serveRepo()` is a 47-line function that does ref resolution, nav loading, nav coverage check, and routing to markdown/directory/raw handlers.

**Plan:**
- Extract `resolveRefAndPath()` — handles ref resolution and returns hash
- Extract `loadNavAndCheckCoverage()` — loads nav and returns filtered items
- Extract `dispatchToContent()` — routes to markdown page, directory page, or raw redirect
- `serveRepo()` becomes a thin orchestrator calling these three functions

**Why:** Single responsibility per function, independently testable, clearer flow.

---

## 2. Replace linear config lookups with map-based lookups

**Files:** `internal/web/server.go`, `internal/web/doc.go`, `internal/web/webhook.go`

**Current:**
- `handleDoc()` iterates `s.cfg.Repos` to find `trusted` (line 86-91)
- `handleLocalDoc()` iterates `s.cfg.Locals` to find `trusted` (line 118-123)
- `repoSecret()` iterates `s.cfg.Repos` on every webhook (line 63-69)

**Plan:**
- Add `repoTrusted map[string]bool` and `localTrusted map[string]bool` to `Server` struct
- Add `repoSecrets map[string]string` to `Server` struct
- Populate these maps in `New()` at startup using `rc.Key()` for remote repos and `lc.Label` for locals
- Replace linear scans with O(1) map lookups
- Remove `repoSecret()` method entirely

**Why:** Consistent with Store's map pattern, O(1) instead of O(n), negligible startup cost.

---

## 3. Fix template execution error handling

**Files:** `internal/web/doc.go`

**Current:** Template errors are logged but HTTP 200 has already been sent, resulting in partial HTML.

**Plan:**
- In `serveMarkdownPage()`, `serveDirPage()`, and `handleIndex()`:
  - Render template to a `bytes.Buffer` first
  - Only write to `w` on success
  - On error, return 500 with error message
- Move `Content-Type` header setting to after successful buffer render

**Why:** Proper HTTP semantics, users see 500 page instead of broken HTML.

---

## 4. Clean up `dirData` duplicate field

**File:** `internal/web/doc.go`

**Current:** `dirData` has both `CurrentPath` (exported) and `currentPath` (unexported) holding the same value.

**Plan:**
- Remove `currentPath` field
- Update `EntryURL()` to use `d.CurrentPath` directly
- Update `serveDirPage()` to only set `CurrentPath`

**Why:** Single source of truth, no drift risk. Go templates can access exported fields.

---

## 5. Improve `headingTitle()` line limit

**File:** `internal/web/doc.go`

**Current:** Only scans first 20 lines. Large frontmatter could push the title beyond this.

**Plan:**
- Increase limit from 20 to 100 lines (covers even very large frontmatter blocks)
- This is the simplest fix that handles the edge case without adding frontmatter parsing dependency

**Why:** 100 lines covers any realistic frontmatter while keeping the function simple.

---

## 6. Add webhook rate limiting

**File:** `internal/web/webhook.go`

**Current:** No rate limiting on webhook endpoint.

**Plan:**
- Add `webhookLimiter map[string]time.Time` to `Server` struct (last webhook time per repo key)
- Add `webhookLimiterMu sync.Mutex` for thread safety
- In `handleWebhook()`, check if last webhook for this repo was < 30 seconds ago
- If rate limited, return 429 Too Many Requests
- Update timestamp after successful webhook processing

**Why:** Defense in depth. Prevents abuse even if secret is compromised. No new dependencies needed.

---

## Execution Order

1. Map-based config lookups (foundational, affects multiple files)
2. Clean up dirData duplicate field (simple, isolated)
3. Refactor serveRepo() (structural, depends on nothing)
4. Fix template error handling (behavioral change)
5. Improve headingTitle() (one-line change)
6. Add webhook rate limiting (new behavior)
7. Run tests after all changes
