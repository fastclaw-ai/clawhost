# FastClaw

Kubernetes-native platform for managing and orchestrating [OpenClaw](https://openclaw.ai/) bot instances. Provides a RESTful API to create, deploy, and manage AI agent bots in a multi-tenant environment.

## Features

- **Bot Lifecycle** - Create, start, stop, restart, upgrade, and delete bot instances
- **Kubernetes Native** - Each bot runs as an isolated Pod with its own Service
- **Multi-tenant** - App-level isolation with per-user bot ownership
- **Skills** - Dynamically manage skills for running bots
- **IM Channels** - Connect bots to Telegram, Slack, Discord, Teams, LINE, Feishu, etc.
- **Device Pairing** - Approve and manage device access with auto-approval
- **Model Providers** - Configure multiple AI providers (Anthropic, OpenAI, MiniMax, etc.)
- **Proxy** - HTTP and WebSocket proxy to bot instances, with subdomain routing

## Architecture

```
┌────────┐       ┌───────────────────────┐       ┌─────────────────────┐
│ Client │──────▶│       FastClaw        │──────▶│    K8s Cluster      │
└────────┘       │                       │       │                     │
    │            │  ┌─────────┐ ┌──────┐ │       │  ┌───────────────┐  │
    │  Subdomain │  │ Bot API │ │Admin │ │  K8s  │  │ OpenClaw Pod  │  │
    │  Routing   │  │         │ │ API  │ │  API  │  │               │  │
    └───────────▶│  └─────────┘ └──────┘ │◀────▶│  │  Gateway      │  │
                 │  ┌──────────────────┐ │       │  │  IM Channels  │  │
                 │  │ Proxy (HTTP/WS)  │─┼──────▶│  │  Devices      │  │
                 │  └──────────────────┘ │       │  └───────────────┘  │
                 │           │           │       │  ┌───────────────┐  │
                 └───────────┼───────────┘       │  │  Shared PVC   │  │
                             │                   │  └───────────────┘  │
                       ┌─────┴─────┐             └─────────────────────┘
                       │PostgreSQL │
                       └───────────┘
```

Each bot runs as an isolated K8s Pod with its own Deployment + Service. FastClaw manages the full lifecycle and proxies all traffic via subdomain routing, no per-bot Ingress needed. Bot config is synced bidirectionally between PostgreSQL and the pod.

## Prerequisites

- Go 1.24+ (for building from source)
- Kubernetes cluster (1.28+) - locally via [OrbStack](https://orbstack.dev/) or Docker Desktop
- `kubectl` and optionally `helm` (v3)

> FastClaw does **not** need to run inside the K8s cluster. It only needs a kubeconfig that can reach the cluster API.

## Quick Start

### Option A: Helm Install (recommended)

Build the image locally first, then deploy everything (FastClaw + PostgreSQL + RBAC) into your K8s cluster:

```bash
docker build -t fastclaw:latest .

helm install fastclaw deploy/helm/fastclaw \
  -n fastclaw --create-namespace \
  --set adminToken="my-admin-token" \
  --set domain.botDomain="fastclaw.loc"
```

Verify:

```bash
kubectl -n fastclaw get pods
kubectl -n fastclaw port-forward svc/fastclaw 18080:18080
curl http://localhost:18080/health
```

See [Helm values](#helm-chart) for full configuration options.

### Option B: kubectl Apply

```bash
# Create namespace and RBAC
kubectl apply -f deploy/k8s/namespace.yaml
kubectl apply -f deploy/k8s/rbac.yaml
kubectl apply -f deploy/k8s/pvc.yaml

# Create secrets (edit first!)
cp deploy/k8s/secrets.yaml.example deploy/k8s/secrets.yaml
# edit deploy/k8s/secrets.yaml with your tokens/passwords
kubectl apply -f deploy/k8s/secrets.yaml

# Deploy PostgreSQL and FastClaw
kubectl apply -f deploy/k8s/postgres.yaml
kubectl apply -f deploy/k8s/configmap.yaml
kubectl apply -f deploy/k8s/deployment.yaml

# Port-forward to access locally
kubectl -n fastclaw port-forward svc/fastclaw 18080:18080
```

### Option C: Local Binary

Run FastClaw on your host, connecting to a K8s cluster via kubeconfig.

```bash
git clone https://github.com/fastclaw-ai/fastclaw.git
cd fastclaw
go build -o fastclaw .
cp config.example.toml config.toml
```

Start a PostgreSQL instance:

```bash
docker run -d --name fastclaw-pg \
  -e POSTGRES_DB=fastclaw \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  -p 5432:5432 postgres:16
```

Prepare K8s namespace and storage:

```bash
kubectl create namespace fastclaw
kubectl apply -f deploy/k8s/pvc.yaml
```

Edit `config.toml` - key settings for local dev:

```toml
[kubernetes]
local_dev = true    # use ClusterIP for direct pod access from host

[api]
admin_token = "my-admin-token"
```

Run:

```bash
./fastclaw server
curl http://localhost:18080/health
```

### Create Your First Bot

Once the server is running (via any option above):

```bash
# 1. Create an App (each app gets its own API token)
curl -s -X POST http://localhost:18080/bot/api/v1/admin/apps \
  -H "Authorization: Bearer my-admin-token" \
  -H "Content-Type: application/json" \
  -d '{"name": "my-app"}'
# Save the api_token from the response

export API_TOKEN="<api_token>"

# 2. Create a Bot
curl -s -X POST http://localhost:18080/bot/api/v1/bots \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user-001",
    "name": "my-bot",
    "slug": "my-bot",
    "config": {
      "model": "claude-sonnet-4-20250514",
      "api_key": "sk-ant-xxx"
    }
  }'

export BOT_ID="<id>"

# 3. Start the Bot (creates K8s Deployment + Service)
curl -X POST http://localhost:18080/bot/api/v1/bots/$BOT_ID/start \
  -H "Authorization: Bearer $API_TOKEN"

# 4. Check status
curl http://localhost:18080/bot/api/v1/bots/$BOT_ID/status \
  -H "Authorization: Bearer $API_TOKEN"

# 5. Access via proxy
curl http://localhost:18080/proxy/$BOT_ID/

# Stop / Restart / Delete
curl -X POST http://localhost:18080/bot/api/v1/bots/$BOT_ID/stop -H "Authorization: Bearer $API_TOKEN"
curl -X POST http://localhost:18080/bot/api/v1/bots/$BOT_ID/restart -H "Authorization: Bearer $API_TOKEN"
curl -X DELETE http://localhost:18080/bot/api/v1/bots/$BOT_ID -H "Authorization: Bearer $API_TOKEN"
```
### Web UI

FastClaw includes a lightweight built-in web console for common API operations.

- URL: `http://localhost:18080/ui`
- Supports: health check, admin app create/list/reset-token, bot create/list/get/status/start/stop/restart/delete
- Auth: provide admin token and app token in the page

### Local Subdomain Routing (Caddy)

Bot WebUI is accessed via subdomains (e.g., `my-bot.fastclaw.loc`). For local development, use [Caddy](https://caddyserver.com/) to handle TLS and route all subdomains to FastClaw.

Create a `Caddyfile` in the project root:

```caddyfile
# API
fastclaw.loc {
    tls internal
    reverse_proxy localhost:18080
}

# Bot subdomains
*.fastclaw.loc {
    tls internal
    reverse_proxy localhost:18080
}
```

Run Caddy:

```bash
caddy run
```

Then add DNS entries to `/etc/hosts` (or use a local DNS like dnsmasq):

```
127.0.0.1  fastclaw.loc
```

> For wildcard `*.fastclaw.loc`, `/etc/hosts` doesn't support wildcards. Use [dnsmasq](https://thekelleys.org.uk/dnsmasq/doc.html) or set up a local DNS resolver. On macOS with OrbStack, you can also use `*.orb.local` domains directly.

## Deployment

### Helm Chart

```bash
helm install fastclaw deploy/helm/fastclaw \
  -n fastclaw --create-namespace \
  --set adminToken="my-secret-token"
```

Key values (`deploy/helm/fastclaw/values.yaml`):

| Parameter                  | Default                  | Description                                                              |
| -------------------------- | ------------------------ | ------------------------------------------------------------------------ |
| `adminToken`               | `change-me`              | Admin API token                                                          |
| `server.image.repository`  | `fastclaw`               | FastClaw image                                                           |
| `server.image.tag`         | `latest`                 | Image tag                                                                |
| `server.replicas`          | `1`                      | Number of replicas                                                       |
| `postgresql.enabled`       | `true`                   | Deploy built-in PostgreSQL                                               |
| `postgresql.auth.password` | `postgres`               | DB password                                                              |
| `externalDatabase.host`    | `""`                     | External DB host (when `postgresql.enabled=false`)                       |
| `storage.size`             | `10Gi`                   | Shared PVC size for bot data                                             |
| `openclaw.image`           | `1panel/openclaw:latest` | OpenClaw bot image                                                       |
| `openclaw.cpuLimit`        | `2000m`                  | Bot CPU limit                                                            |
| `openclaw.memoryLimit`     | `4Gi`                    | Bot memory limit                                                         |
| `domain.botDomain`         | `fastclaw.ai`            | Domain for bot subdomains and API (unless `apiDomain` is set separately) |
| `ingress.enabled`          | `false`                  | Enable ingress                                                           |

Use an external database:

```bash
helm install fastclaw deploy/helm/fastclaw \
  -n fastclaw --create-namespace \
  --set adminToken="my-token" \
  --set postgresql.enabled=false \
  --set externalDatabase.host="db.example.com" \
  --set externalDatabase.password="secret"
```

Upgrade:

```bash
helm upgrade fastclaw deploy/helm/fastclaw -n fastclaw
```

Uninstall:

```bash
helm uninstall fastclaw -n fastclaw
```

### Raw K8s Manifests

All manifests are in `deploy/k8s/`:

| File                   | Description                       |
| ---------------------- | --------------------------------- |
| `namespace.yaml`       | Namespace                         |
| `rbac.yaml`            | ServiceAccount, Role, RoleBinding |
| `pvc.yaml`             | Shared storage for bot data       |
| `postgres.yaml`        | PostgreSQL Deployment + Service   |
| `secrets.yaml.example` | Secret template (copy and edit)   |
| `configmap.yaml`       | FastClaw config.toml              |
| `deployment.yaml`      | FastClaw Deployment + Service     |

### Docker

```bash
docker build -t fastclaw:latest .
docker run -p 18080:18080 -v ./config.toml:/app/config.toml fastclaw:latest
```

## API Reference

All routes are prefixed with `/bot/api/v1` and require `Authorization: Bearer <token>`.

### Health Check

```
GET /health
```

### Admin - Apps

| Method | Endpoint                                 | Description         |
| ------ | ---------------------------------------- | ------------------- |
| POST   | `/bot/api/v1/admin/apps`                 | Create app          |
| GET    | `/bot/api/v1/admin/apps`                 | List apps           |
| GET    | `/bot/api/v1/admin/apps/:id`             | Get app             |
| PUT    | `/bot/api/v1/admin/apps/:id`             | Update app          |
| DELETE | `/bot/api/v1/admin/apps/:id`             | Delete app          |
| POST   | `/bot/api/v1/admin/apps/:id/reset-token` | Reset app API token |

### Admin - Bot Upgrade

| Method | Endpoint                             | Description          |
| ------ | ------------------------------------ | -------------------- |
| POST   | `/bot/api/v1/admin/bots/upgrade`     | Upgrade all bots     |
| POST   | `/bot/api/v1/admin/bots/:id/upgrade` | Upgrade specific bot |

### Bots

| Method | Endpoint                           | Description         |
| ------ | ---------------------------------- | ------------------- |
| POST   | `/bot/api/v1/bots`                 | Create bot          |
| GET    | `/bot/api/v1/bots?user_id=xxx`     | List bots           |
| GET    | `/bot/api/v1/bots/:id`             | Get bot             |
| PUT    | `/bot/api/v1/bots/:id`             | Update bot          |
| DELETE | `/bot/api/v1/bots/:id`             | Delete bot          |
| POST   | `/bot/api/v1/bots/:id/start`       | Start bot           |
| POST   | `/bot/api/v1/bots/:id/stop`        | Stop bot            |
| POST   | `/bot/api/v1/bots/:id/restart`     | Restart bot         |
| GET    | `/bot/api/v1/bots/:id/status`      | Get bot status      |
| GET    | `/bot/api/v1/bots/:id/connect`     | Get connection info |
| POST   | `/bot/api/v1/bots/:id/reset-token` | Reset bot token     |

### Skills

| Method | Endpoint                            | Description  |
| ------ | ----------------------------------- | ------------ |
| GET    | `/bot/api/v1/bots/:id/skills`       | List skills  |
| PUT    | `/bot/api/v1/bots/:id/skills/:name` | Upsert skill |
| DELETE | `/bot/api/v1/bots/:id/skills/:name` | Delete skill |

### IM Channels

| Method | Endpoint                                                 | Description           |
| ------ | -------------------------------------------------------- | --------------------- |
| POST   | `/bot/api/v1/bots/:id/channels`                          | Add channel           |
| GET    | `/bot/api/v1/bots/:id/channels`                          | List channels         |
| DELETE | `/bot/api/v1/bots/:id/channels/:channel`                 | Remove channel        |
| GET    | `/bot/api/v1/bots/:id/channels/:channel/pairing`         | List pairing requests |
| POST   | `/bot/api/v1/bots/:id/channels/:channel/pairing/approve` | Approve pairing       |
| POST   | `/bot/api/v1/bots/:id/channels/:channel/pairing/revoke`  | Revoke pairing        |
| GET    | `/bot/api/v1/bots/:id/channels/:channel/pairing/users`   | Get paired users      |

### Devices

| Method | Endpoint                                           | Description    |
| ------ | -------------------------------------------------- | -------------- |
| GET    | `/bot/api/v1/bots/:id/devices`                     | List devices   |
| POST   | `/bot/api/v1/bots/:id/devices/:request_id/approve` | Approve device |
| DELETE | `/bot/api/v1/bots/:id/devices/:device_id`          | Revoke device  |

### Model Providers

| Method | Endpoint                                       | Description     |
| ------ | ---------------------------------------------- | --------------- |
| GET    | `/bot/api/v1/bots/:id/config/models`           | List providers  |
| POST   | `/bot/api/v1/bots/:id/config/models`           | Add provider    |
| GET    | `/bot/api/v1/bots/:id/config/models/:provider` | Get provider    |
| PUT    | `/bot/api/v1/bots/:id/config/models/:provider` | Update provider |
| DELETE | `/bot/api/v1/bots/:id/config/models/:provider` | Delete provider |

### Agent Defaults

| Method | Endpoint                               | Description        |
| ------ | -------------------------------------- | ------------------ |
| GET    | `/bot/api/v1/bots/:id/config/defaults` | Get agent defaults |
| PUT    | `/bot/api/v1/bots/:id/config/defaults` | Set agent defaults |

### Proxy

| Method | Endpoint           | Description           |
| ------ | ------------------ | --------------------- |
| ANY    | `/proxy/:bot_id/*` | Proxy requests to bot |
| WS     | `/proxy/:bot_id/*` | WebSocket proxy       |

Subdomain routing: `{bot-id}.{bot_domain_suffix}/*` routes to the corresponding bot automatically.

## License

Apache License 2.0 - see [LICENSE](LICENSE).
