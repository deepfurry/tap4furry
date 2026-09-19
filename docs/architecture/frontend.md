# Tap4Furry — Frontend Architecture

## Public Web

```text
Astro
+
React 19 Islands
```

Rendering:

```text
Astro Node SSR
+
Selective prerender
+
Cloudflare HTML / asset cache
```

Production runtime:

```text
Node 24 LTS
```

Node 24 is chosen for compatibility and low operational risk, not maximum raw runtime throughput.

Deno remains the preferred future non-Node Astro SSR alternative.
Bun may be re-evaluated when Astro/runtime compatibility risk is lower.

## Public Rendering Rule

Public HTML remains anonymous and cacheable.

```text
Public HTML
→ Astro SSR
→ Cloudflare

Private state
→ React Island
→ /api/me/*
```

Do not render per-user Save / Want / Have / auth state into shared SSR HTML.

## Astro Responsibility

Astro handles public layout, SEO/GEO content, Resource metadata, structured data,
Category/Tag/Collection pages, and content-heavy rendering.

React Islands handle interactive surfaces such as:

- Search
- Filters
- Save / Want / Have
- Auth modal
- Collection picker
- Submission wizard
- Poll interaction
- Contact Request
- Graph visualization

Rule:

> **No interaction, no React.**

## Admin

```text
React 19
+
Vite SPA
```

Admin is not SSR and does not use Astro.

## Styling

### Tailwind CSS v4
Layout only:

- flex/grid
- gap
- width/height
- position
- containers
- responsive layout

### SCSS / SCSS Modules
Visual styling:

- colors
- typography
- borders
- radius
- shadows
- states
- animations
- detailed component appearance

## UI Primitives

Use Base UI as headless React primitives.

Build Tap4Furry-owned components on top with SCSS Modules.

Shared primitives may include:

```text
Button
Input
Textarea
Dialog
Select
Combobox
Popover
Tooltip
Checkbox
Switch
Tabs
```

Do not force public business components and admin business components into one shared UI layer.

## State

```text
Server State      → TanStack Query v5
URL State         → TanStack Router (Admin)
Local UI State    → React state
Cross-page UI     → Zustand only when justified
```

Public Web should avoid a global React provider tree.

## Forms

```text
React Hook Form
+
Zod
```

Frontend validation is UX validation. Business validation remains authoritative in Go.

## API Client

```text
OpenAPI
  ↓
Orval
  ↓
@tap4furry/api-client/public
@tap4furry/api-client/admin
```

No duplicate hand-written transport models.

## Same-origin Browser API

```text
tap4furry.com/api/*
→ Public API

admin.tap4furry.com/api/*
→ Admin API
```

This simplifies cookies, CSRF, and CORS.

## Auth UI

Do not store JWTs in localStorage.

Use opaque server-side sessions with HttpOnly Secure cookies.

Login should preserve current user intent.

## i18n

Public:

```text
English first
zh-Hans later
ja later
```

Suggested routing:

```text
/resources/foo
/zh/resources/foo
/ja/resources/foo
```

UI translation and Resource-content translation are separate systems.

Admin is English-only for P0/P1.

## Media

```text
Browser
→ request presigned upload
→ R2/S3
→ Worker validation/processing
→ published media key
```

Database stores `media_key`, not provider-specific permanent URLs.

Strip EXIF metadata by default.

Astro Assets is for static site assets, not the main UGC/media pipeline.

## Workspace

```text
pnpm
pnpm workspace
```

No Turbo/Nx initially.
