# Agents Dashboard

A production-ready Go web application that provides a real-time dashboard for monitoring and managing Asterisk call center agents and queues. Access is protected behind Google OAuth2 — only authorized users from your organization can log in.

> ⚠️ This site is restricted exclusively to authorized members of **Samana Group**. Unauthorized access is strictly prohibited.

---

## Features

- 🔐 **Google OAuth2 authentication** — secure login, only permitted accounts can access the dashboard
- 📊 **Queue monitoring** — real-time view of all Asterisk queues with stats (calls, completed, abandoned, hold time, talk time, service level)
- 👥 **Agent management** — view all queue members grouped by active/inactive status, with calls taken, last call time, device status, and pause state
- ⏸️ **Pause / Resume agents** — pause or resume individual agents per queue, or all agents at once
- 🚪 **Join / Leave queues** — dynamically add or remove agents from queues
- 🔄 **AMI over HTTP** — communicates with Asterisk via the HTTP Manager Interface (`/mxml`) using XML parsing
- 🐳 **Docker support** — includes `Dockerfile`, `docker-compose.yml`, and a `docker/` directory for containerized deployment

---

## Project Structure

```
.
├── cmd/
│   └── dashboard/
│       └── main.go          # HTTP server, routes, handlers, auth middleware
├── pkg/
│   └── ami/                 # Asterisk AMI HTTP client library
│       ├── client.go        # HTTP client, XML parser, Config and Params types
│       ├── actions.go       # High-level AMI action methods
│       ├── queue_member.go  # QueueMemberEvent struct and parser
│       └── queue_params.go  # QueueParamsEvent struct, parser, ParseQueueStatus
├── templates/               # Go HTML templates
├── static/                  # CSS and static assets
├── docker/                  # Docker helper files
├── Dockerfile
├── docker-compose.yml
├── Makefile
├── go.mod
├── go.sum
├── queues.json              # Queue name mapping configuration
└── .env.example             # Environment variable template
```

---

## Prerequisites

### 1. Asterisk HTTP AMI

Enable the HTTP manager interface in Asterisk:

`/etc/asterisk/http.conf`:
```ini
[general]
enabled=yes
bindaddr=0.0.0.0
bindport=8088
```

`/etc/asterisk/manager.conf`:
```ini
[general]
enabled=yes
webenabled=yes

[admin]
secret=your-secret
read=all
write=all
permit=0.0.0.0/0.0.0.0
```

Reload after changes:
```bash
asterisk -rx "manager reload"
asterisk -rx "http restart"
```

### 2. Google OAuth2 Credentials

1. Go to [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
2. Create a project (or select one)
3. Click **"Create Credentials" → "OAuth 2.0 Client ID"**
4. Application type: **Web application**
5. Add Authorized Redirect URI: `http://localhost:8080/auth/callback`
6. Copy the **Client ID** and **Client Secret**

---

## Configuration

Copy the example environment file and fill in your values:

```bash
cp .env.example .env
```

`.env` variables:

| Variable | Description | Default |
|---|---|---|
| `GOOGLE_CLIENT_ID` | Google OAuth2 Client ID | — |
| `GOOGLE_CLIENT_SECRET` | Google OAuth2 Client Secret | — |
| `REDIRECT_URL` | OAuth2 callback URL | `http://localhost:8080/auth/callback` |
| `SESSION_KEY` | Secret key for signing session cookies | — |
| `AMI_HOST` | Asterisk server address | `localhost` |
| `AMI_PORT` | Asterisk HTTP port | `8088` |
| `AMI_USERNAME` | AMI username | — |
| `AMI_SECRET` | AMI password | — |
| `PORT` | HTTP server port | `8080` |

---

## Running

### Locally

```bash
export $(cat .env | xargs) && go run ./cmd/dashboard
```

Open [http://localhost:8080](http://localhost:8080)

### With Make

```bash
make run
```

### With Docker Compose

```bash
docker-compose up --build
```

---

## Routes

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | Public home page |
| `GET` | `/login` | Redirects to Google login |
| `GET` | `/auth/callback` | Google OAuth2 callback |
| `GET` | `/logout` | Clears session and logs out |
| `GET` | `/dashboard` | Main queue and agent dashboard (protected) |
| `GET` | `/pausemember/:queue` | Pause agent in a specific queue (protected) |
| `GET` | `/resumemember/:queue` | Resume agent in a specific queue (protected) |
| `GET` | `/pausemember/all` | Pause agent in all queues (protected) |
| `GET` | `/resumemember/all` | Resume agent in all queues (protected) |
| `GET` | `/joinqueue/:queue` | Join a queue dynamically (protected) |
| `GET` | `/leavequeue/:queue` | Leave a queue dynamically (protected) |

---

## AMI Package

The `pkg/ami` package is a standalone Asterisk AMI HTTP client that can be imported independently. It targets the `/mxml` endpoint and parses XML responses using `encoding/xml`.

```go
client := ami.NewClient(ami.Config{
    Host:     "192.168.1.100",
    Port:     8088,
    Username: "admin",
    Secret:   "mysecret",
})

// Get all queue statuses with members
resp, _ := client.QueueStatus(ctx)
queues := ami.ParseQueueStatus(resp)

for _, q := range queues {
    fmt.Printf("Queue: %s  calls=%d  members=%d\n", q.Queue, q.Calls, len(q.Members))
}
```

---

## Production Checklist

- [ ] Set `SESSION_KEY` to a long random string
- [ ] Set `Secure: true` on session cookie options (requires HTTPS)
- [ ] Add your production domain to Google Console Authorized Redirect URIs
- [ ] Set `REDIRECT_URL` to your production callback URL
- [ ] Restrict AMI `permit` in `manager.conf` to your server's IP only
- [ ] Run behind a reverse proxy (nginx / Caddy) with TLS