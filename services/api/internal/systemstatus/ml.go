package systemstatus

import (
	"strconv"
	"time"
)

type NATSSnapshot struct {
	Configured             bool
	Connected              bool
	JetStreamOK            bool
	IntrospectionAvailable bool
	StreamPresent          bool
	ConsumersPresent       bool
	OldestPendingAgeSec    int64
	PendingMessages        int
	Redelivered            int
	DisabledConsumerCount  int
	CheckedAt              time.Time
}

type MLSnapshot struct {
	Configured   bool
	Live         bool
	Ready        bool
	Version      string
	MissingRoles []string
	StorageReady bool
	CheckedAt    time.Time
}

func NATSStatusFromSnapshot(s NATSSnapshot) ComponentStatus {
	st := ComponentStatus{
		Key:       "nats",
		Label:     "NATS / JetStream",
		State:     StatusOK,
		CheckedAt: checkedAtOrNow(s.CheckedAt),
		Details: map[string]string{
			"pending_message_count":      strconv.Itoa(s.PendingMessages),
			"redelivered_count":          strconv.Itoa(s.Redelivered),
			"disabled_consumer_count":    strconv.Itoa(s.DisabledConsumerCount),
			"oldest_pending_age_seconds": strconv.FormatInt(s.OldestPendingAgeSec, 10),
		},
	}
	if !s.Configured && !s.Connected {
		st.State = StatusNotConfigured
		st.ReasonCode = "nats_not_configured"
		return st
	}
	if !s.Connected || !s.JetStreamOK {
		st.State = StatusUnavailable
		st.ReasonCode = "nats_unavailable"
		return st
	}
	if !s.IntrospectionAvailable {
		st.State = StatusUnknown
		st.ReasonCode = "jetstream_introspection_unavailable"
		return st
	}
	if !s.StreamPresent {
		st.State = StatusDegraded
		st.ReasonCode = "jetstream_stream_missing"
		return st
	}
	if !s.ConsumersPresent {
		st.State = StatusDegraded
		st.ReasonCode = "jetstream_consumers_missing"
		return st
	}
	if s.OldestPendingAgeSec > 3600 || s.Redelivered > 0 {
		st.State = StatusDegraded
		st.ReasonCode = "consumer_lag"
	}
	return st
}

func MLStatusFromSnapshot(s MLSnapshot) ComponentStatus {
	st := ComponentStatus{
		Key:       "ml",
		Label:     "ML service",
		State:     StatusOK,
		CheckedAt: checkedAtOrNow(s.CheckedAt),
		Details: map[string]string{
			"version":            s.Version,
			"missing_role_count": strconv.Itoa(len(s.MissingRoles)),
		},
	}
	if !s.Configured && !s.Live && !s.Ready {
		st.State = StatusDisabled
		st.ReasonCode = "ml_disabled"
		return st
	}
	if !s.Live {
		st.State = StatusUnavailable
		st.ReasonCode = "ml_unreachable"
		return st
	}
	if !s.Ready || len(s.MissingRoles) > 0 {
		st.State = StatusDegraded
		st.ReasonCode = "ml_roles_unavailable"
	}
	return st
}
