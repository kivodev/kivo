package api

import (
	"context"
	"encoding/base64"
	"net/netip"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/playback"
)

// GetServer returns the server's info. Anyone may see the basics. A
// signed-in client also gets what it needs to reach the server and play,
// and an admin gets the newer release, if one is out.
func (h *handlers) GetServer(ctx context.Context, req gen.GetServerRequestObject) (gen.GetServerResponseObject, error) {
	out := h.serverInfo()
	proof, err := h.proof(ctx, req.Params)
	if err != nil {
		return nil, err
	}
	out.Proof = proof
	if callerOf(ctx).User.IsZero() {
		return gen.GetServer200JSONResponse(out), nil
	}
	local, public := h.addresses(requestOf(ctx).ip)
	out.LocalUrls = &local
	if public != "" {
		out.PublicURL = &public
	}
	ladder := []gen.QualityRung{}
	for _, r := range playback.Ladder() {
		ladder = append(ladder, gen.QualityRung{BitrateBps: int64(r.Bitrate), Height: r.Height})
	}
	out.QualityLadder = &ladder
	if r, ok := h.deps.Release.Newer(); ok {
		v, err := h.viewerOf(ctx, callerOf(ctx).Caller)
		if err != nil {
			return nil, err
		}
		if v.Admin() {
			out.NewRelease = &gen.NewRelease{Version: r.Version, URL: r.URL}
		}
	}
	return gen.GetServer200JSONResponse(out), nil
}

// proof is the server's answer to a probe's challenge, for the app whose
// session has the probe's ID. It is nil if the probe lacks an ID or a
// challenge, or if no session has the ID.
func (h *handlers) proof(ctx context.Context, p gen.GetServerParams) (*gen.ServerProof, error) {
	if p.Challenge == nil && p.ProbeID == nil {
		return nil, nil
	}
	challenge, err := base64.RawURLEncoding.DecodeString(deref(p.Challenge))
	if err != nil || len(challenge) < 16 || len(challenge) > 64 {
		return nil, invalid("challenge: 16 to 64 bytes, base64url without padding", field("challenge", "invalid"))
	}
	probe, err := base64.RawURLEncoding.DecodeString(deref(p.ProbeID))
	if err != nil || len(probe) != auth.ProbeIDLen {
		return nil, invalid("probeId: 16 bytes, base64url without padding", field("probeId", "invalid"))
	}
	req := requestOf(ctx)
	if len(req.host) > maxProbeHost {
		return nil, nil
	}
	claim := auth.ProbeClaim{Challenge: challenge, ServerID: h.deps.Settings.ServerID().String(), Host: req.host, Local: h.deps.Settings.Get().IsLocal(req.ip)}
	mac, ok, err := h.deps.Auth.Prove(ctx, probe, claim)
	if err != nil || !ok {
		return nil, err
	}
	return &gen.ServerProof{Mac: base64.RawURLEncoding.EncodeToString(mac), Host: claim.Host, Local: claim.Local}, nil
}

// maxProbeHost is the longest host a proof names, which is a DNS name and
// a port.
const maxProbeHost = 255

// castProtocol is the version of the contract between Cast senders, the
// Cast receiver and the server, which ServerInfo states.
const castProtocol = 3

// serverInfo is the server as anyone may see it, even signed out. It
// gives the server's ID, name, version and cast protocol.
func (s *Server) serverInfo() gen.ServerInfo {
	return gen.ServerInfo{ID: s.deps.Settings.ServerID(), Name: s.deps.Settings.Get().ServerName, Version: s.deps.Version, CastProtocol: ptr(castProtocol)}
}

// addresses lists the server's URLs for a client at ip. The local URLs
// come first, with the client's own network ahead of the others. The
// public URL comes last, if one is set.
func (s *Server) addresses(ip netip.Addr) (local []string, public string) {
	return s.deps.Discovery.LocalURLs(ip), s.deps.Settings.Get().PublicURL
}

func (h *handlers) GetHealth(context.Context, gen.GetHealthRequestObject) (gen.GetHealthResponseObject, error) {
	if h.stopping.Load() {
		return gen.GetHealth503JSONResponse{Status: "stopping"}, nil
	}
	return gen.GetHealth200JSONResponse{Status: "ok"}, nil
}
