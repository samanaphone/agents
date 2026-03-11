package ami

import (
	"strconv"
	"strings"
)

// QueueStrategy represents the call distribution strategy of a queue,
// as reported in the QueueParams "Strategy" field.
type QueueStrategy string

const (
	QueueStrategyRingAll     QueueStrategy = "ringall"
	QueueStrategyLeastRecent QueueStrategy = "leastrecent"
	QueueStrategyFewestCalls QueueStrategy = "fewestcalls"
	QueueStrategyRandom      QueueStrategy = "random"
	QueueStrategyRRMemory    QueueStrategy = "rrmemory"
	QueueStrategyLinear      QueueStrategy = "linear"
	QueueStrategyWrappedUp   QueueStrategy = "wrandom"
)

// QueueParamsEvent represents an AMI "Event: QueueParams" stanza.
//
// This event is emitted as part of the QueueStatus action response —
// one QueueParamsEvent per configured queue, followed by zero or more
// QueueMemberEvent and QueueEntryEvent blocks, ending with QueueStatusComplete.
//
// Example raw event (from /mxml <generic> attributes):
//
//	event="QueueParams"
//	queue="support"
//	max="0"
//	strategy="rrmemory"
//	calls="3"
//	holdtime="45"
//	talktime="120"
//	completed="540"
//	abandoned="51"
//	servicelevel="60"
//	servicelevelperf="91.5"
//	servicelevelperf2="88.2"
//	weight="0"
type QueueParamsEvent struct {
	// Queue is the name of the queue.
	Queue string

	// Max is the maximum number of callers allowed to wait in the queue.
	// 0 means unlimited.
	Max int

	// Strategy is the call distribution strategy (e.g. "rrmemory", "ringall").
	Strategy QueueStrategy

	// Calls is the current number of callers waiting in the queue.
	Calls int

	// Holdtime is the average hold time (in seconds) for this queue,
	// calculated over completed calls.
	Holdtime int

	// TalkTime is the average talk time (in seconds) for completed calls
	// in this queue.
	TalkTime int

	// Completed is the total number of calls that have been successfully
	// answered and completed by a queue member.
	Completed int

	// Abandoned is the total number of calls that were hung up by the caller
	// before being answered.
	Abandoned int

	// ServiceLevel is the target service level threshold in seconds,
	// as configured in queues.conf (e.g. 60 = calls should be answered
	// within 60 seconds).
	ServiceLevel int

	// ServiceLevelPerf is the primary service level performance metric —
	// the percentage of calls answered within the ServiceLevel threshold.
	// Expressed as a float (e.g. 91.5 means 91.5%).
	ServiceLevelPerf float64

	// ServiceLevelPerf2 is the secondary service level performance metric.
	// Present in newer Asterisk versions; calculated differently from
	// ServiceLevelPerf (abandoned calls may be treated differently).
	ServiceLevelPerf2 float64

	// Weight is the queue's weight used for priority when a member belongs
	// to multiple queues. Higher weight queues are preferred.
	Weight int

	// Members is the list of queue members assigned to this queue,
	// populated by ParseQueueStatus when iterating a full QueueStatus response.
	Members []*QueueMemberEvent
}

// QueueParamsFromEvent parses a map of AMI XML attributes (as returned in
// Response.Events) into a QueueParamsEvent.
//
// For a full QueueStatus response that includes members, use ParseQueueStatus
// instead — it correlates QueueParams and QueueMember events automatically.
func QueueParamsFromEvent(attrs map[string]string) *QueueParamsEvent {
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

	parseFloat := func(key string) float64 {
		f, _ := strconv.ParseFloat(get(key), 64)
		return f
	}

	return &QueueParamsEvent{
		Queue:             get("Queue"),
		Max:               parseInt("Max"),
		Strategy:          QueueStrategy(strings.ToLower(get("Strategy"))),
		Calls:             parseInt("Calls"),
		Holdtime:          parseInt("Holdtime"),
		TalkTime:          parseInt("TalkTime"),
		Completed:         parseInt("Completed"),
		Abandoned:         parseInt("Abandoned"),
		ServiceLevel:      parseInt("ServiceLevel"),
		ServiceLevelPerf:  parseFloat("ServicelevelPerf"),
		ServiceLevelPerf2: parseFloat("ServicelevelPerf2"),
		Weight:            parseInt("Weight"),
		Members:           []*QueueMemberEvent{},
	}
}

// ParseQueueStatus processes the full Response.Events slice from a QueueStatus
// action and returns a slice of QueueParamsEvent, each with its Members list
// populated from the interleaved QueueMember events.
//
// Asterisk returns QueueStatus events in this order:
//
//	Event: QueueParams  (one per queue)
//	Event: QueueMember  (zero or more, belonging to the preceding QueueParams)
//	Event: QueueEntry   (zero or more callers waiting — not parsed here)
//	Event: QueueStatusComplete
//
// Example usage:
//
//	resp, err := client.QueueStatus(ctx)
//	if err != nil { log.Fatal(err) }
//
//	queues := ami.ParseQueueStatus(resp)
//	for _, q := range queues {
//	    fmt.Printf("Queue: %s  calls=%d  members=%d\n", q.Queue, q.Calls, len(q.Members))
//	    for _, m := range q.Members {
//	        fmt.Printf("  %-20s  status=%-12s  paused=%v\n", m.Name, m.Status, m.Paused)
//	    }
//	}
func ParseQueueStatus(resp *Response) []*QueueParamsEvent {
	var queues []*QueueParamsEvent

	for _, attrs := range resp.Events {
		var eventType string
		for k, v := range attrs {
			if strings.EqualFold(k, "event") {
				eventType = strings.ToLower(v)
				break
			}
		}

		switch eventType {
		case "queueparams":
			queues = append(queues, QueueParamsFromEvent(attrs))

		case "queuemember":
			if len(queues) == 0 {
				break
			}
			member := QueueMemberFromEvent(attrs)
			last := queues[len(queues)-1]
			// Attach to the last seen QueueParams with a matching queue name.
			// Falls back to appending to the most recent queue if names differ
			// (should not happen in practice).
			if strings.EqualFold(member.Queue, last.Queue) || last.Queue == "" {
				last.Members = append(last.Members, member)
			}
		}
	}

	return queues
}