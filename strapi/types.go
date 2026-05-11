package strapi

import "time"

type ListResponse struct {
	Data []Entry `json:"data"`
}

type Entry struct {
	ID         int64      `json:"id"`
	Attributes Attributes `json:"attributes"`

	// Some Strapi setups return a flattened object (without attributes wrapper).
	Key                string       `json:"key"`
	Locale             string       `json:"locale"`
	Local              string       `json:"local"`
	TextValue          string       `json:"text_value"`
	IconAsset          IconAssetRef `json:"icon_asset"`
	IconName           string       `json:"icon_name"`
	IconTintHex        string       `json:"icon_tint_hex"`
	PlatformVisibility string       `json:"platform_visibility"`
	MinAppVersion      string       `json:"min_app_version"`
	UpdatedAt          time.Time    `json:"updatedAt"`
	PublishedAt        time.Time    `json:"publishedAt"`
}

type Attributes struct {
	Key                string       `json:"key"`
	Locale             string       `json:"locale"`
	Local              string       `json:"local"`
	TextValue          string       `json:"text_value"`
	IconAsset          IconAssetRef `json:"icon_asset"`
	IconName           string       `json:"icon_name"`
	IconTintHex        string       `json:"icon_tint_hex"`
	PlatformVisibility string       `json:"platform_visibility"`
	MinAppVersion      string       `json:"min_app_version"`
	UpdatedAt          time.Time    `json:"updatedAt"`
	PublishedAt        time.Time    `json:"publishedAt"`
}

// IconAssetRef handles both flat (Strapi v4 default) and nested (legacy) response formats
type IconAssetRef struct {
	// Flat format (Strapi v4 direct response) - has full asset data
	ID         int64   `json:"id"`
	DocumentID string  `json:"documentId"`
	URL        string  `json:"url"`
	Name       string  `json:"name"`
	Size       float64 `json:"size"`
	Mime       string  `json:"mime"`
	Hash       string  `json:"hash"`
	Ext        string  `json:"ext"`

	// Nested format (wrapped in data)
	Data *MediaData `json:"data"`
}

type MediaData struct {
	ID         int64           `json:"id"`
	Attributes MediaAttributes `json:"attributes"`
}

type MediaAttributes struct {
	URL string `json:"url"`
}
