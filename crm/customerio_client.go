package crm

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type CustomerIOClient interface {
	Enabled() bool
	IdentifyUser(ctx context.Context, personID string, attributes map[string]any) error
	TrackEvent(ctx context.Context, personID, eventName string, properties map[string]any, eventTime time.Time) error
	ValidateWebhookSignature(signature string, payload []byte) bool
}

type noopCustomerIOClient struct{}

func (noopCustomerIOClient) Enabled() bool                                              { return false }
func (noopCustomerIOClient) IdentifyUser(context.Context, string, map[string]any) error { return nil }
func (noopCustomerIOClient) TrackEvent(context.Context, string, string, map[string]any, time.Time) error {
	return nil
}
func (noopCustomerIOClient) ValidateWebhookSignature(string, []byte) bool { return false }

type customerIOClient struct {
	siteID        string
	apiKey        string
	trackAPIURL   string
	webhookSecret string
	httpClient    *http.Client
}

func NewCustomerIOClient(siteID, apiKey, trackAPIURL, webhookSecret string, timeout time.Duration) CustomerIOClient {
	if strings.TrimSpace(siteID) == "" || strings.TrimSpace(apiKey) == "" || strings.TrimSpace(trackAPIURL) == "" {
		return noopCustomerIOClient{}
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &customerIOClient{
		siteID:        strings.TrimSpace(siteID),
		apiKey:        strings.TrimSpace(apiKey),
		trackAPIURL:   strings.TrimRight(strings.TrimSpace(trackAPIURL), "/"),
		webhookSecret: strings.TrimSpace(webhookSecret),
		httpClient:    &http.Client{Timeout: timeout},
	}
}

func (c *customerIOClient) Enabled() bool {
	return c.siteID != "" && c.apiKey != "" && c.trackAPIURL != ""
}

func (c *customerIOClient) IdentifyUser(ctx context.Context, personID string, attributes map[string]any) error {
	endpoint := fmt.Sprintf("%s/customers/%s", c.trackAPIURL, url.PathEscape(personID))
	return c.doJSON(ctx, http.MethodPut, endpoint, attributes)
}

func (c *customerIOClient) TrackEvent(ctx context.Context, personID, eventName string, properties map[string]any, eventTime time.Time) error {
	body := map[string]any{
		"name": eventName,
		"data": properties,
	}
	if !eventTime.IsZero() {
		body["timestamp"] = eventTime.UTC().Unix()
	}
	endpoint := fmt.Sprintf("%s/customers/%s/events", c.trackAPIURL, url.PathEscape(personID))
	return c.doJSON(ctx, http.MethodPost, endpoint, body)
}

func (c *customerIOClient) ValidateWebhookSignature(signature string, payload []byte) bool {
	if c.webhookSecret == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(c.webhookSecret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(signature)), []byte(strings.ToLower(expected))) == 1
}

func (c *customerIOClient) doJSON(ctx context.Context, method, endpoint string, body any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal customer.io payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build customer.io request: %w", err)
	}
	req.SetBasicAuth(c.siteID, c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("customer.io request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("customer.io request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	return nil
}
