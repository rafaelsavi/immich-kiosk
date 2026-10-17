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

		filterBuilderData := common.FilterBuilderViewData{
			ViewData: viewData,
			Users:    users,
		}

		return Render(c, http.StatusOK, views.FilterBuilder(filterBuilderData))
	}
}

// FilterBuilderSearch handles the POST /filter-builder/search request triggered via htmx.
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

		filterRaw := strings.TrimSpace(c.FormValue("filter"))
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

		if filterRaw == "" {
			return Render(c, http.StatusOK, partials.FilterBuilderResult(common.FilterBuilderResultData{
				Error: "Please enter a valid JSON filter before running simulation.",
			}))
		}

		var knownUsers []string
		for u := range baseConfig.ImmichUsersAPIKeys {
			uClean := strings.TrimSpace(u)
			if uClean != "" {
				knownUsers = append(knownUsers, uClean)
			}
		}
		sort.Strings(knownUsers)

		userFilters, err := immich.ParseUserFilters(filterRaw, knownUsers)
		if err != nil {
			return Render(c, http.StatusOK, partials.FilterBuilderResult(common.FilterBuilderResultData{
				Error:      fmt.Sprintf("Invalid Filter JSON syntax or structure: %s", err.Error()),
				FilterJSON: filterRaw,
			}))
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

		kioskURL := fmt.Sprintf("%s://%s/?filter=%s", scheme, kioskHost, url.QueryEscape(filterQuery))
		if baseConfig.Kiosk.Password != "" {
			kioskURL += "&password=" + url.QueryEscape(baseConfig.Kiosk.Password)
		}

		redirectYAML := fmt.Sprintf("redirects:\n  - name: \"Custom Filtered Slideshow\"\n    url: \"/?filter=%s\"", url.QueryEscape(filterQuery))

		// Execute queries against Immich API
		var items []common.FilterBuilderAssetItem
		totalCount := 0

		type userQuery struct {
			user   string
			filter immich.SearchFilter
		}
		var queriesToRun []userQuery

		if selectedUser != "" && selectedUser != "all" {
			// Query specific selected user
			if f, ok := userFilters.ForUser(selectedUser); ok {
				queriesToRun = append(queriesToRun, userQuery{user: selectedUser, filter: f})
			} else {
				// Fall back to first available filter
				for _, f := range userFilters {
					queriesToRun = append(queriesToRun, userQuery{user: selectedUser, filter: f})
					break
				}
			}
		} else {
			// Multi-user or global
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
				// Global filter across all known users
				for _, u := range knownUsers {
					if f, ok := userFilters.ForUser(u); ok {
						queriesToRun = append(queriesToRun, userQuery{user: u, filter: f})
					}
				}
			} else {
				// Single default user
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
