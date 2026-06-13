# full_Server

Unified RustDesk Server with integrated API, Web Admin, and Web Client.

## Features

- **Rust Server** (hbbs + hbbr): ID server and relay server
- **Go API Server**: User management, authentication, address book
- **Web Admin**: Management dashboard at `/admin/`
- **Web Client**: Browser-based remote desktop client
- **s6-overlay**: Process supervision and automatic restarts

## Quick Start

### 1. Clone the repository

```bash
git clone https://github.com/your-username/full_Server.git
cd full_Server
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

AGPL-3.0 - See [LICENSE](LICENSE) for details.

## Credits

- [rustdesk/rustdesk-server](https://github.com/rustdesk/rustdesk-server) - Original RustDesk server
- [lejianwen/rustdesk-api](https://github.com/lejianwen/rustdesk-api) - Go API server
- [lejianwen/rustdesk-api-web](https://github.com/lejianwen/rustdesk-api-web) - Web admin interface
