package metadata

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// ContentRating is a content rating's name and the minimum age it implies.
// A user's ceiling is compared against that age.
type ContentRating struct {
	Name string
	Age  int
}

// contentRatings lists each country's content ratings in the order the
// content rating filter and the edit dialog show them. Film ratings come
// first, youngest first, followed by TV ratings. Most countries missing here
// already rate by age ("12", "FSK 16", "MA15+"), which contentRating reads
// directly.
var contentRatings = map[string][]ContentRating{
	"US": {{"G", 0}, {"PG", 10}, {"PG-13", 13}, {"R", 17}, {"NC-17", 18},
		{"TV-Y", 0}, {"TV-Y7", 7}, {"TV-G", 0}, {"TV-PG", 10}, {"TV-14", 14}, {"TV-MA", 17}},
	"GB": {{"U", 0}, {"UC", 0}, {"PG", 8}, {"12A", 12}, {"12", 12}, {"15", 15}, {"18", 18}, {"R18", 18}},
	"AU": {{"E", 0}, {"G", 0}, {"PG", 8}, {"M", 15}, {"MA15+", 15}, {"R18+", 18}, {"X18+", 18}},
	"CA": {{"G", 0}, {"PG", 8}, {"14A", 14}, {"18A", 18}, {"R", 18}, {"A", 18},
		{"C", 0}, {"C8", 8}, {"14+", 14}, {"18+", 18}},
	"DE": {{"0", 0}, {"6", 6}, {"12", 12}, {"16", 16}, {"18", 18}},
	"FR": {{"U", 0}, {"10", 10}, {"12", 12}, {"16", 16}, {"18", 18}},
	"NL": {{"AL", 0}, {"6", 6}, {"9", 9}, {"12", 12}, {"14", 14}, {"16", 16}, {"18", 18}},
	"BR": {{"L", 0}, {"10", 10}, {"12", 12}, {"14", 14}, {"16", 16}, {"18", 18}},
}

// ContentRatings lists the content ratings of a content rating country, in
// the order the content rating filter and the edit dialog show them. country
// is a library's country; empty means the server's. It returns an empty list
// for a country Kivo has no list for.
func (s *Service) ContentRatings(country string) []ContentRating {
	return slices.Clone(contentRatings[cmp.Or(country, s.deps.Settings.Get().ContentRatingCountry)])
}

// WorkContentRatings returns ContentRatings for the content rating country
// of a work. The work must be one the Viewer can see.
func (s *Service) WorkContentRatings(ctx context.Context, v users.Viewer, work id.ID) ([]ContentRating, error) {
	if err := s.deps.Catalog.Visible(ctx, v, work); err != nil {
		return nil, err
	}
	_, country, err := s.locale(ctx, work)
	if err != nil {
		return nil, err
	}
	return s.ContentRatings(country), nil
}

// contentRating reads a content rating from country. It returns the
// rating's name and the age it implies. It returns "" and nil for a rating
// that means nothing to us, such as empty, "NR" or "Unrated". A name in the
// country's list is spelled the way the list spells it. NFO files often say
// "Rated PG-13" or "US:PG-13". A rating prefixed with another country is
// ignored.
func contentRating(country, rating string) (string, *int) {
	rating = strings.TrimSpace(rating)
	if c, rest, ok := strings.Cut(rating, ":"); ok && len(c) == 2 {
		if !strings.EqualFold(c, country) {
			return "", nil
		}
		rating = strings.TrimSpace(rest)
	}
	rating = strings.TrimSpace(strings.TrimPrefix(rating, "Rated "))
	for _, r := range contentRatings[country] {
		if strings.EqualFold(r.Name, rating) {
			return r.Name, &r.Age
		}
	}
	digits := strings.TrimFunc(rating, func(r rune) bool { return !unicode.IsDigit(r) })
	if i := strings.IndexFunc(digits, func(r rune) bool { return !unicode.IsDigit(r) }); i >= 0 {
		digits = digits[:i]
	}
	if age, err := strconv.Atoi(digits); err == nil && age <= 21 {
		return rating, &age
	}
	return "", nil
}
