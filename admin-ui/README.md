# DeskForge Admin UI

Vue 3 admin panel for [DeskForge](../README.md) — a unified, self-hosted, RustDesk-compatible server.

## Tech stack

Vue 3.5 · Vite 6 · Element Plus 2.8 · Pinia · Vue Router · Axios · Sass

## Development

```bash
npm install
npm run dev     # local dev server
npm run build   # production build → dist/, served by the Go API at /admin/
```

The dev server proxies API calls to the Go backend; see `vite.config.js` for the proxy target.

## Layout

- `src/views/` — page modules
- `src/components/ui/` — shared design-system primitives (`DataTable`, `AppDialog`, `AppDrawer`, `FilterBar`, `PageHeader`, `PageSection`, `DangerZone`, …)
- `src/store/` — Pinia stores
- `src/api/` — REST client wrappers
- `src/utils/i18n/` — locale JSON (English, Russian, Chinese)

## Theming

Light / Dark / Auto modes via CSS variables in `src/styles/style.scss`. Dark mode is toggled with `html[data-theme="dark"]`; never hardcode colors — use the tokens.
