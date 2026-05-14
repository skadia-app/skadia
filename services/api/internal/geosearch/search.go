package geosearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type Source string

const (
	SourceLocal    Source = "local"
	SourceExternal Source = "external"
)

type ExternalPolicy string

const (
	ExternalPolicyDisabled      ExternalPolicy = "disabled"
	ExternalPolicyManualConfirm ExternalPolicy = "manual_confirm"
	ExternalPolicyAllowed       ExternalPolicy = "allowed"
)

type ExternalStatus string

const (
	ExternalStatusDisabled                 ExternalStatus = "disabled"
	ExternalStatusUnconfigured             ExternalStatus = "unconfigured"
	ExternalStatusAvailable                ExternalStatus = "available"
	ExternalStatusCached                   ExternalStatus = "cached"
	ExternalStatusUnreachable              ExternalStatus = "unreachable"
	ExternalStatusRateLimited              ExternalStatus = "rate_limited"
	ExternalStatusRequiresUserConfirmation ExternalStatus = "requires_user_confirmation"
	ExternalStatusSkippedByPrivacyPolicy   ExternalStatus = "skipped_by_privacy_policy"
	ExternalStatusSkipped                  ExternalStatus = "skipped"
)

type LocalStatus string

const (
	LocalStatusAvailable   LocalStatus = "available"
	LocalStatusUnavailable LocalStatus = "unavailable"
)

type QuotaStatus string

const (
	QuotaStatusAvailable   QuotaStatus = "available"
	QuotaStatusRateLimited QuotaStatus = "rate_limited"
	QuotaStatusUnlimited   QuotaStatus = "unlimited"
)

type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type SearchOptions struct {
	Query           string
	Type            string
	Limit           int
	Near            *Point
	UserID          string
	IncludeExternal bool
	PrivateInput    bool
	ConsentToken    string
}

type Result struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Subtype    string   `json:"subtype,omitempty"`
	Lat        float64  `json:"lat"`
	Lng        float64  `json:"lng"`
	Similarity float64  `json:"similarity,omitempty"`
	DistanceM  *float64 `json:"distance_m,omitempty"`
	Source     Source   `json:"source"`
}

type SearchResult struct {
	Results        []Result        `json:"results"`
	Count          int             `json:"count"`
	LocalStatus    LocalStatus     `json:"local_status"`
	ExternalStatus ExternalStatus  `json:"external_status"`
	Quota          *QuotaState     `json:"rate_limit,omitempty"`
	ConsentRequest *ConsentRequest `json:"consent_request,omitempty"`
}

type QuotaState struct {
	Status         QuotaStatus `json:"status"`
	LimitPerHour   int         `json:"limit_per_hour,omitempty"`
	Remaining      int         `json:"remaining,omitempty"`
	ResetAt        time.Time   `json:"reset_at,omitempty"`
	BlockingReason string      `json:"blocking_reason,omitempty"`
	Used           bool        `json:"-"`
}

type ConsentRequest struct {
	Reason string `json:"reason"`
}

type LocalSearcher interface {
	SearchLocal(context.Context, SearchOptions) ([]Result, error)
}

type ForwardSearcher interface {
	ForwardSearch(context.Context, SearchOptions) ([]Result, error)
}

type ServiceConfig struct {
	Local    LocalSearcher
	External ForwardSearcher
	Policy   ExternalPolicy
	Cache    *Cache
	Quota    *Limiter
}

type Service struct {
	local    LocalSearcher
	external ForwardSearcher
	policy   ExternalPolicy
	cache    *Cache
	quota    *Limiter
}

func NewService(cfg ServiceConfig) *Service {
	policy := cfg.Policy
	if policy == "" {
		policy = ExternalPolicyDisabled
	}
	return &Service{
		local:    cfg.Local,
		external: cfg.External,
		policy:   policy,
		cache:    cfg.Cache,
		quota:    cfg.Quota,
	}
}

func (s *Service) Search(ctx context.Context, opts SearchOptions) (SearchResult, error) {
	opts = normalizeOptions(opts)
	out := SearchResult{
		LocalStatus:    LocalStatusAvailable,
		ExternalStatus: ExternalStatusSkipped,
	}

	if s.local != nil && opts.Type != "external" {
		results, err := s.local.SearchLocal(ctx, opts)
		if err != nil {
			out.LocalStatus = LocalStatusUnavailable
		} else {
			out.Results = append(out.Results, results...)
		}
	} else if opts.Type != "external" {
		out.LocalStatus = LocalStatusUnavailable
	}

	if opts.IncludeExternal && (opts.Type == "all" || opts.Type == "external" || opts.Type == "place") {
		external, status, quota, consent := s.searchExternal(ctx, opts)
		out.ExternalStatus = status
		out.Quota = quota
		out.ConsentRequest = consent
		out.Results = append(out.Results, external...)
	}

	if out.Results == nil {
		out.Results = []Result{}
	}
	out.Count = len(out.Results)
	return out, nil
}

func (s *Service) searchExternal(ctx context.Context, opts SearchOptions) ([]Result, ExternalStatus, *QuotaState, *ConsentRequest) {
	switch s.policy {
	case ExternalPolicyDisabled:
		if opts.PrivateInput {
			return nil, ExternalStatusSkippedByPrivacyPolicy, nil, nil
		}
		return nil, ExternalStatusDisabled, nil, nil
	case ExternalPolicyManualConfirm:
		return nil, ExternalStatusRequiresUserConfirmation, quotaPtr(s.quota.Check(opts.UserID)), &ConsentRequest{Reason: "external_lookup"}
	case ExternalPolicyAllowed:
	default:
		return nil, ExternalStatusDisabled, nil, nil
	}

	if s.external == nil {
		return nil, ExternalStatusUnconfigured, quotaPtr(s.quota.Check(opts.UserID)), nil
	}

	key := externalCacheKey(opts)
	if cached, ok := s.cache.get(key); ok {
		quota := s.quota.Check(opts.UserID)
		return cached, ExternalStatusCached, quotaPtr(quota), nil
	}

	quota := s.quota.Allow(opts.UserID)
	if quota.Status == QuotaStatusRateLimited {
		return nil, ExternalStatusRateLimited, &quota, nil
	}

	results, err := s.external.ForwardSearch(ctx, opts)
	if err != nil {
		return nil, ExternalStatusUnreachable, &quota, nil
	}
	for i := range results {
		if results[i].Source == "" {
			results[i].Source = SourceExternal
		}
	}
	s.cache.set(key, results)
	return results, ExternalStatusAvailable, &quota, nil
}

func quotaPtr(state QuotaState) *QuotaState {
	return &state
}

func normalizeOptions(opts SearchOptions) SearchOptions {
	opts.Query = strings.TrimSpace(opts.Query)
	if opts.Type == "" {
		opts.Type = "all"
	}
	switch opts.Type {
	case "all", "street", "place", "external":
	default:
		opts.Type = "all"
	}
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}
	return opts
}

func externalCacheKey(opts SearchOptions) string {
	scope := "global"
	if opts.UserID != "" {
		scope = "user:" + opts.UserID
	}
	near := ""
	if opts.Near != nil {
		near = fmt.Sprintf("%.5f,%.5f", opts.Near.Lat, opts.Near.Lng)
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		scope,
		strings.ToLower(strings.TrimSpace(opts.Query)),
		opts.Type,
		near,
		fmt.Sprintf("%d", opts.Limit),
	}, "|")))
	return hex.EncodeToString(sum[:])
}

type Queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type LocalDBSearcher struct {
	DB Queryer
}

func NewLocalDBSearcher(db Queryer) *LocalDBSearcher {
	return &LocalDBSearcher{DB: db}
}

func (s *LocalDBSearcher) SearchLocal(ctx context.Context, opts SearchOptions) ([]Result, error) {
	if s == nil || s.DB == nil {
		return []Result{}, nil
	}
	opts = normalizeOptions(opts)
	var results []Result

	if opts.Type == "all" || opts.Type == "street" {
		query, args, err := buildStreetSearchQuery(opts)
		if err != nil {
			return nil, err
		}
		rows, err := s.DB.Query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var r Result
			r.Type = "street"
			r.Source = SourceLocal
			if err := rows.Scan(&r.Name, &r.Subtype, &r.Lat, &r.Lng, &r.Similarity, &r.DistanceM); err != nil {
				rows.Close()
				return nil, err
			}
			results = append(results, r)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	if opts.Type == "all" || opts.Type == "place" {
		query, args, err := buildPlaceSearchQuery(opts)
		if err != nil {
			return nil, err
		}
		rows, err := s.DB.Query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var r Result
			var category string
			r.Type = "place"
			r.Source = SourceLocal
			if err := rows.Scan(&r.Name, &category, &r.Subtype, &r.Lat, &r.Lng, &r.Similarity); err != nil {
				rows.Close()
				return nil, err
			}
			r.Type = category
			results = append(results, r)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	if results == nil {
		results = []Result{}
	}
	return results, nil
}

func buildStreetSearchQuery(opts SearchOptions) (string, []any, error) {
	opts = normalizeOptions(opts)
	if opts.Near != nil {
		return `
				SELECT name, highway,
				       ST_Y(ST_Centroid(geom)) AS lat, ST_X(ST_Centroid(geom)) AS lng,
				       similarity(name, $1) AS sim,
				       ST_Distance(geom::geography, ST_Point($2,$3)::geography) AS dist_m
				FROM osm_ways
				WHERE name % $1 AND name IS NOT NULL
				ORDER BY geom::geography <-> ST_Point($2,$3)::geography
				LIMIT $4`, []any{opts.Query, opts.Near.Lng, opts.Near.Lat, opts.Limit}, nil
	}
	return `
				SELECT name, highway,
				       ST_Y(ST_Centroid(geom)) AS lat, ST_X(ST_Centroid(geom)) AS lng,
				       similarity(name, $1) AS sim,
				       NULL::float8 AS dist_m
				FROM osm_ways
				WHERE name % $1 AND name IS NOT NULL
				ORDER BY similarity(name, $1) DESC
				LIMIT $2`, []any{opts.Query, opts.Limit}, nil
}

func buildPlaceSearchQuery(opts SearchOptions) (string, []any, error) {
	opts = normalizeOptions(opts)
	return `
			SELECT name, category, subtype,
			       ST_Y(ST_Centroid(geom)) AS lat, ST_X(ST_Centroid(geom)) AS lng,
			       similarity(name, $1) AS sim
			FROM osm_pois
			WHERE name % $1 AND name IS NOT NULL
			  AND category IN ('settlement', 'summit', 'pass')
			ORDER BY similarity(name, $1) DESC
			LIMIT $2`, []any{opts.Query, opts.Limit}, nil
}
