# DeskForge

A unified, self-hosted, RustDesk-compatible server with integrated API, Web Admin, and Web Client.

## Features

- **Rust Server** (hbbs + hbbr): ID server and relay server
- **Go API Server**: User management, authentication, address book
- **Web Admin** (`admin-ui/`): Vue 3 management dashboard with redesigned UI
- **Web Client**: Browser-based remote desktop client
- **s6-overlay**: Process supervision and automatic restarts

## Web Admin

The admin panel has been fully reworked (2026-06).

**Tech stack:** Vue 3.5, Vite 6, Element Plus 2.8, Pinia, Vue Router, Axios, Sass

**Current state:**
- Light / Dark / Auto theme modes with CSS variables design tokens
- Redesigned sidebar navigation (Dashboard, Devices, Access, Monitoring, Security, Server)
- Unified table component (`DataTable`) across all views
- Unified dialog/drawer components (`AppDialog`, `AppDrawer`)
- Shared filter bar (`FilterBar`) on monitoring pages
- Dashboard with Quick Connect panel
- Connection pulse status indicator
- Redesigned login/register screens
- Danger zone for server commands
- **Locales:** English, Russian, Chinese (Simplified)
- Custom client builder, OAuth/SSO, API tokens

### Screenshots

| Dashboard | Custom Client Builder |
| --- | --- |
| ![Dashboard](docs/screenshots/dashboard.png) | ![Custom Client Builder](docs/screenshots/client-builder.png) |

## Feature matrix

**Implemented:** username/password (bcrypt), JWT, per-device access tokens, captcha +
brute-force protection, OAuth/OIDC (GitHub, Google, generic), LDAP/LDAPS/AD; user CRUD with
admin/regular roles, groups, registration; peer CRUD, device groups, online status,
peer-UUID binding; personal + shared address books with collections, rules, tags, token-based
web-client sharing, batch ops; connection / file-transfer / login audit; server commands to
hbbs/hbbr; admin panel, web client, Swagger; SQLite / MySQL / PostgreSQL.

**Not implemented (vs RustDesk Pro):** 2FA/MFA, fine-grained RBAC, session recording, device
policy/assignment, remote script execution, unattended-access management, webhook/SIEM
integrations, HA/clustering, backup/restore, license-key management, custom branding,
policy-based device groups.

**Localization:** admin-ui ships English, Russian, Chinese (Simplified) — 3 locales under
`admin-ui/src/utils/i18n/`. The Go API message catalog (`api/resources/i18n/`) additionally
carries `zh_TW`, `ko`, `fr`, `es` — 7 locales total on the backend.

## Quick Start

### 1. Clone the repository

```bash
git clone https://github.com/bashrusakh/DeskForge.git
cd DeskForge
```

### 2. Configure environment

Edit `docker/docker-compose.yml` and replace:

- `your-server` with your server's IP or domain
- `your-secret-jwt-key-change-this` with a secure random string

### 3. Start the server

```bash
cd docker
docker compose up -d
```

### 4. Get the public key

```bash
docker compose logs | grep "Public Key"
```

Copy the key from `id_ed25519.pub`.

### 5. Access Web Admin

Open `http://your-server:21114/admin/` in your browser.

Default credentials:
- Username: `admin`
- Password: Check Docker logs for the generated password

### 6. Configure RustDesk Client

In your RustDesk client settings:

- **ID Server**: `your-server:21116`
- **Relay Server**: `your-server:21117`
- **API Server**: `http://your-server:21114`
- **Key**: Paste the public key from step 4

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `RELAY` | Relay server address | `relay.example.com` |
| `ENCRYPTED_ONLY` | Only allow encrypted connections | `0` |
| `MUST_LOGIN` | Require login to connect | `N` |
| `TZ` | Timezone | `UTC` |
| `RUSTDESK_API_RUSTDESK_ID_SERVER` | ID server address | - |
| `RUSTDESK_API_RUSTDESK_RELAY_SERVER` | Relay server address | - |
| `RUSTDESK_API_RUSTDESK_API_SERVER` | API server URL | - |
| `RUSTDESK_API_KEY_FILE` | Path to public key file | `/data/id_ed25519.pub` |
| `RUSTDESK_API_JWT_KEY` | JWT secret key | - |

## Ports

| Port | Protocol | Description |
|------|----------|-------------|
| 21114 | TCP | API Server / Web Admin |
| 21115 | TCP | NAT type test |
| 21116 | TCP/UDP | ID Server |
| 21117 | TCP | Relay Server |
| 21118 | TCP | WebSocket |
| 21119 | TCP | Web Server |

## Building from Source

```bash
cd docker
docker compose build
```

## License

AGPL-3.0 — See [LICENSE](LICENSE) for details.

## Credits

- [rustdesk/rustdesk-server](https://github.com/rustdesk/rustdesk-server) — Original RustDesk server
- [lejianwen/rustdesk-api](https://github.com/lejianwen/rustdesk-api) — Go API server
- [lejianwen/rustdesk-api-web](https://github.com/lejianwen/rustdesk-api-web) — Original web admin interface
