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
	req := httptest.NewRequest(http.MethodGet, "/url-builder/filters", nil)
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
	assert.Contains(t, body, "Filter Playground")
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
	req := httptest.NewRequest(http.MethodPost, "/url-builder/filters/search", strings.NewReader(form.Encode()))
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
	req := httptest.NewRequest(http.MethodPost, "/url-builder/filters/search", strings.NewReader(form.Encode()))
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

func TestFilterBuilderPage_MultiUserAggregation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		key := r.Header.Get("x-api-key")

		switch {
		case strings.Contains(r.URL.Path, "people"):
			if key == "key1" {
				w.Write([]byte(`{"people":[{"id":"p1","name":"RafaelPerson"}],"total":1,"hasNextPage":false}`))
			} else if key == "key2" {
				w.Write([]byte(`{"people":[{"id":"p2","name":"MariaPerson"}],"total":1,"hasNextPage":false}`))
			} else {
				w.Write([]byte(`{"people":[],"total":0,"hasNextPage":false}`))
			}
		case strings.Contains(r.URL.Path, "albums"):
			if key == "key1" {
				w.Write([]byte(`[{"id":"a1","albumName":"RafaelAlbum"}]`))
			} else if key == "key2" {
				w.Write([]byte(`[{"id":"a2","albumName":"MariaAlbum"}]`))
			} else {
				w.Write([]byte(`[]`))
			}
		case strings.Contains(r.URL.Path, "cities"):
			if key == "key1" {
				w.Write([]byte(`[{"exifInfo":{"country":"Germany","city":"Berlin"}}]`))
			} else if key == "key2" {
				w.Write([]byte(`[{"exifInfo":{"country":"Brazil","city":"Florianopolis"}}]`))
			} else {
				w.Write([]byte(`[]`))
			}
		default:
			w.Write([]byte(`[]`))
		}
	}))
	defer server.Close()

	cache.Initialize()
	e := echo.New()

	baseConfig := config.New()
	baseConfig.ImmichURL = server.URL
	baseConfig.ImmichAPIKey = "key1"
	baseConfig.Kiosk.EnableURLBuilder = true
	baseConfig.ImmichUsersAPIKeys = map[string]string{
		"rafael": "key1",
		"maria":  "key2",
	}

	com := common.New()
	handler := FilterBuilderPage(baseConfig, com)

	// 1. All Users (empty or no user query param)
	reqAll := httptest.NewRequest(http.MethodGet, "/url-builder/filters", nil)
	recAll := httptest.NewRecorder()
	cAll := e.NewContext(reqAll, recAll)
	errAll := handler(cAll)

	assert.NoError(t, errAll)
	assert.Equal(t, http.StatusOK, recAll.Code)
	bodyAll := recAll.Body.String()

	assert.Contains(t, bodyAll, "RafaelPerson (@rafael)")
	assert.Contains(t, bodyAll, "MariaPerson (@maria)")
	assert.Contains(t, bodyAll, "RafaelAlbum (@rafael)")
	assert.Contains(t, bodyAll, "MariaAlbum (@maria)")
	assert.Contains(t, bodyAll, "Germany")
	assert.Contains(t, bodyAll, "Brazil")
	assert.Contains(t, bodyAll, "Berlin")
	assert.Contains(t, bodyAll, "Florianopolis")

	// 2. Specific User (?user=rafael)
	reqUser := httptest.NewRequest(http.MethodGet, "/url-builder/filters?user=rafael", nil)
	recUser := httptest.NewRecorder()
	cUser := e.NewContext(reqUser, recUser)
	errUser := handler(cUser)

	assert.NoError(t, errUser)
	assert.Equal(t, http.StatusOK, recUser.Code)
	bodyUser := recUser.Body.String()

	assert.Contains(t, bodyUser, "RafaelPerson")
	assert.Contains(t, bodyUser, "RafaelAlbum")
	assert.Contains(t, bodyUser, "Germany")
	assert.Contains(t, bodyUser, "Berlin")

	assert.NotContains(t, bodyUser, "MariaPerson")
	assert.NotContains(t, bodyUser, "MariaAlbum")
	assert.NotContains(t, bodyUser, "Brazil")
	assert.NotContains(t, bodyUser, "Florianopolis")
}

func TestFilterBuilderSearch_MultiSelectBalanced(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			{"id":"asset1","type":"IMAGE","exifInfo":{"country":"Australia","city":"Melbourne"}},
			{"id":"asset2","type":"IMAGE","exifInfo":{"country":"Albania","city":"Tirana"}},
			{"id":"asset3","type":"IMAGE","exifInfo":{"country":"Australia","city":"Sydney"}},
			{"id":"asset4","type":"IMAGE","exifInfo":{"country":"Albania","city":"Durres"}}
		]`))
	}))
	defer server.Close()

	e := echo.New()
	form := url.Values{}
	form.Add("country", "Albania")
	form.Add("country", "Australia")
	req := httptest.NewRequest(http.MethodPost, "/url-builder/filters/search", strings.NewReader(form.Encode()))
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

	assert.Contains(t, body, "Albania")
	assert.Contains(t, body, "Australia")
	assert.Contains(t, body, "Melbourne")
	assert.Contains(t, body, "Tirana")
	assert.NotContains(t, body, "Filter Error")
}



