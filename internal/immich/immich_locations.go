package immich

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"charm.land/log/v2"
	"github.com/damongolding/immich-kiosk/internal/cache"
)

type LocationMetadata struct {
	Countries []string
	States    []string
	Cities    []string
	Cameras   []string
}

type CityAssetItem struct {
	ExifInfo struct {
		City    string `json:"city"`
		State   string `json:"state"`
		Country string `json:"country"`
		Model   string `json:"model"`
		Make    string `json:"make"`
	} `json:"exifInfo"`
}

// AllLocations fetches assets from the search/cities endpoint and extracts unique
// countries, states, cities, and camera models to populate select dropdowns in the UI.
func (a *Asset) AllLocations(requestID, deviceID string) (LocationMetadata, error) {
	cacheKey := "filter_builder_locations:" + a.requestConfig.ImmichURL
	if cached, ok := cache.Get(cacheKey); ok {
		if locs, ok := cached.(LocationMetadata); ok {
			return locs, nil
		}
	}

	u, err := url.Parse(a.requestConfig.ImmichURL)
	if err != nil {
		return LocationMetadata{}, err
	}

	apiURL := url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   path.Join("api", "search", "cities"),
	}

	body, _, _, err := a.immichAPICall(a.ctx, http.MethodGet, apiURL.String(), nil)
	if err != nil {
		log.Warn(requestID+" Failed to fetch cities explore data", "err", err)
		return LocationMetadata{}, err
	}

	var items []CityAssetItem
	if err := json.Unmarshal(body, &items); err != nil {
		return LocationMetadata{}, err
	}

	countrySet := make(map[string]struct{})
	stateSet := make(map[string]struct{})
	citySet := make(map[string]struct{})
	cameraSet := make(map[string]struct{})

	for _, item := range items {
		c := strings.TrimSpace(item.ExifInfo.Country)
		if c != "" {
			countrySet[c] = struct{}{}
		}
		s := strings.TrimSpace(item.ExifInfo.State)
		if s != "" {
			stateSet[s] = struct{}{}
		}
		city := strings.TrimSpace(item.ExifInfo.City)
		if city != "" {
			citySet[city] = struct{}{}
		}
		mod := strings.TrimSpace(item.ExifInfo.Model)
		if mod != "" {
			cameraSet[mod] = struct{}{}
		}
	}

	locs := LocationMetadata{
		Countries: mapToSortedSlice(countrySet),
		States:    mapToSortedSlice(stateSet),
		Cities:    mapToSortedSlice(citySet),
		Cameras:   mapToSortedSlice(cameraSet),
	}

	cache.SetWithExpiration(cacheKey, locs, 30*time.Minute)
	return locs, nil
}

func mapToSortedSlice(m map[string]struct{}) []string {
	res := make([]string, 0, len(m))
	for k := range m {
		res = append(res, k)
	}
	sort.Strings(res)
	return res
}
