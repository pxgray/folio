# Dashboard Styling Design

**Date:** 2026-04-14
**Status:** Approved

## Summary

Add CSS styling to the Folio dashboard. Currently almost all dashboard-specific CSS classes are undefined — the dashboard renders on bare PicoCSS defaults with no layout, no sidebar, no button styles, and no table styles. This spec covers adding a `/* ── Dashboard ── */` section to the existing `style.css`.

## Decisions Made

- **Layout:** Classic left sidebar (`.dash-layout` = sidebar + content flex row)
- **Sidebar tone:** Light off-white (`#f8fafc`) with a blue (`#2563eb`) active-state accent
- **Login page:** Centered card on a grey background
- **Implementation:** All CSS added to `style.css` — no new files, no new dependencies, no JS

## Scope

### What changes

`internal/assets/static/style.css` — append a new `/* ── Dashboard ── */` section defining all missing classes.

`internal/assets/templates/dashboard_base.html` — minimal structural addition only: wrap the sidebar links in a brand header div and a nav section div, so CSS has elements to target. No logic changes, no new data requirements.

No new files. No JavaScript.

### Classes to define

**Layout**
- `.dash-layout` — flex row, full viewport height
- `.dash-sidebar` — 200px wide, `#f8fafc` background, right border, flex column
- `.dash-main` — flex 1, white background, `24px 28px` padding

**Sidebar internals**
- `.sidebar-brand` — logo/name header, bold, bottom border
- `.sidebar-nav` — padding wrapper for nav links
- `.sidebar-nav-label` — small uppercase section label (`10px`, `#94a3b8`)
- `.sidebar-nav-link` — block link, `7px 16px` padding, hover: light grey bg; active: blue text + left border strip + `#eff6ff` bg
- `.sidebar-footer` — small muted user/role line, top border

**Buttons** (currently completely undefined)
- `.btn` — outlined secondary: white bg, `#e2e8f0` border, `#374151` text, `5px` radius
- `.btn-primary` — `#2563eb` bg + border, white text, semibold
- `.btn-danger` — white bg, red border (`#fca5a5`), red text (`#dc2626`)
- `.btn-link` — no border/bg, blue text, inline action style

**Tables**
- `.dashboard-table` / `.users-table` — full-width, collapsed borders, `1px solid #e2e8f0` outer border, `6px` radius; `thead` on `#f8fafc`; `th` uppercase 11px labels; `td` `10px 12px` padding; `tbody tr:hover` subtle highlight

**Flash messages** (currently undefined)
- `.flash` — flex row with gap, `10px 14px` padding, `5px` radius
- `.flash-success` — `#f0fdf4` bg, `#bbf7d0` border, `#166534` text
- `.flash-error` — `#fef2f2` bg, `#fecaca` border, `#991b1b` text

**Badges** (missing variants)
- `.badge-admin` — `#ede9fe` bg, `#5b21b6` text
- `.badge-user` — `#f1f5f9` bg, `#475569` text
- `.badge-self` — same as `.badge-user` (the "you" tag)

**Dashboard page structure**
- `.dashboard-page` and `.dashboard-content` — defined identically as a `max-width: 900px` container (templates use both class names inconsistently; define both to avoid future confusion)
- `.dashboard-page-header` — flex row, space-between, align-center, `margin-bottom: 20px`

**Forms**
- `.field-warning` — amber-tinted info box (`#fffbeb` bg, `#fde68a` border, `#92400e` text) for the "restart required" notice in admin settings
- `.oauth-row` — flex row, space-between, align-center, `10px 0` padding, bottom border; used in Account Settings for linked OAuth accounts

**Login page**
- `.login-box` — centers a card: `display: flex; min-height: 100vh; align-items: center; justify-content: center; background: #f1f5f9`; the inner card is `background: #fff; border: 1px solid #e2e8f0; border-radius: 10px; padding: 32px 36px; width: 360px; box-shadow: 0 1px 8px rgba(0,0,0,.06)`
- `.login-box h1` — brand heading style
- `.error` — red error text for login failures
- `.oauth-buttons` — flex column, gap for OAuth links
- `.btn-github` — dark (`#24292f`) filled button
- `.btn-google` — outlined button with Google-blue border

## Visual Specification

### Colors (all reference PicoCSS vars where possible, raw hex where not)

| Role | Value |
|---|---|
| Sidebar background | `#f8fafc` |
| Sidebar border | `#e2e8f0` |
| Active nav bg | `#eff6ff` |
| Active nav text | `#2563eb` |
| Active nav strip | `#3b82f6` |
| Primary button | `#2563eb` |
| Danger text | `#dc2626` |
| Muted text | `#64748b` / `#94a3b8` |
| Login page bg | `#f1f5f9` |

### Typography

All sizes in `em`/`rem` or `px` matching existing style.css conventions. No new fonts — inherits PicoCSS's system font stack.

### Dark mode

PicoCSS handles dark mode via `data-theme="dark"` on `<html>`. The dashboard sidebar and login background colors are hardcoded light-mode values. A follow-on task can add `[data-theme="dark"] .dash-sidebar { ... }` overrides if desired — out of scope for this implementation.

## Out of Scope

- Dark mode overrides for dashboard
- Responsive/mobile layout for the dashboard sidebar
- Any JavaScript changes
- Any template changes
- New icon assets (sidebar uses text "⬡" placeholder, not SVG icons)
