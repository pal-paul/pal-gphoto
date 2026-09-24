package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/palpal/gphoto/internal/domain"
	"github.com/palpal/gphoto/internal/store"
)

type GoogleAuthorization interface {
	Begin(context.Context, string) (string, error)
	Complete(context.Context, string, string) (domain.GoogleAccount, error)
}

type GoogleSync interface {
	StartSession(context.Context, string) (domain.PickerSession, error)
}

type GoogleState interface {
	ListGoogleAccounts(context.Context) ([]domain.GoogleAccount, error)
	UpdateGoogleAccountMediaType(context.Context, string, domain.MediaType) error
	ListPickerSessions(context.Context, bool) ([]domain.PickerSession, error)
}

type GPhotoHandler struct {
	authorization GoogleAuthorization
	syncer        GoogleSync
	state         GoogleState
}

func NewGPhotoHandler(authorization GoogleAuthorization, syncer GoogleSync, state GoogleState) *GPhotoHandler {
	return &GPhotoHandler{authorization: authorization, syncer: syncer, state: state}
}

func (handler *GPhotoHandler) Router() http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	router.Get("/health", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	})
	router.Route("/api/v1", func(api chi.Router) {
		api.Post("/google/auth", handler.beginAuthorization)
		api.Get("/google/callback", handler.completeAuthorization)
		api.Get("/accounts", handler.listAccounts)
		api.Patch("/accounts/{accountID}", handler.updateAccount)
		api.Post("/accounts/{accountID}/picker-sessions", handler.startPickerSession)
		api.Get("/picker-sessions", handler.listPickerSessions)
	})
	return router
}

func (handler *GPhotoHandler) beginAuthorization(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		writeError(writer, http.StatusBadRequest, errors.New("account name is required"))
		return
	}
	authorizationURL, err := handler.authorization.Begin(request.Context(), input.Name)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]string{"authorizationUrl": authorizationURL})
}

func (handler *GPhotoHandler) completeAuthorization(writer http.ResponseWriter, request *http.Request) {
	state, code := request.URL.Query().Get("state"), request.URL.Query().Get("code")
	if state == "" || code == "" {
		writeError(writer, http.StatusBadRequest, errors.New("state and code are required"))
		return
	}
	account, err := handler.authorization.Complete(request.Context(), state, code)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	writeJSON(writer, http.StatusCreated, account)
}

func (handler *GPhotoHandler) listAccounts(writer http.ResponseWriter, request *http.Request) {
	accounts, err := handler.state.ListGoogleAccounts(request.Context())
	respond(writer, accounts, err)
}

func (handler *GPhotoHandler) updateAccount(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		MediaType domain.MediaType `json:"mediaType"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	if input.MediaType != domain.MediaTypeAll && input.MediaType != domain.MediaTypeImages && input.MediaType != domain.MediaTypeVideos {
		writeError(writer, http.StatusBadRequest, errors.New("mediaType must be all, images, or videos"))
		return
	}
	if err := handler.state.UpdateGoogleAccountMediaType(request.Context(), chi.URLParam(request, "accountID"), input.MediaType); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(writer, http.StatusNotFound, err)
			return
		}
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *GPhotoHandler) startPickerSession(writer http.ResponseWriter, request *http.Request) {
	session, err := handler.syncer.StartSession(request.Context(), chi.URLParam(request, "accountID"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(writer, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(writer, http.StatusBadGateway, err)
		return
	}
	writeJSON(writer, http.StatusCreated, session)
}

func (handler *GPhotoHandler) listPickerSessions(writer http.ResponseWriter, request *http.Request) {
	sessions, err := handler.state.ListPickerSessions(request.Context(), false)
	respond(writer, sessions, err)
}
