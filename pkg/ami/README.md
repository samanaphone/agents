# asterisk-ami

A Go library for communicating with the Asterisk Manager Interface (AMI) over HTTP.

Uses Asterisk's built-in HTTP server (`/rawman` endpoint) — no TCP socket management needed.

## Prerequisites

### 1. Enable HTTP in Asterisk (`/etc/asterisk/http.conf`)

```ini
[general]
enabled=yes
bindaddr=0.0.0.0
bindport=8088
```

### 2. Create an AMI user (`/etc/asterisk/manager.conf`)

```ini
[general]
enabled=yes
webenabled=yes        ; required for HTTP access

[admin]
secret=mysecret
read=all
write=all
permit=127.0.0.1/255.255.255.0
```

Reload after changes:
```bash
asterisk -rx "manager reload"
asterisk -rx "http restart"
```

## Installation

```bash
go get github.com/yourname/asterisk-ami
```

## Quick Start

```go
client := ami.NewClient(ami.Config{
    Host:     "192.168.1.100",
    Port:     8088,
    Username: "admin",
    Secret:   "mysecret",
})

// Ping
if err := client.Ping(ctx); err != nil {
    log.Fatal("Asterisk unreachable:", err)
}

// Originate a call
resp, err := client.Originate(ctx, ami.OriginateParams{
    Channel:  "PJSIP/1001",
    Exten:    "1002",
    Context:  "default",
    CallerID: "Click2Call <0000>",
})

// Any custom action
resp, err := client.Action(ctx, "DBGet", ami.Params{
    "Family": "myapp",
    "Key":    "setting1",
})
fmt.Println(resp.Get("Val"))
```

## Available Methods

| Method | AMI Action |
|---|---|
| `Ping` | Ping |
| `CoreSettings` | CoreSettings |
| `CoreStatus` | CoreStatus |
| `Originate` | Originate |
| `Hangup` | Hangup |
| `Status` | Status |
| `GetVar` | Getvar |
| `SetVar` | Setvar |
| `SIPPeers` | SIPpeers |
| `SIPShowPeer` | SIPshowpeer |
| `PJSIPShowEndpoints` | PJSIPShowEndpoints |
| `ExtensionState` | ExtensionState |
| `QueueStatus` | QueueStatus |
| `QueueAdd` | QueueAdd |
| `QueueRemove` | QueueRemove |
| `QueuePause` | QueuePause |
| `VoicemailUsersList` | VoicemailUsersList |
| `ConfbridgeList` | ConfbridgeList |
| `ConfbridgeKick` | ConfbridgeKick |
| `Reload` | Reload |
| `Command` | Command |
| `Action` | Any action |

## File Structure

```
.
├── ami/
│   ├── client.go    # HTTP client, response parser, Config, Params types
│   └── actions.go   # High-level action methods
├── examples/
│   └── main.go      # Usage examples
└── go.mod
```
