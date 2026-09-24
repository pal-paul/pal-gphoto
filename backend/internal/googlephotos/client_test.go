package googlephotos

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/palpal/gphoto/internal/domain"
)

func TestListMediaFollowsPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("pageToken") == "next" {
			io.WriteString(writer, `{"mediaItems":[{"id":"video","createTime":"2026-09-02T10:00:00Z","mediaFile":{"baseUrl":"https://media/video","mimeType":"video/mp4","filename":"video.mp4"}}]}`)
			return
		}
		io.WriteString(writer, `{"mediaItems":[{"id":"photo","createTime":"2026-09-01T10:00:00Z","mediaFile":{"baseUrl":"https://media/photo","mimeType":"image/jpeg","filename":"photo.jpg"}}],"nextPageToken":"next"}`)
	}))
	defer server.Close()

	items, err := NewClient(server.Client(), server.URL).ListMedia(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ProviderID != "photo" || items[1].ProviderID != "video" {
		t.Fatalf("ListMedia() = %+v", items)
	}
}

func TestDownloadUsesMediaTypeSuffix(t *testing.T) {
	requestedPath := ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestedPath = request.URL.RequestURI()
		io.WriteString(writer, "video bytes")
	}))
	defer server.Close()

	reader, err := NewClient(server.Client(), server.URL).Download(context.Background(), domain.GoogleMediaItem{BaseURL: server.URL + "/media", MimeType: "video/mp4"})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if requestedPath != "/media=dv" {
		t.Fatalf("download path = %q, want /media=dv", requestedPath)
	}
}
