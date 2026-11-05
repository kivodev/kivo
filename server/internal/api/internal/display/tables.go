// Package display puts tracks and versions in words for the wire, from the
// display tables the clients share (shared/display), in English.
package display

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"
)

// The tables are copies of shared/display's, which `make generate` refreshes.
//go:generate cp ../../../../../shared/display/en.json ../../../../../shared/display/media.json .
//go:generate cp ../../../../../shared/display/languages/en.json languages.json
//go:generate cp ../../../../../shared/display/languages/aliases.json aliases.json

var (
	//go:embed en.json
	localeJSON []byte
	//go:embed media.json
	mediaJSON []byte
	//go:embed languages.json
	languagesJSON []byte
	//go:embed aliases.json
	aliasesJSON []byte
)

// words are the locale table's words for tracks.
type words struct {
	UnknownLanguage string `json:"unknownLanguage"`
	Default         string `json:"default"`
	Flags           struct {
		Forced          string `json:"forced"`
		HearingImpaired string `json:"hearingImpaired"`
		Commentary      string `json:"commentary"`
		Original        string `json:"original"`
	} `json:"flags"`
	Channels       string            `json:"channels"`
	Sources        map[string]string `json:"sources"`
	DownloadedFrom string            `json:"downloadedFrom"`
	AudioOnly      string            `json:"audioOnly"`
}

// mediaNames are the names of codecs and formats, the same in every locale.
type mediaNames struct {
	Codecs       map[string]string `json:"codecs"`
	Channels     map[string]string `json:"channels"`
	Layouts      map[string]string `json:"layouts"`
	Resolutions  map[string]string `json:"resolutions"`
	HDR          map[string]string `json:"hdr"`
	VersionHDR   map[string]string `json:"versionHdr"`
	PremiumAudio map[string]string `json:"premiumAudio"`
}

var (
	tracks = decode[struct {
		Tracks words `json:"tracks"`
	}](localeJSON).Tracks
	media     = decode[mediaNames](mediaJSON)
	languages = decode[languageNames](languagesJSON)
	aliases   = decode[map[string]string](aliasesJSON)
)

func decode[T any](b []byte) T {
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		panic("display: " + err.Error())
	}
	return v
}

// lookup is m's value for k, or fallback when it has none.
func lookup(m map[string]string, k, fallback string) string {
	if v, ok := m[k]; ok {
		return v
	}
	return fallback
}

// codec is a codec as people know it: "TrueHD", "SRT".
func codec(c string) string { return lookup(media.Codecs, c, strings.ToUpper(c)) }

// channels is "Stereo", "5.1", "7.1" or the count in words.
func channels(count int) string {
	n := strconv.Itoa(count)
	return lookup(media.Channels, n, strings.ReplaceAll(tracks.Channels, "{channels}", n))
}

// layout is a channel layout as the wire gives it ("2.0", "5.1"; "1.0" is
// "Mono"), or without one the count in words.
func layout(l *string, count int) string {
	if l == nil {
		return channels(count)
	}
	return lookup(media.Layouts, *l, *l)
}

// resolution is a resolution class in words: `4k` is "4K", `sd` "SD".
func resolution(r string) string { return lookup(media.Resolutions, r, r) }

// hdr is an HDR format: "HDR10", "HLG", "Dolby Vision".
func hdr(h string) string { return lookup(media.HDR, h, h) }

// versionHDR is an HDR format as a version's name says it: "HDR", "Dolby Vision".
func versionHDR(h string) string { return lookup(media.VersionHDR, h, hdr(h)) }
