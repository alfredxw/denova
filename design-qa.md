# Design QA: Writing Agent header

- Source visual truth: `/var/folders/3p/tw35s8456m1033g5yxdztf_m0000gn/T/codex-clipboard-7ecf7a70-a513-437b-a5c9-2fdcca12d774.png`
- Wide implementation: `/Users/bytedance/Code/github/denova/docs/design-qa-agent-header-wide.png`
- Narrow implementation: `/Users/bytedance/Code/github/denova/docs/design-qa-agent-header-narrow.png`
- Focused comparison: `/Users/bytedance/Code/github/denova/docs/design-qa-agent-header-comparison.png`
- State: zh-CN, light theme, Writing workspace, empty main surface, populated Agent conversation, Agent panel open.

## Capture normalization

- Source: 2184 x 220 pixels. The focused source crop (2094 x 111 pixels) was normalized from the apparent 2x desktop capture to 1047 x 56 pixels.
- Wide implementation: 1280 x 720 CSS viewport, reported device pixel ratio 2; the browser screenshot was normalized to 1280 x 720 pixels.
- Narrow implementation: 720 x 900 CSS viewport, device pixel ratio 1; screenshot is 720 x 900 pixels.
- Focused comparison stacks the normalized source header above the implementation header so the divider and removed close control can be judged together.

## Findings

No actionable P0, P1, or P2 differences remain.

- Fonts and typography: existing product fonts, sizes, weights, truncation, and control labels are unchanged.
- Spacing and layout rhythm: the workbench tab strip and Agent header both measure 36px high and their bottom borders both resolve to y=45 in the wide viewport.
- Colors and visual tokens: existing surface, border, text, hover, and active tokens are unchanged; the source and implementation were compared in the same light theme.
- Image quality and assets: no image or non-standard icon assets were introduced or replaced.
- Copy and content: no new visible copy was added. The now-unused Chinese and English close labels were removed together.
- Responsive behavior: the Agent header remains intact without a close button at 720 x 900, including with long conversation content. The wide empty-main state also remains intact.

## Interaction and runtime checks

- The existing Agent visibility toggle hides the panel, changes to the show action, and reopens the panel successfully.
- The removed close action is absent after reopening.
- Browser console errors: none.

## Comparison history

1. Initial source finding (P2): the 40px Agent header placed its bottom border below the 36px workbench tab strip; a second close control duplicated the persistent Agent visibility toggle.
2. Fix: changed the docked Agent header to 36px and removed the close control, callback contract, unused icon, and unused bilingual labels.
3. Post-fix evidence: both header regions measure 36px with an identical y=45 bottom edge, the close control count is zero, and hide/show toggle behavior still passes.

## Follow-up polish

None for this scoped change.

final result: passed
