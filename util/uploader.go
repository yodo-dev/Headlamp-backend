package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// Uploader handles file uploads to an external service.
type Uploader struct {
	client   *http.Client
	baseURL  string
	apiToken string
}

// NewUploader creates a new uploader instance.
func NewUploader(baseURL, apiToken string) *Uploader {
	return &Uploader{
		client:   &http.Client{},
		baseURL:  baseURL,
		apiToken: apiToken,
	}
}

// UploadResponse defines the structure of a successful upload response.
// This is based on the Strapi v4 API, but is generic enough for other services.
type UploadResponse struct {
	ID  int    `json:"id"`
	URL string `json:"url"`
}

// UploadFile sends a file to the external content provider.
func (u *Uploader) UploadFile(fileName string, fileContent io.Reader, uploadPath string) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("files", fileName)
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}

	_, err = io.Copy(part, fileContent)
	if err != nil {
		return "", fmt.Errorf("failed to copy file content: %w", err)
	}

	// Add the path field to specify the upload folder
	_ = writer.WriteField("path", uploadPath)

	err = writer.Close()
	if err != nil {
		return "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Assuming a standard /api/upload endpoint
	fmt.Println(u.baseURL)
	url := fmt.Sprintf("%s/api/upload", u.baseURL)
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", u.apiToken))
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := u.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request to external content provider: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("external content provider returned non-200/201 status: %d - %s", resp.StatusCode, string(respBody))
	}

	var uploadResp []UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(uploadResp) == 0 {
		return "", fmt.Errorf("no files returned in response")
	}

	// The response from the external provider is a relative path, so we prepend the base URL.
	fullURL := fmt.Sprintf("%s%s", u.baseURL, uploadResp[0].URL)
	return fullURL, nil
}
