# Dashboard Styling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add CSS styling to the Folio dashboard, which currently renders on bare PicoCSS defaults with no layout, no sidebar structure, no button styles, and no table styles.

**Architecture:** All styles go into a new `/* ── Dashboard ── */` section appended to `internal/assets/static/style.css`. One minimal structural change to `internal/assets/templates/dashboard_base.html` wraps the sidebar links with brand/nav divs that CSS can target. No new files, no JavaScript.

**Tech Stack:** PicoCSS v2 (already loaded as base), plain CSS, Go `text/template`. Test runner: `go-task test` (binary may be installed as `go-task`).

---

## File Map

| File | Change |
|---|---|
| `internal/assets/templates/dashboard_base.html` | Add sidebar brand div + nav wrappers; split admin links |
| `internal/assets/static/style.css` | Append `/* ── Dashboard ── */` section (~150 lines) |

No other files change.

---

### Task 1: Restructure the dashboard sidebar template

The current sidebar is a flat `<nav>` with bare `<a>` tags. We need a brand header div and a nav-links wrapper so CSS has elements to target. We also split the single "Admin" link into "Users" + "Admin Settings" links to match the routes that already exist.

**Files:**
- Modify: `internal/assets/templates/dashboard_base.html`

- [ ] **Step 1: Read the current template**

  Open `internal/assets/templates/dashboard_base.html`. Current sidebar block:

  ```html
  <nav class="dash-sidebar">
    <a href="/-/dashboard/">Repos</a>
    <a href="/-/dashboard/settings">Settings</a>
    {{if .IsAdmin}}<a href="/-/dashboard/admin/">Admin</a>{{end}}
  </nav>
  ```

- [ ] **Step 2: Replace the sidebar block**

  Replace the entire `<nav class="dash-sidebar">…</nav>` with:

  ```html
  <nav class="dash-sidebar">
    <div class="sidebar-brand">Folio</div>
    <div class="sidebar-nav">
      <span class="sidebar-nav-label">Dashboard</span>
      <a href="/-/dashboard/">Repos</a>
      <a href="/-/dashboard/settings">Settings</a>
      {{if .IsAdmin}}
      <span class="sidebar-nav-label">Admin</span>
      <a href="/-/dashboard/admin/">Users</a>
      <a href="/-/dashboard/admin/settings">Settings</a>
      {{end}}
    </div>
  </nav>
  ```

- [ ] **Step 3: Run tests to verify template parses**

  ```bash
  go-task test
  ```

  Expected: all tests pass. Template parse errors surface immediately because `dashboard.New()` parses templates at startup, which runs in every test.

- [ ] **Step 4: Commit**

  ```bash
  git add internal/assets/templates/dashboard_base.html
  git commit -m "feat(dashboard): restructure sidebar template for styling"
  ```

---

### Task 2: Add dashboard layout and sidebar CSS

Wire up the flex layout and sidebar visual design. This is the highest-impact change — it turns the currently broken layout into a real two-column sidebar dashboard.

**Files:**
- Modify: `internal/assets/static/style.css`

- [ ] **Step 1: Append the layout section to `style.css`**

  Add at the very end of `internal/assets/static/style.css`:

  ```css
  /* ── Dashboard layout ── */

  body.dashboard {
    /* body is already flex column (min-height:100vh) from global rules */
  }

  .dash-layout {
    flex: 1;
    display: flex;
    align-items: stretch;
  }

  .dash-sidebar {
    width: 200px;
    flex-shrink: 0;
    background: #f8fafc;
    border-right: 1px solid #e2e8f0;
    display: flex;
    flex-direction: column;
  }

  .dash-main {
    flex: 1;
    min-width: 0;
    padding: 1.5rem 1.75rem;
    background: #fff;
  }

  /* ── Sidebar internals ── */

  .sidebar-brand {
    padding: 0.875rem 1rem 0.75rem;
    font-weight: 700;
    font-size: 1rem;
    color: var(--pico-color);
    border-bottom: 1px solid #e2e8f0;
    letter-spacing: -0.01em;
    flex-shrink: 0;
  }

  .sidebar-nav {
    padding: 0.5rem 0;
    flex: 1;
    display: flex;
    flex-direction: column;
  }

  .sidebar-nav-label {
    display: block;
    padding: 0.5rem 0.875rem 0.25rem;
    font-size: 0.7rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: #94a3b8;
  }

  /* Override PicoCSS nav a styles — :where() has zero specificity so .dash-sidebar a wins */
  .dash-sidebar a {
    display: block;
    padding: 0.4rem 1rem;
    color: #475569;
    text-decoration: none;
    font-size: 0.875rem;
    border-left: 3px solid transparent;
    margin: 0;
  }

  .dash-sidebar a:hover {
    color: var(--pico-color);
    background: #f1f5f9;
  }
  ```

  > **Note — active nav state not implemented here.** Highlighting the current nav link requires each handler to pass an active-page key to the base template, which is a Go-level change out of scope for this CSS-only plan. Add it as a follow-on task if desired.

- [ ] **Step 2: Run tests**

  ```bash
  go-task test
  ```

  Expected: all tests pass (CSS changes cannot break Go tests).

- [ ] **Step 3: Visual check**

  ```bash
  go-task run -- serve
  ```

  Open `http://localhost:8080`. After setup (or if already configured), sign in and visit `/-/dashboard/`. Verify:
  - Two-column layout appears (sidebar left, content right)
  - Sidebar shows "Folio" brand header
  - "Dashboard" and (if admin) "Admin" section labels appear
  - Nav links are readable and styled

- [ ] **Step 4: Commit**

  ```bash
  git add internal/assets/static/style.css
  git commit -m "feat(dashboard): add layout and sidebar CSS"
  ```

---

### Task 3: Add button styles

The templates use `.btn`, `.btn-primary`, `.btn-danger`, and `.btn-link` extensively. None are defined. PicoCSS styles native `<button>` elements but not these classes.

**Files:**
- Modify: `internal/assets/static/style.css`

- [ ] **Step 1: Append button styles to `style.css`**

  ```css
  /* ── Dashboard buttons ── */

  .btn {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.375rem 0.875rem;
    border-radius: 5px;
    font-size: 0.875rem;
    font-weight: 500;
    border: 1px solid #e2e8f0;
    background: #fff;
    color: #374151;
    cursor: pointer;
    text-decoration: none;
    line-height: 1.5;
  }
  .btn:hover {
    background: #f8fafc;
    color: #374151;
    text-decoration: none;
  }

  .btn-primary {
    background: #2563eb;
    border-color: #2563eb;
    color: #fff;
    font-weight: 600;
  }
  .btn-primary:hover {
    background: #1d4ed8;
    border-color: #1d4ed8;
    color: #fff;
  }

  .btn-danger {
    background: #fff;
    border-color: #fca5a5;
    color: #dc2626;
  }
  .btn-danger:hover {
    background: #fef2f2;
    color: #dc2626;
    border-color: #fca5a5;
  }

  .btn-link {
    background: none;
    border: none;
    color: #2563eb;
    padding: 0;
    font-size: 0.875rem;
    cursor: pointer;
    text-decoration: none;
    line-height: inherit;
  }
  .btn-link:hover { text-decoration: underline; }
  ```

- [ ] **Step 2: Run tests**

  ```bash
  go-task test
  ```

  Expected: all tests pass.

- [ ] **Step 3: Commit**

  ```bash
  git add internal/assets/static/style.css
  git commit -m "feat(dashboard): add button styles"
  ```

---

### Task 4: Add flash message and page structure styles

Flash messages (`.flash`, `.flash-success`, `.flash-error`) and page container classes (`.dashboard-page`, `.dashboard-content`, `.dashboard-page-header`) are used in nearly every template but are unstyled.

Note: some templates use `.dashboard-page`, others use `.dashboard-content` — both get identical definitions.

**Files:**
- Modify: `internal/assets/static/style.css`

- [ ] **Step 1: Append flash + page structure styles to `style.css`**

  ```css
  /* ── Dashboard flash messages ── */

  .flash {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.625rem 0.875rem;
    border-radius: 5px;
    margin-bottom: 1rem;
    font-size: 0.875rem;
  }
  .flash-success {
    background: #f0fdf4;
    border: 1px solid #bbf7d0;
    color: #166534;
  }
  .flash-error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
  }

  /* ── Dashboard page structure ── */

  /* Both class names are used inconsistently across templates — define both identically */
  .dashboard-page,
  .dashboard-content {
    max-width: 900px;
  }

  .dashboard-page-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 1.5rem;
  }
  .dashboard-page-header h1 {
    margin: 0;
  }
  ```

- [ ] **Step 2: Run tests**

  ```bash
  go-task test
  ```

  Expected: all tests pass.

- [ ] **Step 3: Commit**

  ```bash
  git add internal/assets/static/style.css
  git commit -m "feat(dashboard): add flash message and page structure styles"
  ```

---

### Task 5: Add table styles

`.dashboard-table` (repos list) and `.users-table` (admin users list) are unstyled. Both get the same visual treatment.

**Files:**
- Modify: `internal/assets/static/style.css`

- [ ] **Step 1: Append table styles to `style.css`**

  ```css
  /* ── Dashboard tables ── */

  .dashboard-table,
  .users-table {
    width: 100%;
    border-collapse: collapse;
    border: 1px solid #e2e8f0;
    border-radius: 6px;
    overflow: hidden;
    font-size: 0.875rem;
  }

  .dashboard-table thead tr,
  .users-table thead tr {
    background: #f8fafc;
  }

  .dashboard-table th,
  .users-table th {
    text-align: left;
    padding: 0.5rem 0.75rem;
    font-size: 0.7rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: #64748b;
    border-bottom: 1px solid #e2e8f0;
  }

  .dashboard-table td,
  .users-table td {
    padding: 0.625rem 0.75rem;
    border-bottom: 1px solid #f1f5f9;
    color: #374151;
    vertical-align: middle;
  }

  .dashboard-table tbody tr:last-child td,
  .users-table tbody tr:last-child td {
    border-bottom: none;
  }

  .dashboard-table tbody tr:hover td,
  .users-table tbody tr:hover td {
    background: #fafafa;
  }
  ```

- [ ] **Step 2: Run tests**

  ```bash
  go-task test
  ```

  Expected: all tests pass.

- [ ] **Step 3: Visual check**

  ```bash
  go-task run -- serve
  ```

  Visit `/-/dashboard/` and `/-/dashboard/admin/`. Verify:
  - Repos table has header row, alternating hover, styled badges
  - Users table renders cleanly with role badges
  - Flash messages appear with correct green/red backgrounds
  - Buttons look correct throughout (primary blue, outlined secondary, link-style)

- [ ] **Step 4: Commit**

  ```bash
  git add internal/assets/static/style.css
  git commit -m "feat(dashboard): add table styles"
  ```

---

### Task 6: Add badge variants and form helper styles

Three badge variants are missing (`.badge-admin`, `.badge-user`, `.badge-self`). Two form helper classes need styling: `.field-warning` (admin settings restart notice) and `.oauth-row` (linked accounts in user settings).

**Files:**
- Modify: `internal/assets/static/style.css`

- [ ] **Step 1: Append badge variants and form helpers to `style.css`**

  ```css
  /* ── Dashboard badge variants ── */

  .badge-admin { background: #ede9fe; color: #5b21b6; }
  .badge-user  { background: #f1f5f9; color: #475569; }
  .badge-self  { background: #f1f5f9; color: #475569; }

  /* ── Dashboard form helpers ── */

  .field-warning {
    padding: 0.625rem 0.875rem;
    background: #fffbeb;
    border: 1px solid #fde68a;
    border-radius: 5px;
    color: #92400e;
    font-size: 0.875rem;
    margin-bottom: 1rem;
  }

  .oauth-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.625rem 0;
    border-bottom: 1px solid #f1f5f9;
    font-size: 0.875rem;
  }
  .oauth-row:last-child { border-bottom: none; }
  ```

- [ ] **Step 2: Run tests**

  ```bash
  go-task test
  ```

  Expected: all tests pass.

- [ ] **Step 3: Commit**

  ```bash
  git add internal/assets/static/style.css
  git commit -m "feat(dashboard): add badge variants and form helper styles"
  ```

---

### Task 7: Style the login page

The login page (`login.html`) is standalone — it does not use `dashboard_base.html`. It has a `.login-box` class, an `.error` paragraph, an `.oauth-buttons` wrapper, and `.btn-github` / `.btn-google` links. All are currently unstyled.

**Files:**
- Modify: `internal/assets/static/style.css`

- [ ] **Step 1: Append login page styles to `style.css`**

  ```css
  /* ── Login page ── */

  /* body is already display:flex flex-direction:column min-height:100vh.
     margin:auto on .login-box centers it in both axes. */
  body:has(.login-box) {
    background: #f1f5f9;
    justify-content: center;
  }

  .login-box {
    margin: auto;
    background: #fff;
    border: 1px solid #e2e8f0;
    border-radius: 10px;
    padding: 2rem 2.25rem;
    width: 360px;
    max-width: calc(100vw - 2rem);
    box-shadow: 0 1px 8px rgba(0, 0, 0, 0.06);
  }

  .login-box h1 {
    font-size: 1.1rem;
    font-weight: 700;
    margin-bottom: 1.5rem;
  }

  .login-box .error {
    color: #991b1b;
    background: #fef2f2;
    border: 1px solid #fecaca;
    border-radius: 5px;
    padding: 0.5rem 0.75rem;
    font-size: 0.875rem;
    margin-bottom: 1rem;
  }

  .oauth-buttons {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    margin-top: 1rem;
    padding-top: 1rem;
    border-top: 1px solid #e2e8f0;
  }

  .btn-github,
  .btn-google {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0.5rem 1rem;
    border-radius: 5px;
    font-size: 0.875rem;
    font-weight: 500;
    text-decoration: none;
    border: 1px solid #e2e8f0;
    background: #fff;
    color: #374151;
  }

  .btn-github {
    background: #24292f;
    border-color: #24292f;
    color: #fff;
  }
  .btn-github:hover {
    background: #1a1e22;
    border-color: #1a1e22;
    color: #fff;
    text-decoration: none;
  }

  .btn-google:hover {
    background: #f8fafc;
    text-decoration: none;
  }
  ```

- [ ] **Step 2: Run tests**

  ```bash
  go-task test
  ```

  Expected: all tests pass.

- [ ] **Step 3: Visual check of login page**

  ```bash
  go-task run -- serve
  ```

  Visit `/-/auth/login`. Verify:
  - Page has a light grey (`#f1f5f9`) background
  - Login card is centered with white background, border, and subtle shadow
  - Form fields and sign-in button are well-spaced
  - "Sign in with GitHub" button is dark (`#24292f`)
  - "Sign in with Google" button is outlined white

- [ ] **Step 4: Final visual pass over all dashboard pages**

  Check each page while the server is running:
  - `/-/auth/login` — centered card, styled OAuth buttons
  - `/-/dashboard/` — repos table with badges, page header with primary button
  - `/-/dashboard/repos/new` — form fields, submit + cancel buttons
  - `/-/dashboard/repos/{id}` — edit form, danger zone, webhook URL section
  - `/-/dashboard/settings` — profile form sections, OAuth row links/buttons
  - `/-/dashboard/admin/` — users table with admin/user/self badges
  - `/-/dashboard/admin/users/{id}` — edit user form
  - `/-/dashboard/admin/settings` — field-warning notice, sectioned form

- [ ] **Step 5: Commit**

  ```bash
  git add internal/assets/static/style.css
  git commit -m "feat(dashboard): style login page"
  ```
