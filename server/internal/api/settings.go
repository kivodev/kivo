package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/config"
)

func (h *handlers) GetSettings(context.Context, gen.GetSettingsRequestObject) (gen.GetSettingsResponseObject, error) {
	return gen.GetSettings200JSONResponse(settings(h.deps.Settings.Get())), nil
}

func (h *handlers) UpdateSettings(ctx context.Context, req gen.UpdateSettingsRequestObject) (gen.UpdateSettingsResponseObject, error) {
	b := req.Body
	err := h.deps.Settings.Update(ctx, func(st *config.Settings) error {
		if b.ServerName != nil {
			st.ServerName = *b.ServerName
		}
		if b.MetadataLanguage != nil {
			st.MetadataLanguage = *b.MetadataLanguage
		}
		if b.ContentRatingCountry != nil {
			st.ContentRatingCountry = *b.ContentRatingCountry
		}
		set(&st.PlayedThreshold, b.PlayedThreshold)
		set(&st.OpenSubtitlesKey, b.OpenSubtitlesKey)
		set(&st.OpenSubtitlesUsername, b.OpenSubtitlesUsername)
		set(&st.OpenSubtitlesPassword, b.OpenSubtitlesPassword)
		set(&st.FileWatching, b.FileWatching)
		if b.LocalNetworks != nil {
			st.LocalNetworks = *b.LocalNetworks
		}
		set(&st.Accelerator, b.Accelerator)
		set(&st.VideoTranscodeLimit, b.VideoTranscodeLimit)
		set(&st.MaintenanceStart, b.MaintenanceStartHour)
		set(&st.MaintenanceEnd, b.MaintenanceEndHour)
		set(&st.PreviewsAtOnce, b.PreviewsAtOnce)
		set(&st.MarkersAtOnce, b.MarkersAtOnce)
		set(&st.PublicURL, b.PublicURL)
		set(&st.RemoteQualityCap, b.RemoteQualityCapBps)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return gen.UpdateSettings200JSONResponse(settings(h.deps.Settings.Get())), nil
}

func settings(st config.Settings) gen.Settings {
	return gen.Settings{
		ServerName: st.ServerName, MetadataLanguage: st.MetadataLanguage, ContentRatingCountry: st.ContentRatingCountry,
		PlayedThreshold: st.PlayedThreshold, OpenSubtitlesKey: st.OpenSubtitlesKey,
		OpenSubtitlesUsername: st.OpenSubtitlesUsername, OpenSubtitlesPasswordSet: st.OpenSubtitlesPassword != "",
		FileWatching: st.FileWatching, LocalNetworks: nonNil(st.LocalNetworks),
		Accelerator: st.Accelerator, VideoTranscodeLimit: st.VideoTranscodeLimit,
		MaintenanceStartHour: st.MaintenanceStart, MaintenanceEndHour: st.MaintenanceEnd, PreviewsAtOnce: st.PreviewsAtOnce, MarkersAtOnce: st.MarkersAtOnce,
		PublicURL: st.PublicURL, RemoteQualityCapBps: st.RemoteQualityCap,
	}
}

// set applies a field with merge semantics. The field changes only when
// it is present.
func set[T any](dst *T, v *T) {
	if v != nil {
		*dst = *v
	}
}
