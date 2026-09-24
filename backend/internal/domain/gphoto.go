package domain

import "time"

type MediaType string

const (
	MediaTypeAll    MediaType = "all"
	MediaTypeImages MediaType = "images"
	MediaTypeVideos MediaType = "videos"
)

type GoogleAccount struct {
	ID              string    `json:"id"`
	GoogleSubject   string    `json:"googleSubject"`
	Email           string    `json:"email"`
	Name            string    `json:"name"`
	MediaType       MediaType `json:"mediaType"`
	TokenCiphertext []byte    `json:"-"`
	CreatedAt       time.Time `json:"createdAt"`
}

type PickerSession struct {
	ID        string    `json:"id"`
	AccountID string    `json:"accountId"`
	PickerURI string    `json:"pickerUri"`
	Status    string    `json:"status"`
	LastError string    `json:"lastError,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ProviderSession struct {
	ID            string
	PickerURI     string
	MediaItemsSet bool
}

type GoogleMediaItem struct {
	ProviderID string
	Filename   string
	MimeType   string
	BaseURL    string
	CreatedAt  time.Time
}
