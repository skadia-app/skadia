package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

var recordHealthMetricInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "metric_kind": {"type": "string", "enum": ["hrv_rmssd", "sleep_hours", "weight_kg"]},
    "recorded_at_iso": {"type": "string"},
    "date_iso": {"type": "string"},
    "value_ms": {"type": "number", "exclusiveMinimum": 0},
    "total_minutes": {"type": "integer", "exclusiveMinimum": 0},
    "deep_minutes": {"type": "integer", "minimum": 0},
    "rem_minutes": {"type": "integer", "minimum": 0},
    "kg": {"type": "number", "exclusiveMinimum": 0},
    "body_fat_pct": {"type": "number", "minimum": 0, "maximum": 100}
  },
  "required": ["metric_kind"],
  "additionalProperties": false
}`)

type recordHealthMetricArgs struct {
	MetricKind    string   `json:"metric_kind"`
	RecordedAtISO string   `json:"recorded_at_iso"`
	DateISO       string   `json:"date_iso"`
	ValueMS       *float64 `json:"value_ms"`
	TotalMinutes  *int     `json:"total_minutes"`
	DeepMinutes   *int     `json:"deep_minutes"`
	RemMinutes    *int     `json:"rem_minutes"`
	KG            *float64 `json:"kg"`
	BodyFatPct    *float64 `json:"body_fat_pct"`
}

func RecordHealthMetricTool() *Tool {
	return &Tool{
		Name:                 "record_health_metric",
		Description:          "Record a manual health metric after user confirmation.",
		InputSchema:          recordHealthMetricInputSchema,
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		IsUserOwned:          true,
		SummaryForUser:       summarizeRecordHealthMetric,
		ExecuteWrite:         executeRecordHealthMetric,
	}
}

func executeRecordHealthMetric(ctx context.Context, deps *WriteExecutionDeps, userID string, raw json.RawMessage) (any, error) {
	if err := validateArgs("record_health_metric", recordHealthMetricInputSchema, raw); err != nil {
		return nil, err
	}
	var args recordHealthMetricArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	switch args.MetricKind {
	case "hrv_rmssd":
		if args.ValueMS == nil || *args.ValueMS <= 0 {
			return nil, fmt.Errorf("INVALID_VALUE")
		}
		measuredAt, err := time.Parse(time.RFC3339, args.RecordedAtISO)
		if err != nil {
			return nil, fmt.Errorf("INVALID_RECORDED_AT")
		}
		title := "Manueller HRV-Eintrag"
		if err := deps.DB.UpsertHealthHRVSnapshotTx(ctx, deps.Tx, &model.HealthHRVSnapshot{
			UserID:        userID,
			MeasuredAtUTC: measuredAt.UTC(),
			RmssdMS:       args.ValueMS,
			Title:         &title,
			SourceType:    "manual",
		}); err != nil {
			return nil, err
		}
		return map[string]any{"affected_id": measuredAt.UTC().Format(time.RFC3339), "metric_kind": args.MetricKind, "status": "ok"}, nil
	case "sleep_hours":
		if args.TotalMinutes == nil || *args.TotalMinutes <= 0 {
			return nil, fmt.Errorf("INVALID_VALUE")
		}
		if _, err := time.Parse("2006-01-02", args.DateISO); err != nil {
			return nil, fmt.Errorf("INVALID_DATE")
		}
		deepSeconds := optionalMinutesToSeconds(args.DeepMinutes)
		remSeconds := optionalMinutesToSeconds(args.RemMinutes)
		lightSeconds := intPtr(*args.TotalMinutes * 60)
		if deepSeconds != nil {
			*lightSeconds -= *deepSeconds
		}
		if remSeconds != nil {
			*lightSeconds -= *remSeconds
		}
		if *lightSeconds < 0 {
			return nil, fmt.Errorf("INVALID_SLEEP_STAGES")
		}
		if err := deps.DB.UpsertHealthSleepTx(ctx, deps.Tx, &model.HealthSleep{
			UserID:       userID,
			CalendarDate: args.DateISO,
			DeepSeconds:  deepSeconds,
			LightSeconds: lightSeconds,
			RemSeconds:   remSeconds,
			SourceType:   "manual",
		}); err != nil {
			return nil, err
		}
		return map[string]any{"affected_id": args.DateISO, "metric_kind": args.MetricKind, "status": "ok"}, nil
	case "weight_kg":
		if args.KG == nil || *args.KG <= 0 {
			return nil, fmt.Errorf("INVALID_VALUE")
		}
		if _, err := time.Parse("2006-01-02", args.DateISO); err != nil {
			return nil, fmt.Errorf("INVALID_DATE")
		}
		measuredAt, _ := time.Parse(time.RFC3339, args.DateISO+"T12:00:00Z")
		if err := deps.DB.UpsertHealthWeightEntryTx(ctx, deps.Tx, &model.HealthWeightEntry{
			UserID:        userID,
			MeasuredAtUTC: measuredAt,
			WeightG:       int(math.Round(*args.KG * 1000)),
			BodyFatPct:    args.BodyFatPct,
			SourceType:    "manual",
		}); err != nil {
			return nil, err
		}
		return map[string]any{"affected_id": args.DateISO, "metric_kind": args.MetricKind, "status": "ok"}, nil
	default:
		return nil, fmt.Errorf("INVALID_METRIC_KIND")
	}
}

func summarizeRecordHealthMetric(_ context.Context, _ *Deps, _ string, raw json.RawMessage) (string, error) {
	var args recordHealthMetricArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}
	date := args.DateISO
	value := ""
	switch args.MetricKind {
	case "hrv_rmssd":
		date = args.RecordedAtISO
		if args.ValueMS != nil {
			value = fmt.Sprintf("%.1f ms", *args.ValueMS)
		}
	case "sleep_hours":
		if args.TotalMinutes != nil {
			value = fmt.Sprintf("%d min", *args.TotalMinutes)
		}
	case "weight_kg":
		if args.KG != nil {
			value = fmt.Sprintf("%.2f kg", *args.KG)
		}
	}
	return fmt.Sprintf("%s = %s für %s eintragen", args.MetricKind, value, date), nil
}

func optionalMinutesToSeconds(minutes *int) *int {
	if minutes == nil {
		return nil
	}
	return intPtr(*minutes * 60)
}

func intPtr(v int) *int {
	return &v
}
