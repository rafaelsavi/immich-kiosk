package immich

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseUserFilters_SingleFilter(t *testing.T) {
	raw := `{"isFavorite": true, "country": "Japan", "rating": 4}`

	filters, err := ParseUserFilters(raw, nil)
	require.NoError(t, err)
	require.NotNil(t, filters)

	filter, found := filters.ForUser("rafael")
	assert.True(t, found)
	require.NotNil(t, filter.IsFavorite.Eq)
	assert.True(t, *filter.IsFavorite.Eq)
	assert.Equal(t, "Japan", filter.Country.Eq)
	assert.Equal(t, float64(4), filter.Rating.Eq)
}

func TestParseUserFilters_WithOrBranch(t *testing.T) {
	raw := `{
		"isFavorite": true,
		"or": [
			{"country": "Japan"},
			{"country": "France"}
		]
	}`

	filters, err := ParseUserFilters(raw, nil)
	require.NoError(t, err)
	require.NotNil(t, filters)

	filter, found := filters.ForUser("")
	assert.True(t, found)
	require.NotNil(t, filter.IsFavorite.Eq)
	assert.True(t, *filter.IsFavorite.Eq)
	require.NotNil(t, filter.Or)
	assert.Len(t, *filter.Or, 2)
	assert.Equal(t, "Japan", (*filter.Or)[0].Country.Eq)
	assert.Equal(t, "France", (*filter.Or)[1].Country.Eq)
}

func TestParseUserFilters_MultiUser(t *testing.T) {
	raw := `{
		"rafael": {
			"country": "Spain",
			"isFavorite": true
		},
		"maria": {
			"country": "Italy",
			"rating": 5
		}
	}`

	filters, err := ParseUserFilters(raw, []string{"rafael", "maria"})
	require.NoError(t, err)
	require.NotNil(t, filters)

	f1, found1 := filters.ForUser("rafael")
	assert.True(t, found1)
	assert.Equal(t, "Spain", f1.Country.Eq)
	require.NotNil(t, f1.IsFavorite.Eq)
	assert.True(t, *f1.IsFavorite.Eq)

	f2, found2 := filters.ForUser("maria")
	assert.True(t, found2)
	assert.Equal(t, "Italy", f2.Country.Eq)
	assert.Equal(t, float64(5), f2.Rating.Eq)

	_, foundOther := filters.ForUser("unknown")
	assert.False(t, foundOther)
}

func TestParseUserFilters_ObjectNotation(t *testing.T) {
	raw := `{
		"country": {"in": ["Spain", "France"]},
		"personIds": {"any": ["uuid-1", "uuid-2"]}
	}`

	filters, err := ParseUserFilters(raw, nil)
	require.NoError(t, err)
	require.NotNil(t, filters)

	filter, found := filters.ForUser("user1")
	assert.True(t, found)
	assert.Equal(t, []string{"Spain", "France"}, filter.Country.In)
	assert.Equal(t, []string{"uuid-1", "uuid-2"}, filter.PersonIDs.Any)
}

func TestParseUserFilters_InvalidJSON(t *testing.T) {
	raw := `{"isFavorite": invalid}`

	_, err := ParseUserFilters(raw, nil)
	assert.Error(t, err)
}

func TestParseUserFilters_DateFilterCanonical(t *testing.T) {
	raw := `{
		"takenAt": {
			"gte": "2024-01-01T00:00:00Z",
			"lte": "2024-12-31T23:59:59Z"
		}
	}`

	filters, err := ParseUserFilters(raw, []string{"rafael", "maria"})
	require.NoError(t, err)
	require.NotNil(t, filters)

	filter, found := filters.ForUser("")
	assert.True(t, found)
	assert.False(t, filter.TakenAt.Gte.IsZero())
	assert.False(t, filter.TakenAt.Lte.IsZero())
	assert.Equal(t, 2024, filter.TakenAt.Gte.Year())
	assert.Equal(t, 2024, filter.TakenAt.Lte.Year())
}

func TestParseUserFilters_TakenAfterBefore(t *testing.T) {
	raw := `{
		"takenAfter": "2024-01-01T00:00:00Z",
		"takenBefore": "2024-12-31T23:59:59Z"
	}`

	filters, err := ParseUserFilters(raw, []string{"rafael", "maria"})
	require.NoError(t, err)
	require.NotNil(t, filters)

	filter, found := filters.ForUser("")
	assert.True(t, found)
	assert.False(t, filter.TakenAt.Gte.IsZero())
	assert.False(t, filter.TakenAt.Lte.IsZero())
	assert.Equal(t, 2024, filter.TakenAt.Gte.Year())
	assert.Equal(t, 2024, filter.TakenAt.Lte.Year())
}

