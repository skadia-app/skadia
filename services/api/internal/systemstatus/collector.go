package systemstatus

import (
	"context"
	"strconv"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
)

type ComponentCollector func(context.Context) ComponentStatus

type Collector struct {
	Now                 func() time.Time
	Build               map[string]BuildInfo
	ComponentCollectors []ComponentCollector
	FeatureMatrix       map[string]ComponentStatus
}

func (c Collector) Collect(ctx context.Context) SystemStatus {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	components := map[string]ComponentStatus{}
	componentList := make([]ComponentStatus, 0, len(c.ComponentCollectors))
	for _, collect := range c.ComponentCollectors {
		st := SanitizeComponentStatus(collect(ctx))
		components[st.Key] = st
		componentList = append(componentList, st)
	}
	return SystemStatus{
		SchemaVersion: 1,
		GeneratedAt:   now().UTC(),
		Overall:       overallFromComponents(componentList),
		Build:         c.Build,
		Components:    components,
		FeatureMatrix: c.FeatureMatrix,
	}
}

func overallFromComponents(components []ComponentStatus) StatusState {
	for _, st := range components {
		if st.State == StatusBlocked || st.State == StatusUnavailable || st.State == StatusDegraded {
			return StatusDegraded
		}
		if st.Required && (st.State == StatusMissingArtifacts || st.State == StatusUnknown || st.State == StatusNotConfigured) {
			return StatusDegraded
		}
	}
	return StatusOK
}

func FeatureMatrixFromCapabilities(caps capabilities.Capabilities) map[string]ComponentStatus {
	out := map[string]ComponentStatus{}
	for key, feature := range caps.Features {
		state := StatusOK
		if !feature.Configured {
			if feature.Reason == capabilities.ReasonNotConfigured {
				state = StatusNotConfigured
			} else {
				state = StatusDisabled
			}
		} else if !feature.Available {
			if feature.Reason == capabilities.ReasonMissingArtifacts {
				state = StatusMissingArtifacts
			} else {
				state = StatusDegraded
			}
		}
		out[key] = ComponentStatus{
			Key:        key,
			Label:      key,
			State:      state,
			ReasonCode: feature.Reason,
			CheckedAt:  time.Now().UTC(),
			Details: map[string]string{
				"configured": strconv.FormatBool(feature.Configured),
				"available":  strconv.FormatBool(feature.Available),
			},
		}
	}
	return out
}
