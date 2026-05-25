package event

import (
	"errors"
	"reflect"
	"testing"

	natsgo "github.com/nats-io/nats.go"
)

// Compile-time assertions: both *Publisher and *FakeInvalidator must satisfy
// the AnalyticsInvalidator interface so handlers can hold either one.
var _ AnalyticsInvalidator = (*Publisher)(nil)
var _ AnalyticsInvalidator = (*FakeInvalidator)(nil)

type fakeStreamManager struct {
	existing map[string]bool
	added    []natsgo.StreamConfig
}

func (f *fakeStreamManager) StreamInfo(stream string, opts ...natsgo.JSOpt) (*natsgo.StreamInfo, error) {
	if f.existing[stream] {
		return &natsgo.StreamInfo{Config: natsgo.StreamConfig{Name: stream}}, nil
	}
	return nil, errors.New("stream not found")
}

func (f *fakeStreamManager) AddStream(cfg *natsgo.StreamConfig, opts ...natsgo.JSOpt) (*natsgo.StreamInfo, error) {
	f.added = append(f.added, *cfg)
	return &natsgo.StreamInfo{Config: *cfg}, nil
}

func TestEnsureRequiredStreamsCreatesSplitSkadiaStreams(t *testing.T) {
	fake := &fakeStreamManager{existing: map[string]bool{}}

	ensureRequiredStreams(fake)

	if !reflect.DeepEqual(fake.added, requiredStreams) {
		t.Fatalf("added streams = %+v, want %+v", fake.added, requiredStreams)
	}
}

func TestEnsureRequiredStreamsSkipsExistingStreams(t *testing.T) {
	fake := &fakeStreamManager{existing: map[string]bool{"activity": true, "photo": true}}

	ensureRequiredStreams(fake)

	var addedNames []string
	for _, stream := range fake.added {
		addedNames = append(addedNames, stream.Name)
	}
	want := []string{"stage2", "document", "analytics"}
	if !reflect.DeepEqual(addedNames, want) {
		t.Fatalf("added streams = %v, want %v", addedNames, want)
	}
}
