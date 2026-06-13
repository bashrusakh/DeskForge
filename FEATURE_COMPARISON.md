# RustDesk API Feature Comparison

## Fork: lejianwen/rustdesk-api vs RustDesk Pro

### What We Have (Go API fork)

| Category | Feature | Status |
|----------|---------|--------|
| **Auth** | Username/Password (bcrypt) | ✅ |
| **Auth** | JWT tokens | ✅ |
| **Auth** | Access tokens (per-device) | ✅ |
| **Auth** | Captcha + brute-force protection | ✅ |
| **Auth** | OAuth/OIDC (GitHub, Google, generic) | ✅ |
| **Auth** | LDAP + LDAPS + AD | ✅ |
| **Users** | User CRUD | ✅ |
| **Users** | Admin/regular roles | ✅ |
| **Users** | Groups | ✅ |
| **Users** | Enable/disable | ✅ |
| **Users** | Registration (configurable) | ✅ |
| **Devices** | Peer CRUD | ✅ |
| **Devices** | Device groups | ✅ |
| **Devices** | Online status | ✅ |
| **Devices** | Peer-UUID binding | ✅ |
| **Address Book** | Personal + shared | ✅ |
| **Address Book** | Collections + rules | ✅ |
| **Address Book** | Tags | ✅ |
| **Address Book** | Web client sharing (tokens, expiry) | ✅ |
| **Address Book** | Batch operations | ✅ |
| **Audit** | Connection audit | ✅ |
| **Audit** | File transfer audit | ✅ |
| **Audit** | Login logs | ✅ |
| **Server** | Send commands to hbbs/hbbr | ✅ |
| **Server** | Custom commands | ✅ |
| **Web** | Admin panel (`/admin`) | ✅ |
| **Web** | Web client (`/webclient`) | ✅ |
| **Web** | Swagger docs | ✅ |
| **Web** | i18n (7 languages) | ✅ |
| **DB** | SQLite / MySQL / PostgreSQL | ✅ |

### What's Missing (vs Pro)

| Category | Pro Feature | Status |
|----------|-------------|--------|
| **Security** | 2FA/MFA | ❌ |
| **Security** | Fine-grained RBAC | ❌ |
| **Security** | Session recording | ❌ |
| **Devices** | Device assignment/policy | ❌ |
| **Devices** | Remote script execution | ❌ |
| **Devices** | Unattended access management | ❌ |
| **Integrations** | Webhook/API integrations | ❌ |
| **Integrations** | Log export/SIEM | ❌ |
| **Infra** | High availability/clustering | ❌ |
| **Infra** | Backup/restore | ❌ |
| **License** | License key management | ❌ |
| **UI** | Custom branding | ❌ |
| **UI** | Advanced device groups with policies | ❌ |

### Available i18n Languages

- English (`en.toml`)
- Russian (`ru.toml`)
- Chinese Simplified (`zh_CN.toml`)
- Chinese Traditional (`zh_TW.toml`)
- Korean (`ko.toml`)
- French (`fr.toml`)
- Spanish (`es.toml`)

### Architecture

```
┌─────────────────────────────────────────────┐
│              Docker Container                │
│                                             │
│  ┌─────────┐  ┌─────────┐  ┌─────────────┐ │
│  │  hbbs   │  │  hbbr   │  │  Go API     │ │
│  │  (Rust) │  │  (Rust) │  │  (Gin)      │ │
│  │  :21116 │  │  :21117 │  │  :21114     │ │
│  │  :21118 │  │  :21119 │  │             │ │
│  └─────────┘  └─────────┘  └─────────────┘ │
│                                             │
│  s6-overlay manages all services            │
│  Web Admin + Web Client served by Go API    │
└─────────────────────────────────────────────┘
```

### API Endpoints Summary

**Admin** (`/api/admin/`):
- Auth: login, logout, captcha, login-options
- Users: CRUD, register, changePwd, groupUsers
- Groups: CRUD
- Device Groups: CRUD
- Tags: CRUD
- Address Book: CRUD, batchCreate, batchCreateFromPeers
- Address Book Collections: CRUD
- Address Book Collection Rules: CRUD
- Peers: CRUD, batchDelete, simpleData
- OAuth: CRUD, confirm, bind, unbind
- Audit: conn, file, login_log, share_record, user_token
- Server: sendCmd, cmdList, cmdCreate, cmdDelete
- Config: admin, server, app

**Client** (`/api/`):
- Auth: login, logout, login-options
- OIDC: auth, auth-query, callback
- User: info, currentUser
- Address Book: sync, personal, shared, tags
- Peer: info, server-config, server-config-v2
- Shared: shared-peer
