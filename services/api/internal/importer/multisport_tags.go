package importer

import (
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

func ActivitySchemaTagsInputFromData(format, source string, data *model.ActivityData) shadowfs.ActivitySchemaTagsInput {
	input := shadowfs.ActivitySchemaTagsInput{
		Format: format,
		Source: source,
	}
	if data == nil {
		return input
	}
	input.StartedAt = data.StartedAt
	input.SportType = data.SportType
	input.SubType = data.SubType
	input.Title = data.Title
	input.DistanceM = data.DistanceM
	input.ElapsedTimeS = data.ElapsedTimeS
	input.ElevationGainM = data.ElevationGainM
	input.IsElectric = data.IsElectric
	input.IsIndoor = data.IsIndoor
	input.IsVirtual = data.IsVirtual
	input.IsCommute = data.IsCommute
	return input
}

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
