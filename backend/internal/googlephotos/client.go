package googlephotos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/palpal/gphoto/internal/domain"
)

const APIBaseURL = "https://photospicker.googleapis.com/v1"

type Client struct {
	httpClient *http.Client
	baseURL    string
}

type sessionResponse struct {
	ID            string `json:"id"`
	PickerURI     string `json:"pickerUri"`
	ExpireTime    string `json:"expireTime"`
	MediaItemsSet bool   `json:"mediaItemsSet"`
	PollingConfig struct {
		PollInterval string `json:"pollInterval"`
		TimeoutIn    string `json:"timeoutIn"`
	} `json:"pollingConfig"`
}

func NewClient(httpClient *http.Client, baseURL string) *Client {
	if baseURL == "" {
		baseURL = APIBaseURL
	}
	return &Client{httpClient: httpClient, baseURL: strings.TrimRight(baseURL, "/")}
}

func (client *Client) CreateSession(ctx context.Context) (domain.ProviderSession, error) {
	var response sessionResponse
	if err := client.requestJSON(ctx, http.MethodPost, client.baseURL+"/sessions", bytes.NewReader([]byte(`{}`)), &response); err != nil {
		return domain.ProviderSession{}, err
	}
	return domain.ProviderSession{ID: response.ID, PickerURI: response.PickerURI, MediaItemsSet: response.MediaItemsSet}, nil
}

func (client *Client) GetSession(ctx context.Context, sessionID string) (domain.ProviderSession, error) {
	var response sessionResponse
	if err := client.requestJSON(ctx, http.MethodGet, client.baseURL+"/sessions/"+url.PathEscape(sessionID), nil, &response); err != nil {
		return domain.ProviderSession{}, err
	}
	return domain.ProviderSession{ID: response.ID, PickerURI: response.PickerURI, MediaItemsSet: response.MediaItemsSet}, nil
}

func (client *Client) ListMedia(ctx context.Context, sessionID string) ([]domain.GoogleMediaItem, error) {
	items := make([]domain.GoogleMediaItem, 0)
	pageToken := ""
	for {
		values := url.Values{"sessionId": []string{sessionID}, "pageSize": []string{"100"}}
		if pageToken != "" {
			values.Set("pageToken", pageToken)
		}
		var response struct {
			MediaItems []struct {
				ID         string    `json:"id"`
				CreateTime time.Time `json:"createTime"`
				MediaFile  struct {
					BaseURL  string `json:"baseUrl"`
					MimeType string `json:"mimeType"`
					Filename string `json:"filename"`
				} `json:"mediaFile"`
			} `json:"mediaItems"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := client.requestJSON(ctx, http.MethodGet, client.baseURL+"/mediaItems?"+values.Encode(), nil, &response); err != nil {
			return nil, err
		}
		for _, item := range response.MediaItems {
			items = append(items, domain.GoogleMediaItem{
				ProviderID: item.ID,
				Filename:   item.MediaFile.Filename,
				MimeType:   item.MediaFile.MimeType,
				BaseURL:    item.MediaFile.BaseURL,
				CreatedAt:  item.CreateTime,
			})
		}
		pageToken = response.NextPageToken
		if pageToken == "" {
			return items, nil
		}
	}
}

func (client *Client) Download(ctx context.Context, item domain.GoogleMediaItem) (io.ReadCloser, error) {
	suffix := "=d"
	if strings.HasPrefix(item.MimeType, "video/") {
		suffix = "=dv"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, item.BaseURL+suffix, nil)
	if err != nil {
		return nil, fmt.Errorf("create media download request: %w", err)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download media: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("download media: Google returned %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	return response.Body, nil
}

func (client *Client) requestJSON(ctx context.Context, method, endpoint string, body io.Reader, target any) error {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("create Google Photos request: %w", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call Google Photos: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Google Photos returned %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode Google Photos response: %w", err)
	}
	return nil
}
