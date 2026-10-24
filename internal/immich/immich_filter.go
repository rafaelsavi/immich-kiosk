package immich

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"charm.land/log/v2"
	"github.com/damongolding/immich-kiosk/internal/cache"
	"github.com/damongolding/immich-kiosk/internal/kiosk"
)

type SearchFilterBranch struct {
	AlbumIDs         IDsFilter              `url:"albumIds,omitempty,omitzero" json:"albumIds,omitzero"`
	Checksum         StringFilter           `url:"checksum,omitempty,omitzero" json:"checksum,omitzero"`
	City             StringFilterNullable   `url:"city,omitempty,omitzero" json:"city,omitzero"`
	Country          StringFilterNullable   `url:"country,omitempty,omitzero" json:"country,omitzero"`
	CreatedAt        DateFilter             `url:"createdAt,omitempty,omitzero" json:"createdAt,omitzero"`
	Description      StringPatternFilter    `url:"description,omitempty,omitzero" json:"description,omitzero"`
	EncodedVideoPath StringFilter           `url:"encodedVideoPath,omitempty,omitzero" json:"encodedVideoPath,omitzero"`
	FileSizeInBytes  NumberFilter           `url:"fileSizeInBytes,omitempty,omitzero" json:"fileSizeInBytes,omitzero"`
	HasAlbums        BoolFilter             `url:"hasAlbums,omitempty,omitzero" json:"hasAlbums,omitzero"`
	HasPeople        BoolFilter             `url:"hasPeople,omitempty,omitzero" json:"hasPeople,omitzero"`
	HasTags          BoolFilter             `url:"hasTags,omitempty,omitzero" json:"hasTags,omitzero"`
	ID               IDFilter               `url:"id,omitempty,omitzero" json:"id,omitzero"`
	IsEncoded        BoolFilter             `url:"isEncoded,omitempty,omitzero" json:"isEncoded,omitzero"`
	IsFavorite       BoolFilter             `url:"isFavorite,omitempty,omitzero" json:"isFavorite,omitzero"`
	IsMotion         BoolFilter             `url:"isMotion,omitempty,omitzero" json:"isMotion,omitzero"`
	IsOffline        BoolFilter             `url:"isOffline,omitempty,omitzero" json:"isOffline,omitzero"`
	LensModel        StringFilterNullable   `url:"lensModel,omitempty,omitzero" json:"lensModel,omitzero"`
	LibraryID        IDFilterNullable       `url:"libraryId,omitempty,omitzero" json:"libraryId,omitzero"`
	Make             StringFilterNullable   `url:"make,omitempty,omitzero" json:"make,omitzero"`
	Model            StringFilterNullable   `url:"model,omitempty,omitzero" json:"model,omitzero"`
	Ocr              StringSimilarityFilter `url:"ocr,omitempty,omitzero" json:"ocr,omitzero"`
	OriginalFileName StringPatternFilter    `url:"originalFileName,omitempty,omitzero" json:"originalFileName,omitzero"`
	OriginalPath     StringPatternFilter    `url:"originalPath,omitempty,omitzero" json:"originalPath,omitzero"`
	PersonIDs        IDsFilter              `url:"personIds,omitempty,omitzero" json:"personIds,omitzero"`
	Rating           NumberFilterNullable   `url:"rating,omitempty,omitzero" json:"rating,omitzero"`
	State            StringFilterNullable   `json:"state,omitzero"`
	TagIDs           IDsFilter              `url:"tagIds,omitempty,omitzero" json:"tagIds,omitzero"`
	TakenAt          DateFilter             `url:"takenAt,omitempty,omitzero" json:"takenAt,omitzero"`
	Type             FilterAssetType        `url:"type,omitempty,omitzero" json:"type,omitzero"`
	UpdatedAt        DateFilter             `url:"updatedAt,omitempty,omitzero" json:"updatedAt,omitzero"`
	Visibility       FilterAssetVisibility  `url:"visibility,omitempty,omitzero" json:"visibility,omitzero"`
}

func (s *StringFilterNullable) UnmarshalJSON(data []byte) error {
	var strVal string
	if err := json.Unmarshal(data, &strVal); err == nil {
		s.Eq = strVal
		return nil
	}
	var sliceVal []string
	if err := json.Unmarshal(data, &sliceVal); err == nil {
		s.In = sliceVal
		return nil
	}
	type rawStringFilterNullable StringFilterNullable
	var raw rawStringFilterNullable
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = StringFilterNullable(raw)
	return nil
}

func (b *BoolFilter) UnmarshalJSON(data []byte) error {
	var boolVal bool
	if err := json.Unmarshal(data, &boolVal); err == nil {
		b.Eq = &boolVal
		return nil
	}
	var raw struct {
		Eq *bool `json:"eq"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	b.Eq = raw.Eq
	return nil
}

func parseFlexibleTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse date string: %q", s)
}

func (d *DateFilter) UnmarshalJSON(data []byte) error {
	var strVal string
	if err := json.Unmarshal(data, &strVal); err == nil {
		t, err := parseFlexibleTime(strVal)
		if err != nil {
			return err
		}
		d.Gte = t
		return nil
	}

	var raw struct {
		Eq  any `json:"eq,omitempty"`
		Gt  any `json:"gt,omitempty"`
		Gte any `json:"gte,omitempty"`
		Lt  any `json:"lt,omitempty"`
		Lte any `json:"lte,omitempty"`
		Ne  any `json:"ne,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	parseVal := func(v any) (time.Time, error) {
		if v == nil {
			return time.Time{}, nil
		}
		if s, ok := v.(string); ok && s != "" {
			return parseFlexibleTime(s)
		}
		return time.Time{}, fmt.Errorf("invalid date value: %v", v)
	}

	if raw.Eq != nil {
		t, err := parseVal(raw.Eq)
		if err != nil {
			return err
		}
		d.Eq = t
	}
	if raw.Gt != nil {
		t, err := parseVal(raw.Gt)
		if err != nil {
			return err
		}
		d.Gt = t
	}
	if raw.Gte != nil {
		t, err := parseVal(raw.Gte)
		if err != nil {
			return err
		}
		d.Gte = t
	}
	if raw.Lt != nil {
		t, err := parseVal(raw.Lt)
		if err != nil {
			return err
		}
		d.Lt = t
	}
	if raw.Lte != nil {
		t, err := parseVal(raw.Lte)
		if err != nil {
			return err
		}
		d.Lte = t
	}
	if raw.Ne != nil {
		t, err := parseVal(raw.Ne)
		if err != nil {
			return err
		}
		d.Ne = t
	}
	return nil
}

func (ids *IDsFilter) UnmarshalJSON(data []byte) error {
	var sliceVal []string
	if err := json.Unmarshal(data, &sliceVal); err == nil {
		ids.Any = sliceVal
		return nil
	}
	var strVal string
	if err := json.Unmarshal(data, &strVal); err == nil {
		ids.Any = []string{strVal}
		return nil
	}
	type rawIDsFilter IDsFilter
	var raw rawIDsFilter
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*ids = IDsFilter(raw)
	return nil
}

func (n *NumberFilterNullable) UnmarshalJSON(data []byte) error {
	var numVal float64
	if err := json.Unmarshal(data, &numVal); err == nil {
		n.Eq = numVal
		return nil
	}
	type rawNumberFilterNullable NumberFilterNullable
	var raw rawNumberFilterNullable
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*n = NumberFilterNullable(raw)
	return nil
}

func (f *FilterAssetType) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		f.In = []AssetType{AssetType(single)}
		return nil
	}
	var list []AssetType
	if err := json.Unmarshal(data, &list); err == nil {
		f.In = list
		return nil
	}
	type rawFilterAssetType FilterAssetType
	var raw rawFilterAssetType
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*f = FilterAssetType(raw)
	return nil
}

func (s *SearchFilter) UnmarshalJSON(data []byte) error {
	type rawSearchFilter SearchFilter
	var raw struct {
		rawSearchFilter
		TakenAfter  any `json:"takenAfter,omitempty"`
		TakenBefore any `json:"takenBefore,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = SearchFilter(raw.rawSearchFilter)
	if raw.TakenAfter != nil {
		if str, ok := raw.TakenAfter.(string); ok && str != "" {
			if t, err := parseFlexibleTime(str); err == nil {
				s.TakenAt.Gte = t
			}
		}
	}
	if raw.TakenBefore != nil {
		if str, ok := raw.TakenBefore.(string); ok && str != "" {
			if t, err := parseFlexibleTime(str); err == nil {
				s.TakenAt.Lte = t
			}
		}
	}
	return nil
}

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
	"takenAfter":       true,
	"takenAt":          true,
	"takenBefore":      true,
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

	// If any value in rawMap is NOT a JSON object (e.g. string, number, bool, array),
	// this cannot be a map of username -> SearchFilter (since SearchFilter is always an object).
	if !isSingleFilter {
		for _, rawVal := range rawMap {
			trimmed := bytes.TrimSpace(rawVal)
			if len(trimmed) > 0 && trimmed[0] != '{' {
				isSingleFilter = true
				break
			}
		}
	}

	// Also, if knownUsers are provided and NONE of the keys in rawMap match a known user,
	// treat it as a single filter.
	if !isSingleFilter && len(knownUsers) > 0 {
		hasKnownUser := false
		for k := range rawMap {
			for _, u := range knownUsers {
				if strings.EqualFold(k, u) {
					hasKnownUser = true
					break
				}
			}
			if hasKnownUser {
				break
			}
		}
		if !hasKnownUser {
			isSingleFilter = true
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

	// Apply kiosk-wide constraints if not explicitly set in the filter, matching RandomAssetWithFilter
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

	requestBody := SearchRandomBody{
		Filter:     filter,
		Size:       size,
		WithExif:   true,
		WithPeople: true,
	}

	apiURL := u.JoinPath(SearchMetadataEndpoint)
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
		// Fallback to SearchRandomEndpoint if metadata search endpoint fails or is rejected
		randomURL := u.JoinPath(SearchRandomEndpoint)
		randomBody, _, _, errRandom := a.immichAPICall(ctx, http.MethodPost, randomURL.String(), jsonBody)
		if errRandom != nil {
			return response, err
		}
		var randomAssets []Asset
		if errUnmarshal := json.Unmarshal(randomBody, &randomAssets); errUnmarshal == nil {
			response.Assets.Items = randomAssets
			response.Assets.Total = len(randomAssets)
			return response, nil
		}
		return response, err
	}

	if err = json.Unmarshal(apiBody, &response); err != nil || len(response.Assets.Items) == 0 {
		var listAssets []Asset
		if errList := json.Unmarshal(apiBody, &listAssets); errList == nil && len(listAssets) > 0 {
			response.Assets.Items = listAssets
			response.Assets.Total = len(listAssets)
			return response, nil
		}
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
