package routes

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/damongolding/immich-kiosk/internal/cache"
	"github.com/damongolding/immich-kiosk/internal/common"
	"github.com/damongolding/immich-kiosk/internal/config"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
)

func TestFilterBuilderPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	cache.Initialize()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/filter-builder", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	baseConfig := config.New()
	baseConfig.ImmichURL = server.URL
	baseConfig.ImmichAPIKey = "test-key"
	baseConfig.Kiosk.EnableURLBuilder = true
	baseConfig.ImmichUsersAPIKeys = map[string]string{
		"rafael": "key1",
		"maria":  "key2",
	}

	com := common.New()
	handler := FilterBuilderPage(baseConfig, com)
	err := handler(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Filter Builder")
	assert.Contains(t, body, "rafael")
	assert.Contains(t, body, "maria")
}

func TestFilterBuilderSearch_EmptyFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"assets":{"items":[],"total":0,"count":0}}`))
	}))
	defer server.Close()

	e := echo.New()
	form := url.Values{}
	form.Set("filter", "")
	req := httptest.NewRequest(http.MethodPost, "/filter-builder/search", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	baseConfig := config.New()
	baseConfig.ImmichURL = server.URL
	baseConfig.ImmichAPIKey = "test-key"
	com := common.New()

	handler := FilterBuilderSearch(baseConfig, com)
	err := handler(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, "Filter Error")
	assert.Contains(t, body, "filter-builder-results")
}

func TestFilterBuilderSearch_InvalidJSON(t *testing.T) {
	e := echo.New()
	form := url.Values{}
	form.Set("filter", "{invalid-json: true}")
	req := httptest.NewRequest(http.MethodPost, "/filter-builder/search", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	baseConfig := config.New()
	com := common.New()

	handler := FilterBuilderSearch(baseConfig, com)
	err := handler(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Invalid Filter JSON")
}
