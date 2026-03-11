package ami

import (
	"strconv"
	"strings"
)

// MembershipType indicates whether the queue member was added statically
// (via queues.conf) or dynamically (via AMI/dialplan).
type MembershipType string

const (
	MembershipStatic  MembershipType = "static"
	MembershipDynamic MembershipType = "dynamic"
	MembershipRealtime MembershipType = "realtime"
)

// MemberStatus represents the numeric device state of a queue member
// as returned in the Status field of a QueueMember event.
// Values mirror the AST_DEVICE_* constants in Asterisk.
type MemberStatus int

const (
	MemberStatusUnknown      MemberStatus = 0 // Device state unknown
	MemberStatusNotInUse     MemberStatus = 1 // Not in use
	MemberStatusInUse        MemberStatus = 2 // Currently in use
	MemberStatusBusy         MemberStatus = 3 // Busy
	MemberStatusInvalid      MemberStatus = 4 // Invalid / unreachable
	MemberStatusUnavailable  MemberStatus = 5 // Unavailable
	MemberStatusRinging      MemberStatus = 6 // Ringing
	MemberStatusRingInUse    MemberStatus = 7 // Ring + in use
	MemberStatusOnHold       MemberStatus = 8 // On hold
)

// String returns a human-readable label for the MemberStatus.
func (s MemberStatus) String() string {
	switch s {
	case MemberStatusNotInUse:
		return "NotInUse"
	case MemberStatusInUse:
		return "InUse"
	case MemberStatusBusy:
		return "Busy"
	case MemberStatusInvalid:
		return "Invalid"
	case MemberStatusUnavailable:
		return "Unavailable"
	case MemberStatusRinging:
		return "Ringing"
	case MemberStatusRingInUse:
		return "RingInUse"
	case MemberStatusOnHold:
		return "OnHold"
	default:
		return "Unknown"
	}
}

// QueueMemberEvent represents an AMI "Event: QueueMember" stanza.
//
// This event is emitted as part of the QueueStatus action response —
// one QueueMemberEvent is returned per member assigned to a queue.
// It is also emitted by QueueMemberAdded, QueueMemberRemoved,
// QueueMemberPaused, QueueMemberPenalty, and QueueMemberStatus events.
//
// Example raw event (from /mxml <generic> attributes):
//
//	event="QueueMember"
//	queue="support"
//	name="Alice"
//	location="PJSIP/1001"
//	stateinterface="PJSIP/1001"
//	membership="dynamic"
//	penalty="0"
//	callstaken="5"
//	lastcall="1711234567"
//	lastpause="0"
//	logintime="1711230000"
//	incall="0"
//	status="1"
//	paused="0"
//	pausedreason=""
//	ringinuse="0"
//	wrapuptime="0"
type QueueMemberEvent struct {
	// Queue is the name of the queue this member belongs to.
	Queue string

	// Name is the display name of the queue member.
	// Corresponds to the AMI "Name" / "MemberName" field.
	Name string

	// Location is the channel technology or address of the member,
	// e.g. "PJSIP/1001" or "SIP/alice". This is the interface used to
	// place calls to the member.
	Location string

	// StateInterface is the channel technology or address used to read
	// the member's device state. Often identical to Location, but may
	// differ (e.g. a hint or a shared line).
	StateInterface string

	// Membership indicates how the member was added to the queue:
	// "static" (queues.conf), "dynamic" (AMI/dialplan), or "realtime".
	Membership MembershipType

	// Penalty is the member's penalty value. Members with a lower penalty
	// are preferred when distributing calls.
	Penalty int

	// CallsTaken is the total number of calls this member has answered
	// from this queue since Asterisk started (or the member last logged in).
	CallsTaken int

	// LastCall is the Unix timestamp (seconds since epoch) of the most
	// recent call this member completed. Zero if no calls have been taken.
	LastCall int64

	// LastPause is the Unix timestamp of the most recent time this member
	// was paused. Zero if the member has never been paused.
	LastPause int64

	// LoginTime is the Unix timestamp of when this member logged in to
	// the queue. Zero if not tracked.
	LoginTime int64

	// InCall indicates the member is currently handling a call (1 = yes, 0 = no).
	InCall bool

	// Status is the numeric device state of the member's channel.
	// Use the MemberStatus* constants for comparison.
	Status MemberStatus

	// Paused indicates whether the member is currently paused (true = paused).
	// Paused members do not receive new calls from the queue.
	Paused bool

	// PausedReason is the optional free-text reason supplied when the member
	// was paused (e.g. via QueuePause with a Reason parameter).
	PausedReason string

	// RingInUse controls whether Asterisk will ring this member even when
	// their device state shows InUse. true = ring anyway.
	RingInUse bool

	// WrapupTime is the number of seconds after completing a call during
	// which the member will not receive another call.
	WrapupTime int
}

// QueueMemberFromEvent parses a map of AMI XML attributes (as returned by
// Response.Events) into a QueueMemberEvent.
//
// Example usage:
//
//	resp, err := client.QueueStatus(ctx)
//	for _, attrs := range resp.Events {
//	    if strings.EqualFold(attrs["event"], "QueueMember") {
//	        member := ami.QueueMemberFromEvent(attrs)
//	        fmt.Printf("%s — status: %s\n", member.Name, member.Status)
//	    }
//	}
func QueueMemberFromEvent(attrs map[string]string) *QueueMemberEvent {
	// Helper to look up a key case-insensitively
	get := func(key string) string {
		for k, v := range attrs {
			if strings.EqualFold(k, key) {
				return v
			}
		}
		return ""
	}

	parseInt := func(key string) int {
		n, _ := strconv.Atoi(get(key))
		return n
	}

	parseInt64 := func(key string) int64 {
		n, _ := strconv.ParseInt(get(key), 10, 64)
		return n
	}

	parseBool := func(key string) bool {
		v := get(key)
		return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}

	// "Name" in older Asterisk versions, "MemberName" in newer ones
	name := get("MemberName")
	if name == "" {
		name = get("Name")
	}

	// "Location" in older versions, "Interface" in newer ones
	location := get("Interface")
	if location == "" {
		location = get("Location")
	}

	return &QueueMemberEvent{
		Queue:          get("Queue"),
		Name:           name,
		Location:       location,
		StateInterface: get("StateInterface"),
		Membership:     MembershipType(strings.ToLower(get("Membership"))),
		Penalty:        parseInt("Penalty"),
		CallsTaken:     parseInt("CallsTaken"),
		LastCall:       parseInt64("LastCall"),
		LastPause:      parseInt64("LastPause"),
		LoginTime:      parseInt64("LoginTime"),
		InCall:         parseBool("InCall"),
		Status:         MemberStatus(parseInt("Status")),
		Paused:         parseBool("Paused"),
		PausedReason:   get("PausedReason"),
		RingInUse:      parseBool("Ringinuse"),
		WrapupTime:     parseInt("Wrapuptime"),
	}
}