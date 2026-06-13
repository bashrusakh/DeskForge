# UI Rework Plan — full_Server Web Admin

## Agent Ownership Notice

This `ui-rework.md` plan and the related `admin-ui/ui-rework-preview.html` prototype are currently owned by OpenCode in the active UI rework task.

Other agents should not edit, overwrite, refactor, or delete this document, the preview prototype, or the planned UI rework direction unless the user explicitly asks them to do so.

## 1. Context

Project: `full_Server`

Admin UI location: `admin-ui/`

Current stack:

- Vue 3.5.13
- Vite 6.3.4
- Element Plus 2.8.2
- Pinia 2.2.8
- Vue Router 4
- Axios
- Sass
- Element Plus icons
- VueUse `useDark`

The project is a web admin panel for a RustDesk-compatible remote access server. It manages devices, users, address books, groups, monitoring logs, server commands, OAuth/SSO, API tokens, and custom client builds.

## 2. Goal

Rework the admin UI into a simple, clean, modern, functional interface for a remote access server.

The new UI should:

- be easy to understand for administrators;
- avoid overloaded menus and unclear functions;
- have unified fonts, tables, checkboxes, forms, dialogs, buttons, and states;
- support light, dark, and auto themes;
- look modern without copying AnyDesk or TeamViewer branding;
- borrow product ideas from remote access tools:
  - quick connect by ID;
  - clear online/offline status;
  - device-first workflow;
  - visible security and session history;
  - simple operational controls;
  - dangerous actions separated from normal actions.

## 3. Current Problems

### Navigation

Current navigation is too detailed and API-oriented. It exposes many low-level entities at the same level:

- Devices
- Users
- Groups
- Address Book
- Security
- Monitoring
- Client Builder
- Server
- My Profile

This creates cognitive overload.

### Layout

Current layout:

- dark sidebar;
- dark header;
- tags bar under header;
- classic Element Plus admin shell.

Issues:

- tags bar adds visual noise;
- sidebar/header colors are hardcoded;
- layout is desktop-first only;
- no clear product identity.

### Styling

Current styling is scattered:

- hardcoded colors like `#2d3a4b`, `#3f454b`, `#283342`, `#409eff`;
- minimal global styles;
- no design tokens;
- no spacing scale;
- no typography scale;
- no proper theme system;
- dark mode only through `html.dark`.

### Tables

Tables are repeated manually in many views.

Common duplicated structure:

```html
<el-card class="list-query">
<el-card class="list-body">
<el-card class="list-page">
```

Issues:

- no shared table component;
- no shared filters;
- no shared pagination;
- no shared empty/loading states;
- no shared column manager;
- action columns are inconsistent;
- table density differs between pages.

### Forms and Dialogs

Forms and dialogs are repeated manually.

Issues:

- inconsistent widths;
- inconsistent labels;
- inconsistent footer actions;
- many dialogs lack validation;
- submit buttons often lack loading state;
- simple and complex forms use the same pattern.

### Theme

Current theme support is minimal:

- dark mode switch exists in `src/layout/components/setting/index.vue`;
- Element Plus dark CSS is imported;
- custom CSS variables are almost absent;
- hardcoded colors remain in many components.

## 4. Product Direction

The UI should become a connection control center, not just a CRUD admin panel.

The admin should quickly answer four questions:

1. Is the server healthy?
2. Which devices are online?
3. Who connected and what happened?
4. How do I connect to a device?

The main design idea:

> A clean operational console for remote access, where connection, device status, security, and monitoring are obvious at a glance.

## 5. Borrowed Ideas from Remote Access Products

This plan borrows interaction ideas, not branding.

From AnyDesk/TeamViewer-like products:

- quick connect by ID;
- visible online/offline device status;
- address book as a primary workflow;
- compact session history;
- security/permission-first thinking;
- quick actions on the main screen;
- clear separation between normal and dangerous operations.

For this project, these ideas become:

- `Quick Connect` card on dashboard;
- `Connection Pulse` status indicator;
- device table optimized for remote access operations;
- monitoring section for login, connection, file transfer, and shared sessions;
- server commands separated into safe and dangerous zones.

## 6. New Information Architecture

Reduce top-level navigation to six main sections.

```text
Dashboard
Devices
Access
Monitoring
Security
Server
```

### Dashboard

Purpose:

- server health;
- quick connect;
- online devices;
- recent activity.

### Devices

Includes:

- all devices;
- device groups;
- quick connect;
- device edit;
- device actions.

### Access

Includes:

- address books;
- collections;
- tags;
- share rules;
- shared sessions.

### Monitoring

Includes:

- login history;
- connection history;
- file transfer history;
- shared sessions, if not placed under Access.

### Security

Includes:

- users;
- API tokens;
- OAuth / SSO providers;
- blocklist / blacklist.

### Server

Includes:

- server config;
- server commands;
- relay settings;
- must-login settings;
- usage settings;
- custom client builder;
- GitHub build settings.

### My Profile

Move out of the sidebar.

Keep it in the user menu in the top-right header.

## 7. Layout Concept

```text
┌─────────────────────────────────────────────────────┐
│ Logo / title              Search        Theme  User │
├───────────────┬─────────────────────────────────────┤
│ Dashboard     │                                     │
│ Devices       │  Page content                       │
│ Access        │                                     │
│ Monitoring    │                                     │
│ Security      │                                     │
│ Server        │                                     │
└───────────────┴─────────────────────────────────────┘
```

### Layout rules

- sidebar is semantic and compact;
- header is clean;
- no mandatory tags bar;
- page content has consistent padding;
- mobile layout uses drawer navigation;
- sidebar can collapse to icon-only mode;
- user actions live in the header user menu.

## 8. Visual Identity

### Fonts

Use two font families maximum.

Primary UI font:

```text
Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont,
"Segoe UI", sans-serif
```

Monospace font for IDs, IPs, tokens, commands, logs:

```text
JetBrains Mono, ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas,
"Liberation Mono", monospace
```

If external fonts are not desired, use system fallbacks only.

### Font weights

Use only:

- 400 — body text;
- 500 — secondary text;
- 600 — section titles;
- 700 — numbers and strong emphasis.

### Type scale

```text
12px — captions, badges, table metadata
13px — table body
14px — body, labels, inputs
16px — page actions, form labels
18px — section headings
24px — page titles
32px — dashboard numbers
```

## 9. Signature Visual Element

Use one memorable product-specific element:

## Connection Pulse

A small animated or static status indicator used for:

- online/offline devices;
- active sessions;
- ID server status;
- relay server status;
- quick connect actions.

This gives the UI a remote-access identity without copying AnyDesk or TeamViewer.

## 10. Design Tokens

All colors, spacing, radius, shadows, and typography should move to tokens.

### Light theme tokens

```css
--color-bg: #F5F7FB;
--color-surface: #FFFFFF;
--color-surface-2: #EEF3F8;
--color-text: #111827;
--color-muted: #64748B;
--color-border: #D9E2EC;
--color-primary: #2563EB;
--color-primary-soft: #DBEAFE;
--color-success: #16A34A;
--color-success-soft: #DCFCE7;
--color-warning: #F59E0B;
--color-warning-soft: #FEF3C7;
--color-danger: #DC2626;
--color-danger-soft: #FEE2E2;
--color-code-bg: #F1F5F9;
```

### Dark theme tokens

```css
--color-bg: #0B1020;
--color-surface: #111827;
--color-surface-2: #182235;
--color-text: #E5E7EB;
--color-muted: #94A3B8;
--color-border: #263244;
--color-primary: #60A5FA;
--color-primary-soft: rgba(96, 165, 250, 0.16);
--color-success: #22C55E;
--color-success-soft: rgba(34, 197, 94, 0.16);
--color-warning: #FBBF24;
--color-warning-soft: rgba(251, 191, 36, 0.16);
--color-danger: #F87171;
--color-danger-soft: rgba(248, 113, 113, 0.16);
--color-code-bg: #1F2937;
```

## 11. Theme System

Support three modes:

```text
Auto
Light
Dark
```

### Behavior

- `auto` follows `prefers-color-scheme`;
- `light` forces light theme;
- `dark` forces dark theme;
- selected mode is stored in `localStorage`;
- theme is applied through `html[data-theme]`;
- Element Plus theme variables must be aligned with custom tokens.

### Files to change

Likely files:

- `src/styles/style.scss`;
- `src/main.js`;
- `src/layout/components/setting/index.vue`;
- `src/store/app.js` or new `src/store/theme.js`;
- `src/layout/components/header.vue`;
- `src/layout/index.vue`.

## 12. Dashboard Redesign

Dashboard should be the operational center.

### Main blocks

```text
┌─────────────────────────────────────────────────────┐
│ Quick Connect                                       │
│ [ RustDesk ID ] [ Connect ] [ Web Client ]          │
└─────────────────────────────────────────────────────┘

┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│ Online       │ │ Users        │ │ Sessions     │
│ devices      │ │ total        │ │ recent       │
└──────────────┘ ┴──────────────┘ ┴──────────────┘

┌───────────────────────────┐ ┌───────────────────────────┐
│ Recent Connections        │ │ Server Health             │
│ login / relay / api       │ │ id server / relay status  │
└───────────────────────────┘ ┴───────────────────────────┘
```

### Dashboard cards

- Online Peers;
- Total Devices;
- Total Users;
- Recent Logins;
- Recent Connections;
- Server Health;
- Web Client status.

### Quick Connect

Must support:

- manual ID input;
- open native client;
- open web client if enabled;
- navigate to device details if found.

## 13. Devices Redesign

Devices is the main working screen.

### Default columns

```text
Status
ID
Hostname
User
OS
Group
Version
Last Online
Actions
```

### Table behavior

- online/offline through `Connection Pulse`;
- ID copy by click;
- hostname/IP with tooltip;
- row actions in compact menu;
- unified pagination;
- unified empty/loading states;
- unified column manager;
- compact and normal density.

### Device actions

- Connect;
- Web Client;
- Add to Address Book;
- Edit;
- Delete.

## 14. Access Redesign

Access should group everything related to remote access permissions.

### Sections

- Address Books;
- Collections;
- Tags;
- Share Rules;
- Shared Sessions.

### UX direction

- fewer separate CRUD pages;
- more card-based grouping;
- share rules shown as policy list;
- shared sessions shown with clear active/expired status.

## 15. Monitoring Redesign

Monitoring should become an audit console.

### Sections

- Login History;
- Connection History;
- File Transfers;
- Shared Sessions.

### UX direction

- unified filters;
- date range filter;
- user filter;
- peer/IP filter;
- type filter;
- export action;
- batch delete only in danger toolbar;
- compact mode for large tables;
- virtual table if data volume becomes too high.

## 16. Security Redesign

Security should look strict, clean, and safe.

### Sections

- Users;
- API Tokens;
- OAuth / SSO;
- Blocklist / Blacklist.

### UX direction

- users in table + drawer edit;
- tokens with clear expiration;
- OAuth providers as cards;
- blocklist/blacklist as policy list;
- destructive actions separated and confirmed.

## 17. Server Redesign

Server is the most dangerous section.

### Sections

- Overview;
- Config;
- Commands;
- Relay / Access Controls;
- Custom Client Builder;
- GitHub Build.

### UX direction

- config shown as readable cards;
- commands separated into Simple and Advanced;
- dangerous commands inside `Danger Zone`;
- explicit confirmation before sending commands;
- command output shown in terminal-like block;
- monospace font for commands and results.

## 18. Unified Components

Create a small internal design system.

### Layout components

```text
AppLayout
AppSidebar
AppHeader
AppShell
PageHeader
PageSection
UserMenu
```

### UI components

```text
StatCard
StatusBadge
ConnectionPulse
DataTable
TableToolbar
FilterBar
ActionMenu
AppDialog
AppDrawer
FormSection
CopyableText
EmptyState
LoadingState
ThemeSwitch
QuickConnect
DangerZone
```

## 19. Shared Composables

Move repeated logic out of views.

```text
usePaginatedList
useCrudList
useConfirmDelete
useBatchDelete
useExportImport
useColumnManager
useTheme
usePageTitle
useCopyToClipboard
```

## 20. DataTable Standard

`DataTable` should replace most direct `el-table` usage.

### Features

- loading;
- empty state;
- selection;
- pagination;
- sortable columns;
- column visibility;
- row actions;
- compact mode;
- normal mode;
- responsive horizontal scroll.

### Table standards

```text
normal row height: 44px
compact row height: 38px
selection column: 44px
body text: 13/14px
header text: 12px
actions: right-aligned
```

## 21. Checkbox Standard

All checkboxes should look and behave the same.

### Rules

- size: 16px;
- label: 14px;
- same disabled state;
- same hover/focus state;
- same spacing in forms and tables;
- no manual per-page overrides.

## 22. Form and Dialog Standard

### AppDialog

Should replace repeated `el-dialog` patterns.

Features:

- title;
- footer actions;
- loading submit;
- cancel/confirm;
- validation integration;
- danger variant.

### AppDrawer

Use for complex forms.

Examples:

- user edit;
- device edit;
- address book edit;
- OAuth provider edit.

### FormSection

Use to group fields.

Example:

```text
Account
Permissions
Security
Notes
```

## 23. Login and Register Redesign

Current login/register screens are simple dark cards with hardcoded colors.

### Desktop login layout

```text
┌──────────────────────────────┬─────────────────────┐
│ Visual panel                 │ Login card          │
│ Remote Access Server         │ Username            │
│ Connection Pulse animation   │ Password            │
│ ID / Relay / API status      │ SSO buttons         │
└──────────────────────────────┴─────────────────────┘
```

### Mobile login layout

- one centered card;
- no heavy visual panel;
- SSO buttons as a clean list;
- captcha and password fields remain accessible.

## 24. Error Page Redesign

Current 404 page is minimal.

New 404 page should include:

- clear message;
- button back to dashboard;
- optional server status summary;
- no unnecessary decoration.

## 25. Implementation Phases

### Phase 1 — Foundation

Deliverables:

- design tokens;
- theme system;
- typography tokens;
- spacing tokens;
- Element Plus overrides;
- theme switch with Auto/Light/Dark.

### Phase 2 — Layout

Deliverables:

- new `AppLayout`;
- new `AppSidebar`;
- new `AppHeader`;
- responsive shell;
- user menu;
- optional removal of tags bar.

### Phase 3 — Design System Components

Deliverables:

- `PageHeader`;
- `PageSection`;
- `StatCard`;
- `StatusBadge`;
- `ConnectionPulse`;
- `DataTable`;
- `TableToolbar`;
- `FilterBar`;
- `ActionMenu`;
- `AppDialog`;
- `AppDrawer`;
- `FormSection`;
- `CopyableText`;
- `EmptyState`;
- `LoadingState`;
- `QuickConnect`;
- `DangerZone`.

### Phase 4 — Shared Composables

Deliverables:

- `usePaginatedList`;
- `useCrudList`;
- `useConfirmDelete`;
- `useBatchDelete`;
- `useExportImport`;
- `useColumnManager`;
- `useTheme`;
- `usePageTitle`;
- `useCopyToClipboard`.

### Phase 5 — High-Priority Screens

Rework in this order:

1. Dashboard;
2. Login/Register;
3. Devices;
4. Monitoring;
5. Server Config/Commands;
6. Users/Security;
7. Access;
8. My Profile.

### Phase 6 — Remaining CRUD Screens

Unify:

- users;
- tags;
- groups;
- address books;
- share records;
- tokens;
- OAuth;
- device groups;
- custom client builder;
- GitHub build settings.

### Phase 7 — QA

Check:

- light theme;
- dark theme;
- auto theme;
- mobile layout;
- route permissions;
- i18n;
- build;
- console errors;
- API errors;
- empty states;
- loading states;
- table selection;
- batch operations;
- export/import;
- server command confirmations.

## 26. Things Not to Break

Do not change backend contracts.

Preserve:

- API endpoints;
- response format;
- auth flow;
- `api-token` header;
- `route_names`;
- hash routing;
- i18n;
- web client flow;
- OIDC flow;
- CSV export/import;
- server command behavior;
- GitHub build long-poll behavior.

## 27. Known Risks

### Element Plus overrides can become fragile

Mitigation:

- use CSS variables first;
- use wrapper components;
- avoid deep overrides unless necessary.

### Large monitoring tables can be slow

Mitigation:

- keep pagination;
- add compact mode;
- use virtual table if needed.

### Server commands are dangerous

Mitigation:

- separate `Danger Zone`;
- add confirmations;
- use terminal-like output;
- make destructive actions visually distinct.

### Dynamic route permissions

Mitigation:

- keep `route_names`;
- build sidebar from existing route metadata;
- do not change permission logic.

### i18n gaps

Mitigation:

- replace raw labels gradually;
- add keys to all translation files;
- keep existing translations stable.

### Theme migration

Mitigation:

- migrate tokens before pages;
- do not hardcode colors in new components.

## 28. Verification Checklist

Before considering the rework complete:

- [ ] `ui-rework.md` exists.
- [ ] Light theme works.
- [ ] Dark theme works.
- [ ] Auto theme works.
- [ ] No hardcoded layout colors remain.
- [ ] Sidebar navigation is simplified.
- [ ] My Profile is moved to user menu.
- [ ] Tags bar is removed or made optional.
- [ ] Tables use a unified component.
- [ ] Checkboxes use one standard.
- [ ] Forms use one dialog/drawer standard.
- [ ] Dashboard has Quick Connect.
- [ ] Devices page shows clear online/offline status.
- [ ] Monitoring pages share one filter model.
- [x] Server commands have danger confirmations.
- [ ] Login/Register are redesigned.
- [ ] Mobile layout works.
- [ ] i18n still works.
- [ ] `npm run build` passes.
- [ ] Console has no new UI errors.

## 29. Current Implementation Status

As of 2026-06-14 UI rework pass:

- Foundation pass started in `admin-ui/`.
- Global design tokens were added in `src/styles/style.scss` for light/dark surfaces, text, borders, status colors, radius, shadows, and typography.
- Theme mode now supports `auto`, `light`, and `dark` through `html[data-theme]`, stored in `localStorage` as `theme-mode`.
- Header/sidebar layout colors were moved off the old hardcoded `#2d3a4b` / `#3f454b` shell palette.
- The always-visible tags bar was removed from the main shell.
- `src/components/ui/ConnectionPulse.vue` was added and used in the shell/dashboard/devices.
- `src/components/ui/ThemeSwitch.vue` was added and used in the header and public auth screens.
- `src/components/ui/CopyableText.vue` was added and used for device IDs.
- `src/components/ui/PageHeader.vue` and `src/components/ui/PageSection.vue` were added and used on Monitoring pages.
- `src/components/ui/DangerZone.vue` was added and used for advanced Server Commands.
- `src/components/ui/EmptyState.vue` and `src/components/ui/LoadingState.vue` were added for upcoming table/form standardization.
- `src/components/ui/FilterBar.vue` was added as the first table filter primitive.
- The dashboard now has a Quick Connect panel for native `rustdesk://` launch, web client launch, and device-list navigation.
- The admin Devices page now has a persistent Status column, ConnectionPulse online/offline state, copyable IDs, and compact Connect/More actions.
- Monitoring pages now share a page header/section structure across login history, connection history, file transfers, and shared sessions.
- Login History received `FilterBar` with user filter, collapsible filter panel, and integrated action buttons.
- Server Commands, Server Config, and GitHub Build settings now share the page header/section structure.
- Advanced Server Commands are visually separated in a Danger Zone and require confirmation before sending custom commands.
- Server command output now uses readonly terminal styling with target hint, Copy/Clear controls, and an empty-output placeholder.
- Address Book entries, collections, share rules, and tags now share the page header/section structure.
- Address Book device IDs use `CopyableText`; wide Access actions are reduced with `More` dropdowns where appropriate.
- Users, API Tokens, OAuth providers, Groups, and Device Groups now share the page header/section structure.
- Wide user actions are reduced with `More` dropdowns while keeping existing CRUD/composable behavior.
- Custom Client Builder and My Profile now share the page header/section structure.
- Custom Client preset/upload handlers are returned from `setup()` so the existing template controls are exposed at runtime.
- My Devices, My Address Book, My Address Book Collections, My Tags, My Shared Sessions, and My Login History now share the page header/section structure.
- Personal device/address-book IDs use `CopyableText` where copy actions already existed.
- The 404 page now uses the shared empty-state primitive and links back to Dashboard.
- Login, register, OAuth approval, and OAuth binding screens were moved to the token-based visual direction and support the theme switch.
- Mobile navigation now uses an `el-drawer`; the header toggle opens the drawer on mobile and collapses the sidebar on desktop.
- `npm run build` passes after installing `admin-ui` dependencies.

Still pending:

- `DataTable`, `AppDialog`, and the rest of the shared design-system components.
- Full table/form/dialog unification across CRUD views.
- Full i18n coverage for new dashboard/auth hero copy.
- Remaining form/dialog standards still need shared primitives and validation/loading unification.
- Monitoring Connection, File Transfer, and Shared Sessions pages need FilterBar.

## 30. Recommended First Implementation Step

Start with foundation only:

1. create theme tokens;
2. create `useTheme`;
3. create `ThemeSwitch`;
4. replace hardcoded colors in layout;
5. update sidebar/header;
6. verify light/dark/auto;
7. run `npm run build`.

Only after foundation is stable, start redesigning Dashboard and Devices.
