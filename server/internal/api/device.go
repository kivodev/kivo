package api

import (
	"net/http"

	"github.com/mmcdole/kivo/internal/auth"
)

// The headers an app identifies itself and its device by, one field each.
const (
	appHeader         = "Kivo-App"
	appVersionHeader  = "Kivo-App-Version"
	platformHeader    = "Kivo-Platform"
	osVersionHeader   = "Kivo-OS-Version"
	deviceModelHeader = "Kivo-Device-Model"
)

// deviceHeaders are the identity headers, lowercase, as a preflight lists
// them.
var deviceHeaders = []string{"kivo-app", "kivo-app-version", "kivo-platform", "kivo-os-version", "kivo-device-model"}

// maxDeviceValue is the most characters an identity header's value may have.
const maxDeviceValue = 100

// parseDevice reads the identity headers. A value that's too long, or
// isn't printable ASCII, is left out.
func parseDevice(h http.Header) auth.Device {
	value := func(name string) string {
		v := h.Get(name)
		if len(v) > maxDeviceValue {
			return ""
		}
		for i := range len(v) {
			if v[i] < ' ' || v[i] > '~' {
				return ""
			}
		}
		return v
	}
	return auth.Device{
		App: value(appHeader), AppVersion: value(appVersionHeader), Platform: auth.Platform(value(platformHeader)),
		OSVersion: value(osVersionHeader), Model: value(deviceModelHeader),
	}
}
