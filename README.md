# Go + Google OAuth2 Authentication

A minimal but production-ready Go web app that protects routes behind Google login.

## Features

- Google OAuth2 sign-in (PKCE-safe state parameter)
- Secure cookie-based sessions (`gorilla/sessions`)
- `requireAuth` middleware — wrap any handler to protect it
- Three pages: Home, Dashboard (protected), Profile (protected)
- Clean HTML/CSS UI — no JS frameworks needed

## Project Structure

```
.
├── main.go               # All server logic
├── go.mod
├── .env.example          # Copy → .env and fill in secrets
└── templates/
    ├── home.html         # Public landing page
    ├── dashboard.html    # Protected dashboard
    └── profile.html      # Protected profile page
```

## Setup

### 1. Create a Google OAuth App

1. Go to [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
2. Create a project (or select one)
3. Click **"Create Credentials" → "OAuth 2.0 Client ID"**
4. Application type: **Web application**
5. Add Authorized Redirect URI: `http://localhost:8080/auth/callback`
6. Copy the **Client ID** and **Client Secret**

### 2. Configure environment

```bash
cp .env.example .env
# Edit .env with your Client ID and Secret
```

### 3. Run

```bash
# Export env vars and run
export $(cat .env | xargs) && go run .
```

Or with [godotenv](https://github.com/joho/godotenv):

```bash
go run .   # if you add godotenv auto-loading
```

Open http://localhost:8080

## Adding Protected Routes

Wrap any handler with `requireAuth`:

```go
mux.Handle("/secret", requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    user := getSessionUser(r)
    fmt.Fprintf(w, "Hello %s, this is secret!", user.Name)
})))
```

## Production Checklist

- [ ] Set `SESSION_KEY` to a long random string
- [ ] Set `Secure: true` in `store.Options` (requires HTTPS)
- [ ] Add your production domain to Google Console Redirect URIs
- [ ] Set `REDIRECT_URL` to your production callback URL
