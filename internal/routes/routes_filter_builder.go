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
	"time"

	"charm.land/log/v2"
	"github.com/damongolding/immich-kiosk/internal/common"
	"github.com/damongolding/immich-kiosk/internal/config"
	"github.com/damongolding/immich-kiosk/internal/immich"
	"github.com/damongolding/immich-kiosk/internal/templates/partials"
	"github.com/damongolding/immich-kiosk/internal/templates/views"
	"github.com/labstack/echo/v5"
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

		im := immich.New(com.Context(), requestConfig)

		ppl, pplErr := im.AllNamedPeople(requestID, deviceID)
		if pplErr != nil {
			log.Warn("filter builder: fetching people", "err", pplErr)
		}

		albs, albErr := im.AllAlbums(requestID, deviceID)
		if albErr != nil {
			log.Warn("filter builder: fetching albums", "err", albErr)
		}

		tags, _, tagsErr := im.AllTags(requestID, deviceID)
		if tagsErr != nil {
			log.Warn("filter builder: fetching tags", "err", tagsErr)
		}

		locs, locsErr := im.AllLocations(requestID, deviceID)
		if locsErr != nil {
			log.Warn("filter builder: fetching locations", "err", locsErr)
		}

		var users []string
		for u := range baseConfig.ImmichUsersAPIKeys {
			uClean := strings.TrimSpace(u)
			if uClean != "" {
				users = append(users, uClean)
			}
		}
		sort.Strings(users)

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
			People:        ppl,
			Albums:        albs,
			Tags:          tags,
			Countries:     locs.Countries,
			States:        locs.States,
			Cities:        locs.Cities,
			Cameras:       locs.Cameras,
			InitialFilter: initialFilter,
		}

		return Render(c, http.StatusOK, views.FilterBuilder(filterBuilderData))
	}
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
			if uClean != "" {
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
		var items []common.FilterBuilderAssetItem
		totalCount := 0

		type userQuery struct {
			user   string
			filter immich.SearchFilter
		}
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

		perUserLimit := limit / len(queriesToRun)
		if perUserLimit < 1 {
			perUserLimit = 1
		}

		var searchErrors []string
		for _, q := range queriesToRun {
			userCfg := requestConfig
			if q.user != "" {
				userCfg.SelectedUser = q.user
			}

			imAsset := immich.New(com.Context(), userCfg)
			resp, err := imAsset.SearchMetadataWithFilter(context.Background(), q.filter, perUserLimit, 1, requestID, deviceID)
			if err != nil {
				log.Error("filter builder search error", "user", q.user, "error", err)
				searchErrors = append(searchErrors, fmt.Sprintf("Error fetching assets for user %q: %s", q.user, err.Error()))
				continue
			}

			totalCount += resp.Assets.Total
			for _, a := range resp.Assets.Items {
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

				items = append(items, common.FilterBuilderAssetItem{
					ID:           a.ID,
					ThumbnailURL: thumbnailURL,
					User:         q.user,
					Type:         string(a.Type),
					City:         a.ExifInfo.City,
					Country:      a.ExifInfo.Country,
					TakenAt:      takenAt,
					IsFavorite:   a.IsFavorite,
					Rating:       a.ExifInfo.Rating,
					People:       peopleNames,
					Filename:     a.OriginalFileName,
				})

				if len(items) >= limit {
					break
				}
			}
			if len(items) >= limit {
				break
			}
		}

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

