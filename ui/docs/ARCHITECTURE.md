# UI Architecture (SvelteKit 5)

This UI is designed to reflect fi-fhir’s core thesis: **users think in workflow/semantic terms, not format terms**. HL7v2/X12 details remain available for inspection, but the default experience is built around **canonical semantic events**, **profiles**, **warnings**, and **workflows**.

## Product Principles (from `fi-fhir/README.md`)

1. **Format-agnostic mental model**
   - Default UI language: `patient_admit`, `lab_result`, `appointment_*`, etc.
   - Format-specific paths (e.g., `PID-3.1`) only appear in an inspector/lineage view.
2. **Workflows over code**
   - Users build routes/actions visually; the UI outputs workflow YAML.
3. **Warnings over errors**
   - Non-fatal issues are first-class objects: filterable, explainable, and trendable.
4. **Three-phase parsing**
   - Always present parsing as: **Byte → Syntactic → Semantic** with phase-tagged issues.

## Layering (keep it modular)

### 1) Contract Layer (generated; committed)

- GraphQL: `ui/src/lib/gen/graphql.ts` generated from `internal/api/graphql/schema.graphql`
- OpenAPI: `ui/src/lib/gen/openapi.ts` generated from `api/openapi.yaml`

**Rule**: generated files are committed and CI fails on drift.

### 2) API Layer (typed boundary)

Location: `ui/src/lib/graphql/*` (and later `ui/src/lib/openapi/*`)

Responsibilities:
- Execute requests using generated contracts
- Normalize/unwrap errors into a single UI error model
- No UI state here

### 3) Domain Layer (UI-owned types)

Location: `ui/src/lib/domain/*` (to be added)

Responsibilities:
- Stable internal models that the UI uses across features
- Mapping helpers (e.g., warning grouping, lineage rendering)
- Avoid leaking GraphQL/OpenAPI types through the whole app

### 4) Feature Modules (vertical slices)

Location: `ui/src/lib/features/<feature>/*` (to be added)

Each feature owns:
- Svelte components
- Stores/state machines
- Minimal “feature services” that call the API layer

### 5) Design System

Location: `ui/src/lib/styles/*`

The UI uses a **design tokens** system for consistent theming:

**Token Files:**
- `tokens.css` - CSS custom properties for colors, spacing, typography, shadows, transitions
- `base.css` - Global styles, CSS reset, and utility classes

**Token Categories:**
- **Colors**: Semantic color system (`--color-primary`, `--color-text-*`, `--color-bg-*`, `--color-border-*`)
- **Spacing**: 4px-based scale (`--space-1` through `--space-12`)
- **Typography**: Font sizes, weights, and line heights (`--text-xs` through `--text-4xl`)
- **Shadows**: Elevation system (`--shadow-sm`, `--shadow-md`, `--shadow-lg`, `--shadow-focus`)
- **Radii**: Border radius tokens (`--radius-sm` through `--radius-full`)
- **Transitions**: Animation timing (`--transition-fast`, `--transition-normal`, `--transition-colors`)

### 6) Shared UI Components

Location: `ui/src/lib/ui/*`

Reusable primitives built on the design system:

**Core Components:**
- `Button` - Primary, secondary, ghost, danger variants with hover animations
- `Badge` - Status indicators with color variants
- `Input`, `Select`, `TextArea` - Form controls with validation states
- `Tabs` - Tab navigation (pills and underline variants)
- `Panel` - Content containers with headers

**Feedback Components:**
- `Toast`, `ToastContainer` - Notifications with auto-dismiss
- `ConfirmModal` - Confirmation dialogs
- `EmptyState` - Zero-state illustrations
- `Skeleton` - Loading placeholders

**Data Display:**
- `WarningList` - Parse warning display with grouping
- `ConfidenceBadge` - Confidence scores with progress bars
- `QualityBadge` - Data quality indicators
- `JsonViewer` - Syntax-highlighted JSON display

**Micro-interactions:**
- Button scale on hover (`transform: scale(1.02)`)
- Card lift shadows on hover (`translateY(-2px)` + elevated shadow)
- Smooth transitions via `--transition-all`

## Directory Conventions

- `ui/src/lib/graphql/**/*.graphql`: **only** `.graphql` documents (no inline query strings).
- `ui/src/lib/gen/*`: generated outputs (only via `npm run codegen`).
- `ui/src/routes/*`: thin route components; heavy logic lives in `src/lib/features/*`.

## Contracts & CI Gates

- Codegen drift:
  - `npm run codegen:check` must be clean.
  - Backend gqlgen generation must be clean (`lint:gqlgen`).
- Type gates:
  - `npm run check` (svelte-check) and `npm run typecheck` (tsc) must pass.

## Non-goals (to keep scope sane)

- No “build-your-own HL7 parser” in the UI.
- No untyped “mapping scripts”; transformations should be represented as typed building blocks that compile down to fi-fhir config.

