# Shipcheck Brand Identity & Assets

This directory contains the official brand assets, color palette, and visual specifications for the **Shipcheck** open-source project.

---

## Brand Assets

| Asset | Preview | Format | Purpose |
| :--- | :---: | :---: | :--- |
| **Primary Icon** | [`shipcheck-icon.svg`](shipcheck-icon.svg) | SVG (128x128) | App icon, favicons, avatar, small badge representation. |
| **Dark Logo** | [`shipcheck-logo-dark.svg`](shipcheck-logo-dark.svg) | SVG (440x96) | Primary wordmark for dark backgrounds and GitHub dark mode. |
| **Light Logo** | [`shipcheck-logo-light.svg`](shipcheck-logo-light.svg) | SVG (440x96) | Primary wordmark for light documentation and white paper PDFs. |
| **Header Banner** | [`shipcheck-header.svg`](shipcheck-header.svg) | SVG (900x180) | Top README banner compatible with GitHub layout. |
| **Social Banner** | [`shipcheck-banner.svg`](shipcheck-banner.svg) | SVG (1280x640) | GitHub OpenGraph card, Twitter/X cards, blog headers. |

---

## Visual Design Language

Shipcheck's identity reflects precision, local-first engineering, and verifiable facts over heuristics:

- **Minimalist & Utilitarian:** Developer-first presentation inspired by terminal diagnostics, Unix tooling, and infrastructure specifications.
- **High Contrast:** Dark slate foundations paired with an electric phosphor mint accent for instant verification feedback.
- **Architectural Honesty:** Visuals emphasize real topology graphs, file relationships, and structured output rather than abstract decoration.

---

## Color Palette

```
  Deep Obsidian       Terminal Charcoal     Border Slate         Phosphor Mint
  #080B0E             #121820               #303A46              #00F59B
  Canvas Background   Card / Surface        Grid & Borders       Primary Accent
```

### Core Palette

| Name | Hex | RGB | Usage |
| :--- | :--- | :--- | :--- |
| **Obsidian Dark** | `#080B0E` | `rgb(8, 11, 14)` | Primary canvas, background foundations |
| **Terminal Slate** | `#121820` | `rgb(18, 24, 32)` | Card surfaces, container fills, terminal panels |
| **Border Muted** | `#303A46` | `rgb(48, 58, 70)` | Isometric container lines, card borders, grid dividers |
| **Phosphor Mint** | `#00F59B` | `rgb(0, 245, 155)` | Verification checkmark, interactive accents, highlights |
| **Text Primary** | `#F0F6FC` | `rgb(240, 246, 252)`| Primary headings and readable code tokens |
| **Text Muted** | `#8B949E` | `rgb(139, 148, 158)`| Subtitles, metadata, and secondary evidence |

### Severity Spectrum

| Tier | Hex | Role |
| :--- | :--- | :--- |
| **CRITICAL** | `#EF4444` | Immediate deployment block, credential exposure, root compromise |
| **HIGH** | `#F59E0B` | Orchestration failure, missing volume/dependency targets |
| **MEDIUM** | `#FACC15` | Anti-patterns, configuration drift, missing template declarations |
| **LOW** | `#38BDF8` | Best-practice advisories, unhandled signals, unpinned tags |
| **INFO** | `#94A3B8` | Diagnostic notes, omitted optional probes |

---

## Typography

- **Wordmark & Interface:** Clean system sans-serif (`-apple-system`, `BlinkMacSystemFont`, `'Segoe UI'`, `Roboto`, `Helvetica`, `Arial`, sans-serif) at bold/heavy weights with tight letter-spacing (`-0.02em`).
- **Code, Evidence, & Terminal Output:** Monospace font stack (`ui-monospace`, `SFMono-Regular`, `'SF Mono'`, `Menlo`, `Consolas`, `'Liberation Mono'`, monospace).

---

## Logo Usage Guidelines

1. **Clear Space:** Maintain clear space equal to at least 25% of the icon's width on all sides.
2. **Minimum Size:** Do not render the full horizontal logo at widths below 180px. Use `shipcheck-icon.svg` for compact spaces (favicons, avatars).
3. **Color Integrity:** Do not alter the geometric angles of the isometric container or apply drop shadows, rainbow gradients, or cartoon effects.
