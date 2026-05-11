package strapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client interface {
	FetchMobileUIConfig(ctx context.Context, locale string) ([]Attributes, time.Time, error)
}

type client struct {
	baseURL    string
	apiToken   string
	cdnPrefix  string
	httpClient *http.Client
}

func NewClient(baseURL, apiToken, cdnPrefix string, timeout time.Duration) Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		apiToken:  strings.TrimSpace(apiToken),
		cdnPrefix: strings.TrimRight(strings.TrimSpace(cdnPrefix), "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *client) FetchMobileUIConfig(ctx context.Context, locale string) ([]Attributes, time.Time, error) {
	if c.baseURL == "" {
		return nil, time.Time{}, fmt.Errorf("strapi base url is not configured")
	}

	reqURL, err := c.buildMobileConfigURL(locale)
	if err != nil {
		return nil, time.Time{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, time.Time{}, err
	}
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, time.Time{}, fmt.Errorf("strapi request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload ListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, time.Time{}, fmt.Errorf("decode strapi response: %w", err)
	}

	entries := make([]Attributes, 0, len(payload.Data))
	var version time.Time
	for _, item := range payload.Data {
		attr := entryToAttributes(item)
		if !matchesRequestedLocale(locale, attr) {
			continue
		}
		if attr.PublishedAt.IsZero() {
			continue
		}

		if attr.PublishedAt.After(version) {
			version = attr.PublishedAt
		}

		if attr.IconAsset.Data != nil {
			attr.IconAsset.Data.Attributes.URL = c.resolveMediaURL(attr.IconAsset.Data.Attributes.URL)
		}
		// Also handle flat icon_asset format from Strapi v4
		if attr.IconAsset.URL != "" {
			attr.IconAsset.URL = c.resolveMediaURL(attr.IconAsset.URL)
		}

		entries = append(entries, attr)
	}

	if version.IsZero() {
		version = time.Now().UTC()
	}

	return entries, version.UTC(), nil
}

func entryToAttributes(item Entry) Attributes {
	attr := item.Attributes
	// For nested/legacy response format, attributes wrapper has the data
	if attr.Key != "" && !attr.PublishedAt.IsZero() {
		return attr
	}

	// For flat response format, use top-level fields
	return Attributes{
		Key:                item.Key,
		Locale:             item.Locale,
		Local:              item.Local,
		TextValue:          item.TextValue,
		IconAsset:          item.IconAsset,
		IconName:           item.IconName,
		IconTintHex:        item.IconTintHex,
		PlatformVisibility: item.PlatformVisibility,
		MinAppVersion:      item.MinAppVersion,
		UpdatedAt:          item.UpdatedAt,
		PublishedAt:        item.PublishedAt,
	}
}

func (c *client) buildMobileConfigURL(locale string) (string, error) {
	u, err := url.Parse(c.baseURL + "/api/mobile-ui-configs")
	if err != nil {
		return "", fmt.Errorf("invalid strapi base url: %w", err)
	}

	q := u.Query()
	q.Set("publicationState", "live")
	q.Set("populate", "icon_asset")
	q.Set("pagination[pageSize]", "200")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func matchesRequestedLocale(requestedLocale string, attr Attributes) bool {
	requested := strings.TrimSpace(requestedLocale)
	if requested == "" {
		return true
	}

	entryLocale := strings.TrimSpace(attr.Locale)
	if entryLocale == "" {
		entryLocale = strings.TrimSpace(attr.Local)
	}
	if entryLocale == "" {
		return true
	}

	return strings.EqualFold(entryLocale, requested)
}

func (c *client) resolveMediaURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	if c.cdnPrefix == "" {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "/") {
		return c.cdnPrefix + trimmed
	}
	return c.cdnPrefix + "/" + trimmed
}
