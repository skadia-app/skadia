package systemstatus

import "testing"

func TestNATSStatusDegradesOnConsumerLag(t *testing.T) {
	got := NATSStatusFromSnapshot(NATSSnapshot{Connected: true, JetStreamOK: true, IntrospectionAvailable: true, StreamPresent: true, ConsumersPresent: true, OldestPendingAgeSec: 7200})
	if got.State != StatusDegraded || got.ReasonCode != "consumer_lag" {
		t.Fatalf("status = %+v", got)
	}
}

func TestNATSStatusDegradesOnMissingStreamOrConsumers(t *testing.T) {
	cases := []struct {
		name       string
		snap       NATSSnapshot
		wantReason string
	}{
		{name: "stream", snap: NATSSnapshot{Connected: true, JetStreamOK: true, IntrospectionAvailable: true, StreamPresent: false, ConsumersPresent: true}, wantReason: "jetstream_stream_missing"},
		{name: "consumers", snap: NATSSnapshot{Connected: true, JetStreamOK: true, IntrospectionAvailable: true, StreamPresent: true, ConsumersPresent: false}, wantReason: "jetstream_consumers_missing"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := NATSStatusFromSnapshot(tt.snap)
			if got.State != StatusDegraded || got.ReasonCode != tt.wantReason {
				t.Fatalf("status = %+v, want degraded %s", got, tt.wantReason)
			}
		})
	}
}

func TestNATSStatusUnknownWhenJetStreamIntrospectionUnavailable(t *testing.T) {
	got := NATSStatusFromSnapshot(NATSSnapshot{Connected: true, JetStreamOK: true, IntrospectionAvailable: false})
	if got.State != StatusUnknown || got.ReasonCode != "jetstream_introspection_unavailable" {
		t.Fatalf("status = %+v, want unknown jetstream_introspection_unavailable", got)
	}
}

func TestNATSStatusUnavailableWhenConfiguredButDisconnected(t *testing.T) {
	got := NATSStatusFromSnapshot(NATSSnapshot{Configured: true, Connected: false})
	if got.State != StatusUnavailable || got.ReasonCode != "nats_unavailable" {
		t.Fatalf("status = %+v, want unavailable nats_unavailable", got)
	}
}

func TestMLStatusSeparatesLiveFromRoleReadiness(t *testing.T) {
	got := MLStatusFromSnapshot(MLSnapshot{Configured: true, Live: true, Ready: true, StatusChecked: true, StatusAccessible: true, MissingRoles: []string{"document_extract"}})
	if got.State != StatusDegraded || got.ReasonCode != "ml_roles_unavailable" {
		t.Fatalf("status = %+v", got)
	}
}

func TestMLStatusDegradesWhenInternalStatusUnavailable(t *testing.T) {
	got := MLStatusFromSnapshot(MLSnapshot{Configured: true, Live: true, Ready: true, StatusChecked: true, StatusAccessible: false})
	if got.State != StatusDegraded || got.ReasonCode != "ml_status_unavailable" {
		t.Fatalf("status = %+v", got)
	}
}
