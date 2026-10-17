package immich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"charm.land/log/v2"
	"github.com/damongolding/immich-kiosk/internal/cache"
	"github.com/damongolding/immich-kiosk/internal/kiosk"
)

// UserFilterMap maps usernames to their respective Immich SearchFilters.
// A key of "" or "*" represents a default/global filter applicable to any user.
type UserFilterMap map[string]SearchFilter

// ForUser returns the SearchFilter for the given username.
// It checks in order:
// 1. Explicit username match
// 2. Default key ("*", "", or "default")
// 3. If there is only one filter in the map, that filter is used.
func (m UserFilterMap) ForUser(username string) (SearchFilter, bool) {
	if len(m) == 0 {
		return SearchFilter{}, false
	}

	if f, exists := m[username]; exists {
		return f, true
	}

	if f, exists := m["*"]; exists {
		return f, true
	}

	if f, exists := m[""]; exists {
		return f, true
	}

	if f, exists := m["default"]; exists {
		return f, true
	}

	if len(m) == 1 {
		for _, f := range m {
			return f, true
		}
	}

	return SearchFilter{}, false
}

// KnownSearchFilterFields are top-level fields recognized as belonging to a SearchFilter.
var KnownSearchFilterFields = map[string]bool{
	"albumIds":         true,
	"checksum":         true,
	"city":             true,
	"country":          true,
	"createdAt":        true,
	"description":      true,
	"encodedVideoPath": true,
	"fileSizeInBytes":  true,
	"hasAlbums":        true,
	"hasPeople":        true,
	"hasTags":          true,
	"id":               true,
	"isEncoded":        true,
	"isFavorite":       true,
	"isMotion":         true,
	"isOffline":        true,
	"lensModel":        true,
	"libraryId":        true,
	"make":             true,
	"model":            true,
	"ocr":              true,
	"or":               true,
	"originalFileName": true,
	"originalPath":     true,
	"personIds":        true,
	"rating":           true,
	"state":            true,
	"tagIds":           true,
	"takenAt":          true,
	"type":             true,
	"updatedAt":        true,
	"visibility":       true,
}

// ParseUserFilters parses a raw JSON filter string.
// It supports two shapes:
// 1. A single SearchFilter, e.g.: {"isFavorite": true, "country": "Japan"}
//    -> Returns UserFilterMap{"": filter}
// 2. A map of user filters, e.g.: {"rafael": {"country": "Spain"}, "maria": {"country": "Italy"}}
//    -> Returns UserFilterMap{"rafael": filter1, "maria": filter2}
func ParseUserFilters(rawJSON string, knownUsers []string) (UserFilterMap, error) {
	rawJSON = strings.TrimSpace(rawJSON)
	if rawJSON == "" {
		return nil, nil
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rawJSON), &rawMap); err != nil {
		return nil, fmt.Errorf("parsing filter JSON: %w", err)
	}

	// Check if this map is a single SearchFilter by looking for known SearchFilter keys.
	isSingleFilter := false
	for k := range rawMap {
		if KnownSearchFilterFields[k] {
			isSingleFilter = true
			break
		}
	}

	if isSingleFilter {
		var filter SearchFilter
		if err := json.Unmarshal([]byte(rawJSON), &filter); err != nil {
			return nil, fmt.Errorf("parsing search filter: %w", err)
		}
		return UserFilterMap{"": filter}, nil
	}

	// Otherwise, treat keys as usernames or multi-user definitions.
	userMap := make(UserFilterMap)
	for username, userJSON := range rawMap {
		var filter SearchFilter
		if err := json.Unmarshal(userJSON, &filter); err != nil {
			return nil, fmt.Errorf("parsing search filter for user %q: %w", username, err)
		}
		userMap[username] = filter
	}

	return userMap, nil
}

// SearchMetadataWithFilter performs a search using a SearchFilter against Immich Search API v2 metadata endpoint.
func (a *Asset) SearchMetadataWithFilter(ctx context.Context, filter SearchFilter, size int, page int, requestID, deviceID string) (SearchMetadataResponse, error) {
	var response SearchMetadataResponse

	u, err := url.Parse(a.requestConfig.ImmichURL)
	if err != nil {
		return response, err
	}

	if size <= 0 {
		size = 24
	}

	requestBody := SearchRandomBody{
		Filter:     filter,
		Size:       size,
		WithExif:   true,
		WithPeople: true,
	}

	apiURL := url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   SearchMetadataEndpoint,
	}

	if page > 1 {
		q := apiURL.Query()
		q.Set("page", fmt.Sprintf("%d", page))
		apiURL.RawQuery = q.Encode()
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return response, err
	}

	apiBody, _, _, err := a.immichAPICall(ctx, http.MethodPost, apiURL.String(), jsonBody)
	if err != nil {
		return response, err
	}

	if err = json.Unmarshal(apiBody, &response); err != nil {
		return response, err
	}

	return response, nil
}

// RandomAssetWithFilter retrieves a random asset matching a SearchFilter for the specified user.
func (a *Asset) RandomAssetWithFilter(userID string, requestID, deviceID string, isPrefetch bool) error {
	if isPrefetch {
		log.Debug(requestID, "PREFETCH", deviceID, "Getting asset with filter", true)
	} else {
		log.Debug(requestID + " Getting asset with filter")
	}

	if userID != "" {
		_, _ = a.ApplyUserFromAssetID(userID)
	}

	filters, err := ParseUserFilters(a.requestConfig.FilterJSON, a.requestConfig.URLParamUsers)
	if err != nil {
		return fmt.Errorf("parsing filter: %w", err)
	}

	filter, found := filters.ForUser(a.SelectedUser())
	if !found {
		return a.RandomAsset(requestID, deviceID, isPrefetch)
	}

	// Apply kiosk-wide constraints if not explicitly set in the filter
	if len(filter.Type.In) == 0 && filter.Type.Eq == "" {
		filter.Type = FilterAssetType{In: []AssetType{ImageType}}
		if a.requestConfig.ShowVideos {
			filter.Type.In = append(filter.Type.In, VideoType)
		}
	}
	if len(filter.Visibility.In) == 0 && filter.Visibility.Eq == "" {
		filter.Visibility = FilterAssetVisibility{In: []AssetVisibility{Timeline}}
		if a.requestConfig.ShowArchived {
			filter.Visibility.In = append(filter.Visibility.In, Archive)
		}
	}

	fetchSize := a.requestConfig.Kiosk.FetchedAssetsSize
	if fetchSize <= 0 {
		fetchSize = 100
	}

	requestBody := SearchRandomBody{
		Filter:     filter,
		WithExif:   true,
		WithPeople: true,
		Size:       fetchSize,
	}

	for range MaxRetries {
		immichAssets, apiURL, err := a.fetchAssets(requestID, deviceID, requestBody)
		if err != nil {
			return err
		}

		apiCacheKey := cache.APICacheKey(apiURL.String(), deviceID, a.requestConfig.SelectedUser)

		if len(immichAssets) == 0 {
			log.Debug(requestID + " No assets matching filter in cache. Refreshing and trying again")
			cache.Delete(apiCacheKey)
			continue
		}

		wantedAssetType := ImageOnlyAssetTypes
		if a.requestConfig.ShowVideos {
			wantedAssetType = AllAssetTypes
		}

		for immichAssetIndex, asset := range immichAssets {
			asset.Bucket = kiosk.SourceFilter
			asset.requestConfig = a.requestConfig
			asset.ctx = a.ctx

			if !asset.isValidAsset(requestID, deviceID, wantedAssetType, a.RatioWanted) {
				continue
			}

			if a.requestConfig.Kiosk.Cache {
				immichAssetsToCache := slices.Delete(immichAssets, immichAssetIndex, immichAssetIndex+1)
				jsonBytes, cacheMarshalErr := json.Marshal(immichAssetsToCache)
				if cacheMarshalErr != nil {
					log.Error("Failed to marshal immichAssetsToCache", "error", cacheMarshalErr)
					return cacheMarshalErr
				}
				cache.Set(apiCacheKey, jsonBytes, a.requestConfig.Duration, a.requestConfig.CacheDuration)
			}

			asset.BucketID = string(kiosk.SourceFilter)
			*a = asset
			return nil
		}

		log.Debug(requestID + " No viable assets left in filter cache. Refreshing and trying again")
		cache.Delete(apiCacheKey)
	}

	return errors.New("no assets found matching filter. Max retries reached")
}
