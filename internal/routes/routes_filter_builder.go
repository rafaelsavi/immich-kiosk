package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/log/v2"
	"github.com/damongolding/immich-kiosk/internal/common"
	"github.com/damongolding/immich-kiosk/internal/config"
	"github.com/damongolding/immich-kiosk/internal/immich"
	"github.com/damongolding/immich-kiosk/internal/templates/partials"
	"github.com/damongolding/immich-kiosk/internal/templates/views"
	"github.com/labstack/echo/v5"
	"golang.org/x/sync/errgroup"
)

// FilterBuilderPage renders the interactive filter builder and simulation page.
func FilterBuilderPage(baseConfig *config.Config, com *common.Common) echo.HandlerFunc {
	return func(c *echo.Context) error {
		requestData, err := InitializeRequestData(c, baseConfig)
		if err != nil {
			return err
		}

		if requestData == nil {
			log.Info("Refreshing clients")
			return nil
		}

		requestConfig := requestData.RequestConfig
		requestID := requestData.RequestID
		deviceID := requestData.DeviceID

		log.Debug(
			requestID,
			"method", c.Request().Method,
			"path", c.Request().URL.String(),
			"requestConfig", requestConfig.String(),
		)

		var users []string
		for u := range baseConfig.ImmichUsersAPIKeys {
			uClean := strings.TrimSpace(u)
			if uClean != "" && uClean != "default" {
				users = append(users, uClean)
			}
		}
		sort.Strings(users)

		selectedUser := strings.TrimSpace(c.QueryParam("user"))
		if selectedUser == "all" {
			selectedUser = ""
		}

		var usersToFetch []string
		isMultiUser := false

		if selectedUser != "" {
			usersToFetch = []string{selectedUser}
		} else if len(users) > 0 {
			usersToFetch = users
			if len(users) > 1 {
				isMultiUser = true
			}
		} else {
			usersToFetch = []string{""}
		}

		var g errgroup.Group
		var mu sync.Mutex

		seenPeopleIDs := make(map[string]bool)
		seenAlbumIDs := make(map[string]bool)
		seenTagIDs := make(map[string]bool)

		countrySet := make(map[string]struct{})
		stateSet := make(map[string]struct{})
		citySet := make(map[string]struct{})
		cameraSet := make(map[string]struct{})

		var allPeople []immich.Person
		var allAlbums immich.Albums
		var allTags immich.Tags

		for _, u := range usersToFetch {
			username := u
			g.Go(func() error {
				userCfg := requestConfig
				if username != "" {
					userCfg.SelectedUser = username
					if key, ok := baseConfig.ImmichUsersAPIKeys[username]; ok {
						userCfg.ImmichAPIKey = key
					}
				}
				im := immich.New(com.Context(), userCfg)

				userPeople, pplErr := im.AllNamedPeople(requestID, deviceID)
				if pplErr != nil {
					log.Warn("filter builder: fetching people", "user", username, "err", pplErr)
				}

				userAlbums, albErr := im.AllAlbums(requestID, deviceID)
				if albErr != nil {
					log.Warn("filter builder: fetching albums", "user", username, "err", albErr)
				}

				userTags, _, tagsErr := im.AllTags(requestID, deviceID)
				if tagsErr != nil {
					log.Warn("filter builder: fetching tags", "user", username, "err", tagsErr)
				}

				userLocs, locsErr := im.AllLocations(requestID, deviceID)
				if locsErr != nil {
					log.Warn("filter builder: fetching locations", "user", username, "err", locsErr)
				}

				mu.Lock()
				defer mu.Unlock()

				for _, p := range userPeople {
					if !seenPeopleIDs[p.ID] {
						seenPeopleIDs[p.ID] = true
						if isMultiUser && username != "" {
							p.Name = fmt.Sprintf("%s (@%s)", p.Name, username)
						}
						allPeople = append(allPeople, p)
					}
				}

				for _, a := range userAlbums {
					if !seenAlbumIDs[a.ID] {
						seenAlbumIDs[a.ID] = true
						if isMultiUser && username != "" {
							a.AlbumName = fmt.Sprintf("%s (@%s)", a.AlbumName, username)
						}
						allAlbums = append(allAlbums, a)
					}
				}

				for _, t := range userTags {
					tagKey := t.ID
					if tagKey == "" {
						tagKey = strings.ToLower(t.Name)
					}
					if !seenTagIDs[tagKey] {
						seenTagIDs[tagKey] = true
						allTags = append(allTags, t)
					}
				}

				for _, c := range userLocs.Countries {
					cClean := strings.TrimSpace(c)
					if cClean != "" {
						countrySet[cClean] = struct{}{}
					}
				}
				for _, s := range userLocs.States {
					sClean := strings.TrimSpace(s)
					if sClean != "" {
						stateSet[sClean] = struct{}{}
					}
				}
				for _, c := range userLocs.Cities {
					cClean := strings.TrimSpace(c)
					if cClean != "" {
						citySet[cClean] = struct{}{}
					}
				}
				for _, m := range userLocs.Cameras {
					mClean := strings.TrimSpace(m)
					if mClean != "" {
						cameraSet[mClean] = struct{}{}
					}
				}

				return nil
			})
		}
		_ = g.Wait()

		sort.Slice(allPeople, func(i, j int) bool {
			return strings.ToLower(allPeople[i].Name) < strings.ToLower(allPeople[j].Name)
		})

		sort.Slice(allAlbums, func(i, j int) bool {
			return strings.ToLower(allAlbums[i].AlbumName) < strings.ToLower(allAlbums[j].AlbumName)
		})

		sort.Slice(allTags, func(i, j int) bool {
			return strings.ToLower(allTags[i].Name) < strings.ToLower(allTags[j].Name)
		})

		countries := mapToSortedSlice(countrySet)
		states := mapToSortedSlice(stateSet)
		cities := mapToSortedSlice(citySet)
		cameras := mapToSortedSlice(cameraSet)

		viewData := common.ViewData{
			KioskVersion: KioskVersion,
			RequestID:    requestID,
			DeviceID:     deviceID,
			Config:       requestConfig,
		}

		initialFilter := strings.TrimSpace(c.QueryParam("filter"))

		filterBuilderData := common.FilterBuilderViewData{
			ViewData:      viewData,
			Users:         users,
			SelectedUser:  selectedUser,
			People:        allPeople,
			Albums:        allAlbums,
			Tags:          allTags,
			Countries:     countries,
			States:        states,
			Cities:        cities,
			Cameras:       cameras,
			InitialFilter: initialFilter,
		}

		return Render(c, http.StatusOK, views.FilterBuilder(filterBuilderData))
	}
}

func mapToSortedSlice(m map[string]struct{}) []string {
	res := make([]string, 0, len(m))
	for k := range m {
		res = append(res, k)
	}
	sort.Strings(res)
	return res
}

// FilterBuilderSearch handles the POST /url-builder/filters/search request triggered via htmx.
func FilterBuilderSearch(baseConfig *config.Config, com *common.Common) echo.HandlerFunc {
	return func(c *echo.Context) error {
		requestData, err := InitializeRequestData(c, baseConfig)
		if err != nil {
			return err
		}

		if requestData == nil {
			return nil
		}

		requestConfig := requestData.RequestConfig
		requestID := requestData.RequestID
		deviceID := requestData.DeviceID

		if err := c.Request().ParseForm(); err != nil {
			return echo.NewHTTPError(http.StatusUnprocessableEntity, "invalid form")
		}

		selectedUser := strings.TrimSpace(c.FormValue("user"))
		limitStr := strings.TrimSpace(c.FormValue("limit"))
		limit := 24
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			if l > 100 {
				limit = 100
			} else {
				limit = l
			}
		}

		rawFilter := strings.TrimSpace(c.FormValue("raw_filter"))
		if rawFilter == "" {
			rawFilter = strings.TrimSpace(c.FormValue("filter"))
		}

		var knownUsers []string
		for u := range baseConfig.ImmichUsersAPIKeys {
			uClean := strings.TrimSpace(u)
			if uClean != "" && uClean != "default" {
				knownUsers = append(knownUsers, uClean)
			}
		}
		sort.Strings(knownUsers)

		var filterRaw string
		var userFilters immich.UserFilterMap

		// Check visual form fields
		favorite := strings.TrimSpace(c.FormValue("favorite"))
		typeVal := strings.TrimSpace(c.FormValue("type"))
		ratingStr := strings.TrimSpace(c.FormValue("rating"))
		ratingVal, _ := strconv.Atoi(ratingStr)
		people := cleanSlice(c.Request().Form["people"])
		hasPeople := strings.TrimSpace(c.FormValue("has_people"))
		albums := cleanSlice(c.Request().Form["albums"])
		hasAlbums := strings.TrimSpace(c.FormValue("has_albums"))
		tags := cleanSlice(c.Request().Form["tags"])
		hasTags := strings.TrimSpace(c.FormValue("has_tags"))
		countries := cleanSlice(c.Request().Form["country"])
		states := cleanSlice(c.Request().Form["state"])
		cities := cleanSlice(c.Request().Form["city"])
		cameras := cleanSlice(c.Request().Form["camera"])
		takenAfter := strings.TrimSpace(c.FormValue("taken_after"))
		takenBefore := strings.TrimSpace(c.FormValue("taken_before"))

		hasVisualCriteria := favorite != "" || typeVal != "" || ratingVal > 0 ||
			len(people) > 0 || hasPeople != "" || len(albums) > 0 || hasAlbums != "" ||
			len(tags) > 0 || hasTags != "" || len(countries) > 0 || len(states) > 0 || len(cities) > 0 ||
			len(cameras) > 0 || takenAfter != "" || takenBefore != ""

		if rawFilter != "" {
			parsedFilters, err := immich.ParseUserFilters(rawFilter, knownUsers)
			if err != nil {
				return Render(c, http.StatusOK, partials.FilterBuilderResult(common.FilterBuilderResultData{
					Error:      fmt.Sprintf("Invalid Filter JSON syntax or structure: %s", err.Error()),
					FilterJSON: rawFilter,
				}))
			}
			userFilters = parsedFilters
			var prettyBuf bytes.Buffer
			if err := json.Indent(&prettyBuf, []byte(rawFilter), "", "  "); err == nil {
				filterRaw = prettyBuf.String()
			} else {
				filterRaw = rawFilter
			}
		} else if hasVisualCriteria {
			var searchFilter immich.SearchFilter

			if favorite == "true" {
				t := true
				searchFilter.IsFavorite = immich.BoolFilter{Eq: &t}
			} else if favorite == "false" {
				f := false
				searchFilter.IsFavorite = immich.BoolFilter{Eq: &f}
			}

			if typeVal == "IMAGE" {
				searchFilter.Type = immich.FilterAssetType{In: []immich.AssetType{immich.ImageType}}
			} else if typeVal == "VIDEO" {
				searchFilter.Type = immich.FilterAssetType{In: []immich.AssetType{immich.VideoType}}
			} else if typeVal == "MOTION" {
				t := true
				searchFilter.IsMotion = immich.BoolFilter{Eq: &t}
			}

			if ratingVal > 0 {
				r := float64(ratingVal)
				searchFilter.Rating = immich.NumberFilterNullable{Gte: r}
			}

			if len(people) > 0 {
				searchFilter.PersonIDs = immich.IDsFilter{Any: people}
			}
			if hasPeople == "true" {
				t := true
				searchFilter.HasPeople = immich.BoolFilter{Eq: &t}
			} else if hasPeople == "false" {
				f := false
				searchFilter.HasPeople = immich.BoolFilter{Eq: &f}
			}

			if len(albums) > 0 {
				searchFilter.AlbumIDs = immich.IDsFilter{Any: albums}
			}
			if hasAlbums == "true" {
				t := true
				searchFilter.HasAlbums = immich.BoolFilter{Eq: &t}
			} else if hasAlbums == "false" {
				f := false
				searchFilter.HasAlbums = immich.BoolFilter{Eq: &f}
			}

			if len(tags) > 0 {
				searchFilter.TagIDs = immich.IDsFilter{Any: tags}
			}
			if hasTags == "true" {
				t := true
				searchFilter.HasTags = immich.BoolFilter{Eq: &t}
			} else if hasTags == "false" {
				f := false
				searchFilter.HasTags = immich.BoolFilter{Eq: &f}
			}

			if len(countries) == 1 {
				searchFilter.Country = immich.StringFilterNullable{Eq: countries[0]}
			} else if len(countries) > 1 {
				searchFilter.Country = immich.StringFilterNullable{In: countries}
			}

			if len(states) == 1 {
				searchFilter.State = immich.StringFilterNullable{Eq: states[0]}
			} else if len(states) > 1 {
				searchFilter.State = immich.StringFilterNullable{In: states}
			}

			if len(cities) == 1 {
				searchFilter.City = immich.StringFilterNullable{Eq: cities[0]}
			} else if len(cities) > 1 {
				searchFilter.City = immich.StringFilterNullable{In: cities}
			}

			if len(cameras) == 1 {
				searchFilter.Model = immich.StringFilterNullable{Eq: cameras[0]}
			} else if len(cameras) > 1 {
				searchFilter.Model = immich.StringFilterNullable{In: cameras}
			}

			if takenAfter != "" {
				if t, err := time.Parse("2006-01-02", takenAfter); err == nil {
					searchFilter.TakenAt.Gte = t
				}
			}
			if takenBefore != "" {
				if t, err := time.Parse("2006-01-02", takenBefore); err == nil {
					searchFilter.TakenAt.Lte = t
				}
			}

			prettyBytes, err := json.MarshalIndent(searchFilter, "", "  ")
			if err != nil {
				return Render(c, http.StatusOK, partials.FilterBuilderResult(common.FilterBuilderResultData{
					Error: fmt.Sprintf("Failed to encode filter: %s", err.Error()),
				}))
			}
			filterRaw = string(prettyBytes)
			userFilters = immich.UserFilterMap{"": searchFilter}
		} else {
			// No visual criteria or raw JSON provided: default to open search so simulation immediately shows real photos
			var searchFilter immich.SearchFilter
			filterRaw = "{}"
			userFilters = immich.UserFilterMap{"": searchFilter}
		}

		// Build Kiosk URL
		kioskHost := c.Request().Header.Get("X-Forwarded-Host")
		if kioskHost == "" {
			kioskHost = c.Request().Host
		}
		scheme := c.Scheme()
		if xf := c.Request().Header.Get("X-Forwarded-Proto"); xf != "" {
			s := strings.ToLower(xf)
			if s == "http" || s == "https" {
				scheme = s
			}
		}

		var compactBuf bytes.Buffer
		filterQuery := filterRaw
		if err := json.Compact(&compactBuf, []byte(filterRaw)); err == nil {
			filterQuery = compactBuf.String()
		}

		var kioskURL string
		if filterQuery == "{}" || filterQuery == "" {
			kioskURL = fmt.Sprintf("%s://%s/", scheme, kioskHost)
			if selectedUser != "" && selectedUser != "all" {
				kioskURL += "?user=" + url.QueryEscape(selectedUser)
			}
			if baseConfig.Kiosk.Password != "" {
				sep := "?"
				if strings.Contains(kioskURL, "?") {
					sep = "&"
				}
				kioskURL += sep + "password=" + url.QueryEscape(baseConfig.Kiosk.Password)
			}
		} else {
			kioskURL = fmt.Sprintf("%s://%s/?filter=%s", scheme, kioskHost, url.QueryEscape(filterQuery))
			if selectedUser != "" && selectedUser != "all" {
				kioskURL += "&user=" + url.QueryEscape(selectedUser)
			}
			if baseConfig.Kiosk.Password != "" {
				kioskURL += "&password=" + url.QueryEscape(baseConfig.Kiosk.Password)
			}
		}

		var redirectYAML string
		if filterQuery == "{}" || filterQuery == "" {
			redirectYAML = "redirects:\n  - name: \"Custom Filtered Slideshow\"\n    url: \"/\""
		} else {
			redirectYAML = fmt.Sprintf("redirects:\n  - name: \"Custom Filtered Slideshow\"\n    url: \"/?filter=%s", url.QueryEscape(filterQuery))
			if selectedUser != "" && selectedUser != "all" {
				redirectYAML += fmt.Sprintf("&user=%s", url.QueryEscape(selectedUser))
			}
			redirectYAML += "\""
		}

		// Execute queries against Immich API
		var queriesToRun []userQuery

		if selectedUser != "" && selectedUser != "all" {
			if f, ok := userFilters.ForUser(selectedUser); ok {
				queriesToRun = append(queriesToRun, userQuery{user: selectedUser, filter: f})
			} else {
				for _, f := range userFilters {
					queriesToRun = append(queriesToRun, userQuery{user: selectedUser, filter: f})
					break
				}
			}
		} else {
			hasSpecificUsers := false
			for u := range userFilters {
				if u != "" && u != "*" && u != "default" {
					hasSpecificUsers = true
					break
				}
			}

			if hasSpecificUsers {
				for u, f := range userFilters {
					if u != "" && u != "*" && u != "default" {
						queriesToRun = append(queriesToRun, userQuery{user: u, filter: f})
					}
				}
			} else if len(knownUsers) > 1 {
				for _, u := range knownUsers {
					if f, ok := userFilters.ForUser(u); ok {
						queriesToRun = append(queriesToRun, userQuery{user: u, filter: f})
					}
				}
			} else {
				for _, f := range userFilters {
					queriesToRun = append(queriesToRun, userQuery{user: "", filter: f})
					break
				}
			}
		}

		if len(queriesToRun) == 0 {
			for _, f := range userFilters {
				queriesToRun = append(queriesToRun, userQuery{user: "", filter: f})
				break
			}
		}

		var g errgroup.Group
		var mu sync.Mutex
		userItemLists := make([][]common.FilterBuilderAssetItem, len(queriesToRun))
		var searchErrors []string

		for idx, query := range queriesToRun {
			i := idx
			q := query
			g.Go(func() error {
				uItems, err := fetchUserSample(
					context.Background(),
					q,
					baseConfig,
					requestConfig,
					com,
					requestID,
					deviceID,
					limit,
					countries,
					states,
					cities,
					cameras,
					people,
					albums,
					tags,
				)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					log.Error("filter builder search error", "user", q.user, "error", err)
					searchErrors = append(searchErrors, fmt.Sprintf("Error fetching assets for user %q: %s", q.user, err.Error()))
					return nil
				}
				userItemLists[i] = uItems
				return nil
			})
		}
		_ = g.Wait()

		items := balanceAndInterleaveItems(userItemLists, limit, countries, states, cities, cameras)
		totalCount := len(items)

		errMsg := ""
		if len(searchErrors) > 0 && len(items) == 0 {
			errMsg = strings.Join(searchErrors, " | ")
		}

		return Render(c, http.StatusOK, partials.FilterBuilderResult(common.FilterBuilderResultData{
			Total:        totalCount,
			Assets:       items,
			FilterJSON:   filterRaw,
			KioskURL:     kioskURL,
			RedirectYAML: redirectYAML,
			Error:        errMsg,
		}))
	}
}

type userQuery struct {
	user   string
	filter immich.SearchFilter
}

func fetchUserSample(
	ctx context.Context,
	q userQuery,
	baseConfig *config.Config,
	requestConfig config.Config,
	com *common.Common,
	requestID, deviceID string,
	limit int,
	countries, states, cities, cameras, people, albums, tags []string,
) ([]common.FilterBuilderAssetItem, error) {
	userCfg := requestConfig
	if q.user != "" {
		userCfg.SelectedUser = q.user
		if key, ok := baseConfig.ImmichUsersAPIKeys[q.user]; ok {
			userCfg.ImmichAPIKey = key
		}
	}

	imAsset := immich.New(com.Context(), userCfg)
	userAssets, err := imAsset.SearchRandomWithFilter(ctx, q.filter, limit, requestID, deviceID)
	if err != nil {
		return nil, err
	}

	// Check if any multi-selected items were starved/missing from the random sample
	if len(countries) > 1 {
		hasCountry := make(map[string]bool)
		for _, a := range userAssets {
			c := strings.ToLower(strings.TrimSpace(a.ExifInfo.Country))
			if c != "" {
				hasCountry[c] = true
			}
		}
		for _, c := range countries {
			cClean := strings.ToLower(strings.TrimSpace(c))
			if !hasCountry[cClean] {
				missingFilter := q.filter
				missingFilter.Country = immich.StringFilterNullable{Eq: c}
				if extra, err := imAsset.SearchRandomWithFilter(ctx, missingFilter, 6, requestID, deviceID); err == nil && len(extra) > 0 {
					userAssets = append(userAssets, extra...)
				}
			}
		}
	}

	if len(cities) > 1 {
		hasCity := make(map[string]bool)
		for _, a := range userAssets {
			city := strings.ToLower(strings.TrimSpace(a.ExifInfo.City))
			if city != "" {
				hasCity[city] = true
			}
		}
		for _, city := range cities {
			cityClean := strings.ToLower(strings.TrimSpace(city))
			if !hasCity[cityClean] {
				missingFilter := q.filter
				missingFilter.City = immich.StringFilterNullable{Eq: city}
				if extra, err := imAsset.SearchRandomWithFilter(ctx, missingFilter, 6, requestID, deviceID); err == nil && len(extra) > 0 {
					userAssets = append(userAssets, extra...)
				}
			}
		}
	}

	if len(states) > 1 {
		hasState := make(map[string]bool)
		for _, a := range userAssets {
			st := strings.ToLower(strings.TrimSpace(a.ExifInfo.State))
			if st != "" {
				hasState[st] = true
			}
		}
		for _, st := range states {
			stClean := strings.ToLower(strings.TrimSpace(st))
			if !hasState[stClean] {
				missingFilter := q.filter
				missingFilter.State = immich.StringFilterNullable{Eq: st}
				if extra, err := imAsset.SearchRandomWithFilter(ctx, missingFilter, 6, requestID, deviceID); err == nil && len(extra) > 0 {
					userAssets = append(userAssets, extra...)
				}
			}
		}
	}

	if len(cameras) > 1 {
		hasCamera := make(map[string]bool)
		for _, a := range userAssets {
			cam := strings.ToLower(strings.TrimSpace(a.ExifInfo.Model))
			if cam != "" {
				hasCamera[cam] = true
			}
		}
		for _, cam := range cameras {
			camClean := strings.ToLower(strings.TrimSpace(cam))
			if !hasCamera[camClean] {
				missingFilter := q.filter
				missingFilter.Model = immich.StringFilterNullable{Eq: cam}
				if extra, err := imAsset.SearchRandomWithFilter(ctx, missingFilter, 6, requestID, deviceID); err == nil && len(extra) > 0 {
					userAssets = append(userAssets, extra...)
				}
			}
		}
	}

	if len(people) > 1 {
		hasPerson := make(map[string]bool)
		for _, a := range userAssets {
			for _, p := range a.People {
				hasPerson[p.ID] = true
			}
		}
		for _, pid := range people {
			if !hasPerson[pid] {
				missingFilter := q.filter
				missingFilter.PersonIDs = immich.IDsFilter{Any: []string{pid}}
				if extra, err := imAsset.SearchRandomWithFilter(ctx, missingFilter, 6, requestID, deviceID); err == nil && len(extra) > 0 {
					userAssets = append(userAssets, extra...)
				}
			}
		}
	}

	if len(albums) > 1 {
		for _, aid := range albums {
			missingFilter := q.filter
			missingFilter.AlbumIDs = immich.IDsFilter{Any: []string{aid}}
			if extra, err := imAsset.SearchRandomWithFilter(ctx, missingFilter, 6, requestID, deviceID); err == nil && len(extra) > 0 {
				userAssets = append(userAssets, extra...)
			}
		}
	}

	if len(tags) > 1 {
		for _, tid := range tags {
			missingFilter := q.filter
			missingFilter.TagIDs = immich.IDsFilter{Any: []string{tid}}
			if extra, err := imAsset.SearchRandomWithFilter(ctx, missingFilter, 4, requestID, deviceID); err == nil && len(extra) > 0 {
				userAssets = append(userAssets, extra...)
			}
		}
	}

	var userItems []common.FilterBuilderAssetItem
	for _, a := range userAssets {
		thumbnailURL := fmt.Sprintf("/image/%s", a.ID)
		if q.user != "" {
			thumbnailURL += fmt.Sprintf("?user=%s", url.QueryEscape(q.user))
		}

		takenAt := ""
		if !a.ExifInfo.DateTimeOriginal.IsZero() {
			takenAt = a.ExifInfo.DateTimeOriginal.Format("2006-01-02 15:04")
		} else if !a.LocalDateTime.IsZero() {
			takenAt = a.LocalDateTime.Format("2006-01-02 15:04")
		}

		var peopleNames []string
		for _, p := range a.People {
			name := strings.TrimSpace(p.Name)
			if name != "" {
				peopleNames = append(peopleNames, name)
			}
		}

		userItems = append(userItems, common.FilterBuilderAssetItem{
			ID:           a.ID,
			ThumbnailURL: thumbnailURL,
			User:         q.user,
			Type:         string(a.Type),
			City:         a.ExifInfo.City,
			State:        a.ExifInfo.State,
			Country:      a.ExifInfo.Country,
			Model:        a.ExifInfo.Model,
			TakenAt:      takenAt,
			IsFavorite:   a.IsFavorite,
			Rating:       a.ExifInfo.Rating,
			People:       peopleNames,
			Filename:     a.OriginalFileName,
		})
	}

	return userItems, nil
}

func balanceAndInterleaveItems(
	userItemLists [][]common.FilterBuilderAssetItem,
	limit int,
	countries, states, cities, cameras []string,
) []common.FilterBuilderAssetItem {
	var balancedUserLists [][]common.FilterBuilderAssetItem
	for _, items := range userItemLists {
		if len(items) == 0 {
			continue
		}
		balanced := balanceCategoryItems(items, countries, states, cities, cameras)
		balancedUserLists = append(balancedUserLists, balanced)
	}

	return interleaveAssetItems(balancedUserLists, limit)
}

func balanceCategoryItems(
	items []common.FilterBuilderAssetItem,
	countries, states, cities, cameras []string,
) []common.FilterBuilderAssetItem {
	if len(items) <= 1 {
		return items
	}

	if len(countries) > 1 {
		bucketMap := make(map[string][]common.FilterBuilderAssetItem)
		for _, c := range countries {
			bucketMap[strings.ToLower(strings.TrimSpace(c))] = []common.FilterBuilderAssetItem{}
		}
		var other []common.FilterBuilderAssetItem
		for _, item := range items {
			key := strings.ToLower(strings.TrimSpace(item.Country))
			if _, ok := bucketMap[key]; ok {
				bucketMap[key] = append(bucketMap[key], item)
			} else {
				other = append(other, item)
			}
		}
		var groups [][]common.FilterBuilderAssetItem
		for _, c := range countries {
			key := strings.ToLower(strings.TrimSpace(c))
			if list := bucketMap[key]; len(list) > 0 {
				groups = append(groups, list)
			}
		}
		if len(other) > 0 {
			groups = append(groups, other)
		}
		return interleaveAssetItems(groups, len(items))
	}

	if len(cities) > 1 {
		bucketMap := make(map[string][]common.FilterBuilderAssetItem)
		for _, c := range cities {
			bucketMap[strings.ToLower(strings.TrimSpace(c))] = []common.FilterBuilderAssetItem{}
		}
		var other []common.FilterBuilderAssetItem
		for _, item := range items {
			key := strings.ToLower(strings.TrimSpace(item.City))
			if _, ok := bucketMap[key]; ok {
				bucketMap[key] = append(bucketMap[key], item)
			} else {
				other = append(other, item)
			}
		}
		var groups [][]common.FilterBuilderAssetItem
		for _, c := range cities {
			key := strings.ToLower(strings.TrimSpace(c))
			if list := bucketMap[key]; len(list) > 0 {
				groups = append(groups, list)
			}
		}
		if len(other) > 0 {
			groups = append(groups, other)
		}
		return interleaveAssetItems(groups, len(items))
	}

	if len(states) > 1 {
		bucketMap := make(map[string][]common.FilterBuilderAssetItem)
		for _, s := range states {
			bucketMap[strings.ToLower(strings.TrimSpace(s))] = []common.FilterBuilderAssetItem{}
		}
		var other []common.FilterBuilderAssetItem
		for _, item := range items {
			key := strings.ToLower(strings.TrimSpace(item.State))
			if _, ok := bucketMap[key]; ok {
				bucketMap[key] = append(bucketMap[key], item)
			} else {
				other = append(other, item)
			}
		}
		var groups [][]common.FilterBuilderAssetItem
		for _, s := range states {
			key := strings.ToLower(strings.TrimSpace(s))
			if list := bucketMap[key]; len(list) > 0 {
				groups = append(groups, list)
			}
		}
		if len(other) > 0 {
			groups = append(groups, other)
		}
		return interleaveAssetItems(groups, len(items))
	}

	if len(cameras) > 1 {
		bucketMap := make(map[string][]common.FilterBuilderAssetItem)
		for _, cam := range cameras {
			bucketMap[strings.ToLower(strings.TrimSpace(cam))] = []common.FilterBuilderAssetItem{}
		}
		var other []common.FilterBuilderAssetItem
		for _, item := range items {
			key := strings.ToLower(strings.TrimSpace(item.Model))
			if _, ok := bucketMap[key]; ok {
				bucketMap[key] = append(bucketMap[key], item)
			} else {
				other = append(other, item)
			}
		}
		var groups [][]common.FilterBuilderAssetItem
		for _, cam := range cameras {
			key := strings.ToLower(strings.TrimSpace(cam))
			if list := bucketMap[key]; len(list) > 0 {
				groups = append(groups, list)
			}
		}
		if len(other) > 0 {
			groups = append(groups, other)
		}
		return interleaveAssetItems(groups, len(items))
	}

	return items
}

func interleaveAssetItems(groups [][]common.FilterBuilderAssetItem, limit int) []common.FilterBuilderAssetItem {
	if len(groups) == 0 {
		return nil
	}
	if len(groups) == 1 {
		seen := make(map[string]bool)
		var res []common.FilterBuilderAssetItem
		for _, item := range groups[0] {
			if !seen[item.ID] {
				seen[item.ID] = true
				res = append(res, item)
				if len(res) >= limit {
					break
				}
			}
		}
		return res
	}

	seen := make(map[string]bool)
	var res []common.FilterBuilderAssetItem
	maxLen := 0
	for _, g := range groups {
		if len(g) > maxLen {
			maxLen = len(g)
		}
	}

	for i := 0; i < maxLen; i++ {
		for _, g := range groups {
			if i < len(g) {
				item := g[i]
				if !seen[item.ID] {
					seen[item.ID] = true
					res = append(res, item)
					if len(res) >= limit {
						return res
					}
				}
			}
		}
	}

	return res
}

func cleanSlice(s []string) []string {
	var res []string
	for _, v := range s {
		v = strings.TrimSpace(v)
		if v != "" {
			res = append(res, v)
		}
	}
	return res
}

