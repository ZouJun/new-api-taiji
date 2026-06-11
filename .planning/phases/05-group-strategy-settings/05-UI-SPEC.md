---
phase: 5
slug: group-strategy-settings
status: approved
shadcn_initialized: false
preset: none
created: 2026-06-11
---

# Phase 5 — UI Design Contract

> Visual and interaction contract for frontend phases. Generated for Phase 5 and aligned to the existing system settings surface in `web/default`.

---

## Design System

| Property | Value |
|----------|-------|
| Tool | none |
| Preset | not applicable |
| Component library | base-ui + local settings primitives |
| Icon library | lucide-react |
| Font | inherit existing app sans stack |

---

## Spacing Scale

Declared values (must be multiples of 4):

| Token | Value | Usage |
|-------|-------|-------|
| xs | 4px | Inline icon gap, helper chips |
| sm | 8px | Field-to-description spacing |
| md | 16px | Default form row gap |
| lg | 24px | Section body gap |
| xl | 32px | Sub-block separation inside strategy editor |
| 2xl | 48px | Not used in this phase |
| 3xl | 64px | Not used in this phase |

Exceptions: none

---

## Typography

| Role | Size | Weight | Line Height |
|------|------|--------|-------------|
| Body | 14px | 400 | 1.5 |
| Label | 14px | 500 | 1.4 |
| Heading | 18px | 600 | 1.3 |
| Display | 24px | 600 | 1.2 |

Use display typography nowhere in this phase. This is an operator workflow, not a marketing surface.

---

## Color

| Role | Value | Usage |
|------|-------|-------|
| Dominant (60%) | existing app background tokens | Page background, form surfaces |
| Secondary (30%) | existing muted/surface tokens | Section grouping, informational callouts |
| Accent (10%) | existing primary brand token | Save actions, selected tabs, active controls |
| Destructive | existing destructive token | Remove strategy row, invalid state, destructive confirmation |

Accent reserved for: primary save button, active section state, enabled strategy emphasis. Do not tint the entire editor with accent color.

---

## Screen Contract

### Placement

- Add a new Operations Settings section named `策略设置`.
- In the Operations Settings registry, place it immediately after `System Behavior`.
- It becomes the second section in `/system-settings/operations`, before `Monitoring & Alerts`.
- Do not move unrelated settings into this section.

### Section Shape

- Reuse the existing `SettingsSection` and `SettingsForm` layout pattern.
- Top area contains:
  - section title
  - one concise overview paragraph
  - one muted precedence/fallback note block
- Main content uses a two-level structure:
  1. global explanation / defaults summary
  2. per-group strategy editor

### Strategy Editor Shape

- Use a structured list or table-like editor, not a raw JSON textarea by default.
- Each group from `GroupRatio` appears as one row/card item with stable height and predictable field placement.
- Each row shows:
  - group name
  - enabled switch
  - stream first-byte retry budget (seconds)
  - retry times
  - timeout HTTP status
  - timeout error message
  - fallback badge when strategy is disabled or missing
- Rows must support scanability first. Operators should compare `default` vs `vip` without opening modal dialogs unless the list grows too dense.

### Fallback Communication

- If a group has no custom strategy, show an explicit fallback message in-row:
  - `Uses global retry and timeout behavior`
- If a field is left blank but the strategy row is enabled, show the exact fallback result for that field rather than a vague "default".
- The UI must explain that:
  - group retry overrides global `RetryTimes`
  - missing group strategy falls back to current global behavior
  - stream retry budget does not replace per-channel timeout settings

### Help Density

- Every field gets a `FormDescription`.
- At least one info callout near the top must explain the difference between:
  - relay retry count
  - AWS SDK retry
  - per-channel timeout
  - stream first-byte retry budget
- Warnings and precedence notes belong in muted bordered blocks, not modal popups.

---

## Interaction Contract

### Primary Workflow

1. Operator opens `策略设置`
2. Reads short explanation and precedence note
3. Reviews auto-loaded group list sourced from `GroupRatio`
4. Enables or disables strategy per group
5. Adjusts numeric and response fields
6. Saves all pending changes through the existing settings action pattern

### Editing Rules

- Numeric fields use number inputs with min bounds:
  - retry budget seconds: min `1`
  - retry times: min `0`
  - timeout HTTP status: min `100`, max `599`
- Disabled strategy rows visually collapse secondary controls or dim them, but still show current saved values for auditability.
- Validation errors must stay inline at the field level.
- Saving should remain page-level, consistent with other settings sections.

### Empty and Partial States

- If no groups can be resolved from `GroupRatio`, show a non-destructive empty state inside the section:
  - heading: `No groups available`
  - body: `Create group ratios first, then return here to configure strategy behavior.`
- If some groups exist but no strategies are configured, render all rows in fallback mode rather than showing an empty table.

---

## Component Contract

| Area | Contract |
|------|----------|
| Section shell | Reuse `SettingsSection` |
| Form shell | Reuse `SettingsForm` and `SettingsPageFormActions` |
| Boolean control | `Switch` |
| Numeric controls | standard `Input type='number'` with safe numeric helpers |
| Group collection | structured repeated rows in one section body; no nested cards inside cards |
| Informational note | plain bordered block using existing utility classes |
| Remove/reset action | secondary or ghost action only if manual row clearing exists |

Do not introduce decorative cards, marketing banners, or full-page wizard flow for this phase.

---

## Copywriting Contract

| Element | Copy |
|---------|------|
| Primary CTA | Save strategy settings |
| Empty state heading | No groups available |
| Empty state body | Create group ratios first, then return here to configure strategy behavior. |
| Error state | We could not save strategy settings. Check the highlighted fields and try again. |
| Destructive confirmation | Reset strategy row: This clears the custom strategy for this group and returns it to global behavior. |

### Required Field Copy

- Section title: `Strategy Settings`
- Section intro: `Configure retry behavior and streaming first-byte retry budgets for each group.`
- Precedence note: `When a group strategy is enabled, its retry count overrides global Retry Times. If no strategy is configured for a group, the system uses the current global retry and timeout behavior.`
- Retry budget description: `Maximum total seconds spent waiting for the first stream response across retries for this group.`
- Retry times description: `Additional relay retries for this group. The first request is not counted.`
- HTTP status description: `Returned when the stream retry first-byte budget is exhausted and no clearer upstream error is available.`
- Error message description: `Custom message for budget exhaustion. Leave empty to use: 资源繁忙，请稍后尝试`

### Copy Style Rules

- Prefer concrete operational language over protocol jargon.
- Never describe this feature as billing or pricing control.
- Use `group` consistently; do not mix `group`, `segment`, and `tier` in the same screen.

---

## Operator Guidance Contract

- Include at least three concrete usage examples in supporting copy or helper text:
  - `default` group with conservative retry
  - `vip` group with longer stream wait budget
  - fallback behavior when no strategy is configured
- Guidance must help an operator answer:
  - when should I raise retry count
  - when should I raise stream retry budget
  - when should I leave a group on fallback
- Do not rely on tooltips alone for critical precedence rules.

---

## Registry Safety

| Registry | Blocks Used | Safety Gate |
|----------|-------------|-------------|
| local project components | `SettingsSection`, `SettingsForm`, `SettingsPageFormActions`, `FormField`, `Switch`, `Input` | not required |
| lucide-react | optional inline icons for info or reset affordances | not required |

No third-party visual registry adoption is needed for this phase.

---

## Checker Sign-Off

- [x] Dimension 1 Copywriting: PASS
- [x] Dimension 2 Visuals: PASS
- [x] Dimension 3 Color: PASS
- [x] Dimension 4 Typography: PASS
- [x] Dimension 5 Spacing: PASS
- [x] Dimension 6 Registry Safety: PASS

**Approval:** approved 2026-06-11
