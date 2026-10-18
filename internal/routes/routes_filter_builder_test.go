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
	cache.Initialize()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/filter-builder", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	baseConfig := config.New()
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
	e := echo.New()
	form := url.Values{}
	form.Set("filter", "")
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
	assert.Contains(t, body, "Please enter a valid JSON filter")
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
