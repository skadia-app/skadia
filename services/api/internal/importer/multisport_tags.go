package importer

import (
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

func MultisportActivitySchemaTagsInput(format, source string, sessions []*model.ActivityData) shadowfs.ActivitySchemaTagsInput {
	input := shadowfs.ActivitySchemaTagsInput{
		Format:    format,
		Source:    source,
		SportType: "multisport",
		Title:     "Multisport",
	}
	for _, session := range sessions {
		if session == nil {
			continue
		}
		if !session.StartedAt.IsZero() && (input.StartedAt.IsZero() || session.StartedAt.Before(input.StartedAt)) {
			input.StartedAt = session.StartedAt
		}
		input.DistanceM += session.DistanceM
		input.ElapsedTimeS += session.ElapsedTimeS
		input.ElevationGainM += session.ElevationGainM
		input.IsElectric = input.IsElectric || session.IsElectric
		input.IsIndoor = input.IsIndoor || session.IsIndoor
		input.IsVirtual = input.IsVirtual || session.IsVirtual
	}
	return input
}
