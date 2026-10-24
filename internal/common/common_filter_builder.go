// Package common provides shared types and utilities for the immich-kiosk application
package common

import (
	"github.com/damongolding/immich-kiosk/internal/immich"
)

// FilterBuilderAssetItem represents a miniature asset preview in the filter playground
type FilterBuilderAssetItem struct {
	ID           string
	ThumbnailURL string
	User         string
	Type         string
	City         string
	State        string
	Country      string
	Model        string
	TakenAt      string
	IsFavorite   bool
	Rating       int
	People       []string
	Filename     string
}

// FilterBuilderViewData holds metadata needed to render the filter playground view
type FilterBuilderViewData struct {
	ViewData      ViewData
	Users         []string
	SelectedUser  string
	People        []immich.Person
	Albums        immich.Albums
	Tags          immich.Tags
	Countries     []string
	States        []string
	Cities        []string
	Cameras       []string
	InitialFilter string
}

// FilterBuilderResultData holds the live test results rendered in the filter playground results card
type FilterBuilderResultData struct {
	Total        int
	TotalExact   bool
	Limit        int
	Assets       []FilterBuilderAssetItem
	FilterJSON   string
	KioskURL     string
	RedirectYAML string
	Error        string
}
