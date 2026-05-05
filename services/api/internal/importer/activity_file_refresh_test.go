package importer

import (
	"context"
	"reflect"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestWriteMetaCompanionAndRefreshActivityFileRunsRefreshAfterSidecar(t *testing.T) {
	ctx := context.Background()
	fileID := "file-from-activity"
	var calls []string

	s := &Service{
		metaCompanionWriter: func(ctx context.Context, userID, activityID, effectiveFileID string, data *model.ActivityData, activity *model.Activity) {
			calls = append(calls, "meta:"+effectiveFileID)
		},
		ActivityFileRefresher: func(ctx context.Context, userID, activityID, effectiveFileID string) {
			calls = append(calls, "refresh:"+effectiveFileID)
		},
	}

	s.writeMetaCompanionAndRefreshActivityFile(ctx, "user-1", "activity-1", "", &model.ActivityData{}, &model.Activity{FileID: &fileID})

	want := []string{"meta:file-from-activity", "refresh:file-from-activity"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}
