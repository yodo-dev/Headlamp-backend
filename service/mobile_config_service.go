package service

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/strapi"
	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

const defaultMobileConfigTTLSeconds = 21600

//go:embed fallback_config.json
var fallbackConfigJSON []byte

type MobileConfigItem struct {
	Key              string `json:"key"`
	Type             string `json:"type"`
	Value            string `json:"value,omitempty"`
	URL              string `json:"url,omitempty"`
	FallbackIconName string `json:"fallback_icon_name,omitempty"`
	IconTintHex      string `json:"icon_tint_hex,omitempty"`
}

type MobileConfigResponse struct {
	ConfigVersion string             `json:"config_version"`
	TTLSeconds    int                `json:"ttl_seconds"`
	Items         []MobileConfigItem `json:"items"`
}

type MobileConfigService struct {
	store      db.Store
	strapi     strapi.Client
	cache      *util.TTLCache
	ttlSeconds int
	fallback   MobileConfigResponse
}

func NewMobileConfigService(store db.Store, strapiClient strapi.Client, ttlSeconds int) (*MobileConfigService, error) {
	if ttlSeconds <= 0 {
		ttlSeconds = defaultMobileConfigTTLSeconds
	}

	fallback := MobileConfigResponse{}
	if err := json.Unmarshal(fallbackConfigJSON, &fallback); err != nil {
		return nil, fmt.Errorf("parse mobile fallback config: %w", err)
	}
	if fallback.TTLSeconds <= 0 {
		fallback.TTLSeconds = ttlSeconds
	}

	return &MobileConfigService{
		store:      store,
		strapi:     strapiClient,
		cache:      util.NewTTLCache(),
		ttlSeconds: ttlSeconds,
		fallback:   fallback,
	}, nil
}

func (s *MobileConfigService) GetConfig(ctx context.Context, locale, platform, appVersion string) (MobileConfigResponse, error) {
	cacheKey := makeCacheKey(locale, platform, appVersion)
	cacheTTL := time.Duration(s.ttlSeconds) * time.Second

	if cached, ok := s.cache.Get(cacheKey); ok {
		var resp MobileConfigResponse
		if err := json.Unmarshal(cached, &resp); err == nil {
			return resp, nil
		}
	}

	cachedRow, cacheErr := s.store.GetMobileConfigCacheByKey(ctx, cacheKey)
	if cacheErr == nil {
		if !cachedRow.IsStale && time.Since(cachedRow.FetchedAt) <= cacheTTL {
			var resp MobileConfigResponse
			if err := json.Unmarshal(cachedRow.Payload, &resp); err == nil {
				s.cache.Set(cacheKey, cachedRow.Payload, cacheTTL)
				return resp, nil
			}
		}
	}

	if s.strapi != nil {
		entries, version, err := s.strapi.FetchMobileUIConfig(ctx, locale)
		if err == nil {
			items := normalizeMobileConfig(entries, platform, appVersion)
			resp := MobileConfigResponse{
				ConfigVersion: version.UTC().Format(time.RFC3339),
				TTLSeconds:    s.ttlSeconds,
				Items:         items,
			}
			payload, marshalErr := json.Marshal(resp)
			if marshalErr == nil {
				_, upsertErr := s.store.UpsertMobileConfigCache(ctx, db.UpsertMobileConfigCacheParams{
					CacheKey:      cacheKey,
					ConfigVersion: version.UTC(),
					Payload:       payload,
				})
				if upsertErr != nil {
					log.Error().Err(upsertErr).Str("cache_key", cacheKey).Msg("failed to upsert mobile config cache")
				}
				s.cache.Set(cacheKey, payload, cacheTTL)
			} else {
				log.Error().Err(marshalErr).Str("cache_key", cacheKey).Msg("failed to marshal mobile config response")
			}
			return resp, nil
		}

		log.Error().Err(err).Str("cache_key", cacheKey).Str("locale", locale).Msg("failed to fetch mobile config from Strapi")
	}

	if cacheErr == nil {
		var staleResp MobileConfigResponse
		if err := json.Unmarshal(cachedRow.Payload, &staleResp); err == nil {
			s.cache.Set(cacheKey, cachedRow.Payload, cacheTTL)
			return staleResp, nil
		}
	}

	if cacheErr != nil && cacheErr != pgx.ErrNoRows {
		return MobileConfigResponse{}, cacheErr
	}

	return s.fallback, nil
}

func (s *MobileConfigService) InvalidateCache(ctx context.Context) error {
	s.cache.Flush()
	return s.store.MarkAllMobileConfigCacheStale(ctx)
}

func makeCacheKey(locale, platform, appVersion string) string {
	return strings.TrimSpace(locale) + "|" + strings.ToLower(strings.TrimSpace(platform)) + "|" + strings.TrimSpace(appVersion)
}

func normalizeMobileConfig(entries []strapi.Attributes, platform, appVersion string) []MobileConfigItem {
	filtered := make([]strapi.Attributes, 0, len(entries))
	for _, entry := range entries {
		if !isPlatformVisible(entry.PlatformVisibility, platform) {
			continue
		}
		if !isVersionAllowed(entry.MinAppVersion, appVersion) {
			continue
		}
		filtered = append(filtered, entry)
	}

	items := make([]MobileConfigItem, 0, len(filtered))
	for _, entry := range filtered {
		textVal := strings.TrimSpace(entry.TextValue)
		// Extract icon URL (handle both flat and nested formats)
		iconURL := ""
		if entry.IconAsset.Data != nil {
			iconURL = strings.TrimSpace(entry.IconAsset.Data.Attributes.URL)
		}
		if iconURL == "" {
			iconURL = strings.TrimSpace(entry.IconAsset.URL)
		}
		iconName := strings.TrimSpace(entry.IconName)
		iconTint := strings.TrimSpace(entry.IconTintHex)

		if textVal != "" {
			// Create text item (optionally include icon if available)
			items = append(items, MobileConfigItem{
				Key:              entry.Key,
				Type:             "text",
				Value:            textVal,
				URL:              iconURL,
				FallbackIconName: iconName,
				IconTintHex:      iconTint,
			})
			continue
		}

		// Create icon item if no text
		if iconURL == "" && iconName == "" {
			continue
		}

		items = append(items, MobileConfigItem{
			Key:              entry.Key,
			Type:             "icon",
			URL:              iconURL,
			FallbackIconName: iconName,
			IconTintHex:      iconTint,
		})
	}

	return items
}

func isPlatformVisible(visibility, platform string) bool {
	v := strings.ToLower(strings.TrimSpace(visibility))
	p := strings.ToLower(strings.TrimSpace(platform))
	if v == "" || v == "all" {
		return true
	}
	return v == p
}

func isVersionAllowed(minVersion, appVersion string) bool {
	minV := strings.TrimSpace(minVersion)
	appV := strings.TrimSpace(appVersion)
	if minV == "" || appV == "" {
		return true
	}
	return compareSemver(appV, minV) >= 0
}

func compareSemver(a, b string) int {
	av := parseSemverParts(a)
	bv := parseSemverParts(b)
	for i := 0; i < 3; i++ {
		if av[i] < bv[i] {
			return -1
		}
		if av[i] > bv[i] {
			return 1
		}
	}
	return 0
}

func parseSemverParts(version string) [3]int {
	var out [3]int
	clean := strings.TrimSpace(version)
	clean = strings.TrimPrefix(clean, "v")
	if idx := strings.IndexAny(clean, "+-"); idx >= 0 {
		clean = clean[:idx]
	}

	parts := strings.Split(clean, ".")
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(strings.TrimSpace(parts[i]))
		if err != nil || n < 0 {
			out[i] = 0
			continue
		}
		out[i] = n
	}
	return out
}
