package api

import (
	"bytes"
	"context"
	"net/url"

	"rsc.io/qr"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/auth"
)

// CreateDeviceCode starts a code for a device. The device polls for a
// session while a signed-in user looks the code up and approves or denies
// it.
func (h *handlers) CreateDeviceCode(ctx context.Context, req gen.CreateDeviceCodeRequestObject) (gen.CreateDeviceCodeResponseObject, error) {
	b, r := req.Body, requestOf(ctx)
	start := auth.DeviceCodeStart{
		Name: b.Name, Kind: auth.SessionKind(deref(b.Kind)),
		ClientID: deref(b.ClientID), Device: r.device, RemoteIP: r.ip,
	}
	if start.Kind == auth.SessionBrowser { // a browser's client ID is its client cookie
		start.ClientID = browserClientID(ctx)
	}
	dl, err := h.deps.Auth.StartDeviceCode(ctx, start)
	if err != nil {
		return nil, err
	}
	return gen.CreateDeviceCode201JSONResponse{
		Code: dl.Code, Secret: dl.Secret, ExpiresAt: dl.Expires.UTC(), Interval: int(dl.Interval.Seconds()), LinkURL: h.linkURL(r, dl.Code),
	}, nil
}

// linkURL is the web client's /link page for code, at the address the
// request reached the server by, with the server's ID.
func (h *handlers) linkURL(r *request, code string) string {
	scheme := "http"
	if r.secure {
		scheme = "https"
	}
	query := url.Values{"code": {code}, "server": {h.deps.Settings.ServerID().String()}}
	link := url.URL{Scheme: scheme, Host: r.host, Path: h.deps.BasePath + "/link", RawQuery: query.Encode()}
	return link.String()
}

// qrScale is a QR module's size in pixels: about 450 pixels across for a
// link, which a phone reads from across a room on a TV.
const qrScale = 10

// GetDeviceCodeQr draws a waiting code's link as a QR code, for a device
// that can show an image but can't draw one. It encodes only the link
// createDeviceCode gives, never the caller's text.
func (h *handlers) GetDeviceCodeQr(ctx context.Context, req gen.GetDeviceCodeQrRequestObject) (gen.GetDeviceCodeQrResponseObject, error) {
	r := requestOf(ctx)
	if err := h.deps.Auth.DeviceCodeWaiting(req.Code, r.ip); err != nil {
		return nil, err
	}
	code, err := qr.Encode(h.linkURL(r, req.Code), qr.M)
	if err != nil {
		return nil, err
	}
	code.Scale = qrScale
	png := code.PNG()
	return gen.GetDeviceCodeQr200ImagePngResponse{
		Body: bytes.NewReader(png), ContentLength: int64(len(png)),
		Headers: gen.GetDeviceCodeQr200ResponseHeaders{CacheControl: ptr("no-store")},
	}, nil
}

func (h *handlers) PollDeviceCode(ctx context.Context, req gen.PollDeviceCodeRequestObject) (gen.PollDeviceCodeResponseObject, error) {
	issued, ok, err := h.deps.Auth.PollDeviceCode(ctx, req.Body.Secret, requestOf(ctx).ip)
	if err != nil {
		return nil, err
	}
	if !ok {
		return gen.PollDeviceCode202Response{}, nil
	}
	return h.issued(ctx, issued), nil
}

func (h *handlers) GetDeviceCode(ctx context.Context, req gen.GetDeviceCodeRequestObject) (gen.GetDeviceCodeResponseObject, error) {
	pending, err := h.deps.Auth.LookUpDeviceCode(ctx, callerOf(ctx).Caller, req.Code, requestOf(ctx).ip)
	if err != nil {
		return nil, err
	}
	out := gen.GetDeviceCode200JSONResponse{Name: pending.Name, Nearby: pending.Nearby, StartedAt: pending.Started.UTC()}
	if pending.Platform != "" {
		out.Platform = ptr(gen.ClientPlatform(pending.Platform))
	}
	return out, nil
}

func (h *handlers) ApproveDeviceCode(ctx context.Context, req gen.ApproveDeviceCodeRequestObject) (gen.ApproveDeviceCodeResponseObject, error) {
	if err := h.deps.Auth.ApproveDeviceCode(ctx, callerOf(ctx).Caller, req.Code, requestOf(ctx).ip); err != nil {
		return nil, err
	}
	return gen.ApproveDeviceCode204Response{}, nil
}

func (h *handlers) DenyDeviceCode(ctx context.Context, req gen.DenyDeviceCodeRequestObject) (gen.DenyDeviceCodeResponseObject, error) {
	if err := h.deps.Auth.DenyDeviceCode(ctx, callerOf(ctx).Caller, req.Code, requestOf(ctx).ip); err != nil {
		return nil, err
	}
	return gen.DenyDeviceCode204Response{}, nil
}
