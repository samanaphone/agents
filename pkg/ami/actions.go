package ami

import (
	"context"
	"fmt"
)

// ── Core ──────────────────────────────────────────────────────────────────────

// Login starts a session on asterisk manager.
func (c *Client) Login(ctx context.Context) error {
	resp, err := c.Action(ctx, "Login", map[string]string{
		"username": c.username,
		"secret": c.secret,
	})
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("ami: login failed: %s", resp.Get("Message"))
	}
	return nil
}

// Logoff ends a session on asterisk manager.
func (c *Client) Logoff(ctx context.Context) error {
	resp, err := c.Action(ctx, "Logoff", nil)
	if err != nil {
		return err
	}
	if resp.IsError() {
		return fmt.Errorf("ami: Logoff failed: %s", resp.Get("Message"))
	}
	return nil
}

// Ping sends a Ping action and returns nil if Asterisk responds with Pong.
func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.Action(ctx, "Ping", nil)
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("ami: ping failed: %s", resp.Get("Message"))
	}
	return nil
}

// CoreSettings returns Asterisk system info (version, max channels, etc).
func (c *Client) CoreSettings(ctx context.Context) (*Response, error) {
	return c.Action(ctx, "CoreSettings", nil)
}

// CoreStatus returns the current Asterisk runtime status.
func (c *Client) CoreStatus(ctx context.Context) (*Response, error) {
	return c.Action(ctx, "CoreStatus", nil)
}

// ── Channels ─────────────────────────────────────────────────────────────────

// OriginateParams holds parameters for an Originate action.
type OriginateParams struct {
	// Channel to call (e.g. "SIP/1001" or "PJSIP/1001")
	Channel string
	// Exten is the extension to dial after answer (used with Context + Priority)
	Exten string
	// Context for the dialplan
	Context string
	// Priority in the dialplan (default "1")
	Priority string
	// Application to run after answer instead of dialplan (e.g. "Playback")
	Application string
	// Data for the Application
	Data string
	// CallerID to set on the outbound call (e.g. "John <1001>")
	CallerID string
	// Timeout in milliseconds to wait for answer (default 30000)
	Timeout string
	// Async — if "true", the action returns immediately without waiting
	Async string
	// ActionID for correlating async responses
	ActionID string
	// Variables to set on the channel (e.g. "VAR1=val1,VAR2=val2")
	Variable string
}

// Originate initiates an outbound call.
func (c *Client) Originate(ctx context.Context, p OriginateParams) (*Response, error) {
	if p.Priority == "" {
		p.Priority = "1"
	}
	if p.Timeout == "" {
		p.Timeout = "30000"
	}
	if p.Async == "" {
		p.Async = "true"
	}

	params := Params{
		"Channel":  p.Channel,
		"Priority": p.Priority,
		"Timeout":  p.Timeout,
		"Async":    p.Async,
	}
	if p.Exten != "" {
		params["Exten"] = p.Exten
	}
	if p.Context != "" {
		params["Context"] = p.Context
	}
	if p.Application != "" {
		params["Application"] = p.Application
	}
	if p.Data != "" {
		params["Data"] = p.Data
	}
	if p.CallerID != "" {
		params["CallerID"] = p.CallerID
	}
	if p.ActionID != "" {
		params["ActionID"] = p.ActionID
	}
	if p.Variable != "" {
		params["Variable"] = p.Variable
	}

	return c.Action(ctx, "Originate", params)
}

// Hangup hangs up a channel by name.
func (c *Client) Hangup(ctx context.Context, channel string) (*Response, error) {
	return c.Action(ctx, "Hangup", Params{"Channel": channel})
}

// GetVar retrieves a channel variable.
func (c *Client) GetVar(ctx context.Context, channel, variable string) (string, error) {
	resp, err := c.Action(ctx, "Getvar", Params{
		"Channel":  channel,
		"Variable": variable,
	})
	if err != nil {
		return "", err
	}
	return resp.Get("Value"), nil
}

// SetVar sets a channel variable.
func (c *Client) SetVar(ctx context.Context, channel, variable, value string) (*Response, error) {
	return c.Action(ctx, "Setvar", Params{
		"Channel":  channel,
		"Variable": variable,
		"Value":    value,
	})
}

// Status returns the status of a channel, or all channels if channel is empty.
func (c *Client) Status(ctx context.Context, channel string) (*Response, error) {
	params := Params{}
	if channel != "" {
		params["Channel"] = channel
	}
	return c.Action(ctx, "Status", params)
}

// ── Queues ────────────────────────────────────────────────────────────────────

// QueueStatus returns the status of all queues (members, callers, stats).
func (c *Client) QueueStatus(ctx context.Context) (*Response, error) {
	return c.Action(ctx, "QueueStatus", nil)
}

// QueueAdd adds a member to a queue.
func (c *Client) QueueAdd(ctx context.Context, queue, iface, membername, stateinterface string) (*Response, error) {
	return c.Action(ctx, "QueueAdd", Params{
		"Queue":     queue,
		"Interface": iface,
		"MemberName": membername,
		"StateInterface": stateinterface,
	})
}

// QueueRemove removes a member from a queue.
func (c *Client) QueueRemove(ctx context.Context, queue, iface string) (*Response, error) {
	return c.Action(ctx, "QueueRemove", Params{
		"Queue":     queue,
		"Interface": iface,
	})
}

// QueuePause pauses or unpauses a queue member.
func (c *Client) QueuePause(ctx context.Context, queue, iface string, paused bool) (*Response, error) {
	p := "false"
	if paused {
		p = "true"
	}
	return c.Action(ctx, "QueuePause", Params{
		"Queue":     queue,
		"Interface": iface,
		"Paused":    p,
	})
}

// ── SIP / PJSIP ───────────────────────────────────────────────────────────────

// SIPPeers returns the list of SIP peers and their status.
func (c *Client) SIPPeers(ctx context.Context) (*Response, error) {
	return c.Action(ctx, "SIPpeers", nil)
}

// SIPShowPeer returns detailed info about a specific SIP peer.
func (c *Client) SIPShowPeer(ctx context.Context, peer string) (*Response, error) {
	return c.Action(ctx, "SIPshowpeer", Params{"Peer": peer})
}

// PJSIPShowEndpoints returns the list of PJSIP endpoints.
func (c *Client) PJSIPShowEndpoints(ctx context.Context) (*Response, error) {
	return c.Action(ctx, "PJSIPShowEndpoints", nil)
}

// ── Extensions ────────────────────────────────────────────────────────────────

// ExtensionState returns the state of a hint/extension.
func (c *Client) ExtensionState(ctx context.Context, exten, context string) (*Response, error) {
	return c.Action(ctx, "ExtensionState", Params{
		"Exten":   exten,
		"Context": context,
	})
}

// ── Voicemail ─────────────────────────────────────────────────────────────────

// VoicemailUsersListParams holds filters for VoicemailUsersList.
type VoicemailUsersListParams struct {
	Context string
}

// VoicemailUsersList lists all voicemail users.
func (c *Client) VoicemailUsersList(ctx context.Context) (*Response, error) {
	return c.Action(ctx, "VoicemailUsersList", nil)
}

// ── Meetme / Conferences ──────────────────────────────────────────────────────

// ConfbridgeList lists participants in a ConfBridge conference.
func (c *Client) ConfbridgeList(ctx context.Context, conference string) (*Response, error) {
	return c.Action(ctx, "ConfbridgeList", Params{"Conference": conference})
}

// ConfbridgeKick kicks a channel from a ConfBridge conference.
func (c *Client) ConfbridgeKick(ctx context.Context, conference, channel string) (*Response, error) {
	return c.Action(ctx, "ConfbridgeKick", Params{
		"Conference": conference,
		"Channel":    channel,
	})
}

// ── Reload ────────────────────────────────────────────────────────────────────

// Reload reloads an Asterisk module (e.g. "chan_sip", "app_queue").
// Pass an empty string to reload all modules.
func (c *Client) Reload(ctx context.Context, module string) (*Response, error) {
	params := Params{}
	if module != "" {
		params["Module"] = module
	}
	return c.Action(ctx, "Reload", params)
}

// Command executes an Asterisk CLI command and returns the output.
func (c *Client) Command(ctx context.Context, command string) (*Response, error) {
	return c.Action(ctx, "Command", Params{"Command": command})
}

func (c *Client) DBGet(ctx context.Context, family, key string) (*Response, error) {
	return c.Action(ctx, "DBGet", Params{
		"Family": family,
		"Key": key,
	})
}

func (c *Client) DBPut(ctx context.Context, family, key, value string) (*Response, error) {
	return c.Action(ctx, "DBPut", Params{
		"Family": family,
		"Key": key,
		"Val": value,
	})
}

func (c *Client) DBGetTree(ctx context.Context, family, key string) (*Response, error) {
	return c.Action(ctx, "DBGetTree", Params{
		"Family": family,
		"Key": key,
	})
}

func (c *Client) DBDelTree(ctx context.Context, family, key string) (*Response, error) {
	return c.Action(ctx, "DBDelTree", Params{
		"Family": family,
		"Key": key,
	})
}

