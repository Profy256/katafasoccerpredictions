# Brand assets

The Katafa mark: a green ring around an ascending bar chart. Green `#16c834`,
navy `#0f1e2e` on the opaque variants.

Every distinct image is stored exactly once. The originals came from a
`katafa-favicon-pack` export that no longer exists — this repo is now the only
copy, so nothing here is regenerable from an outside folder.

## In use

| Path | Size | Role |
| --- | --- | --- |
| `src/app/favicon.ico` | 16/32/48 | Browser tab. All three sizes bundled in the one `.ico`. |
| `src/app/icon.png` | 192 | `rel="icon"` for modern browsers, and the manifest's 192 entry. |
| `src/app/apple-icon.png` | 180 | iOS home screen. Navy background — iOS renders transparency as black. |
| `public/brand/katafa-mark-64.png` | 64 | Header (32px) and footer (24px) mark, at 2x. |
| `public/brand/katafa-mark-512.png` | 512 | Manifest icon; also inlined into the OG card at build time. |
| `public/brand/katafa-mark-512-navy.png` | 512 | Manifest icon for surfaces that composite onto white. |

The three files under `src/app/` are Next.js
[metadata file conventions](https://nextjs.org/docs/app/api-reference/file-conventions/metadata/app-icons):
the `<link>` tags are generated, with content hashes for cache-busting. They are
also plain routes — `/icon.png` serves the 192 — which is why `manifest.ts`
points at `/icon.png` rather than keeping a second 192 in `public/brand/`.

Renaming or moving anything in that table breaks a reference. The users are
`SiteHeader.tsx`, `SiteFooter.tsx`, `manifest.ts` and `opengraph-image.tsx`.

## Not in use

`katafa-icon-16/32/48.png` are the loose PNGs for those sizes; `favicon.ico`
already carries all three, so nothing loads them. Kept only for a surface that
demands a bare PNG.

`katafa-icon-180.png` is the *transparent* 180 — distinct from
`src/app/apple-icon.png`, which is the same size composited on navy.

`preview_strip.png` is a contact sheet, not a shipping asset.

## Re-exporting

The icon was redrawn to match the chosen Katafa logo rather than exported from
it — Canva's edit/copy tools were failing at the time — so it is a close match,
not a pixel-exact cut. For a pixel-perfect version, open the editable logo and
export the icon layer directly:

<https://www.canva.com/d/C6TPbe4ygiH3qwH>

Re-cut every size in the table above from the new export; they are independent
files, not derived at build time.

The original pack also shipped a `favicon-snippet.html` of hand-written `<link>`
tags. It was discarded deliberately: pasting it into `layout.tsx` would
duplicate the tags Next already generates, with unhashed URLs.

## Known mismatch

The mark is green (`#16c834`); the app's `--color-brand` is blue (`#3987e5`), so
the header logo does not match the buttons beside it. Resolving that means
re-theming the palette — and the chart colours in `globals.css` were picked
under a deuteranopia contrast constraint that a green brand hue would need
re-checking against.
