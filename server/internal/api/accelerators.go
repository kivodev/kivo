package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
)

// ListAccelerators lists the accelerators that passed, followed by those
// that can't be used, each with its problem.
func (h *handlers) ListAccelerators(context.Context, gen.ListAcceleratorsRequestObject) (gen.ListAcceleratorsResponseObject, error) {
	caps := h.deps.Accelerators.Capabilities()
	out := gen.AcceleratorList{Tested: caps.Tested, Items: []gen.Accelerator{}}
	for _, a := range caps.Accelerators {
		item := gen.Accelerator{
			ID: string(a.Accelerator), Backend: gen.AcceleratorBackend(a.Backend), Name: a.Name,
			Encoders: []gen.AcceleratorEncoder{}, Decoders: []gen.AcceleratorFormat{},
			ToneMap: optAs[gen.ProcessedOn](a.ToneMap), BurnIn: optAs[gen.ProcessedOn](a.BurnIn),
		}
		for _, e := range a.Encoders {
			item.Encoders = append(item.Encoders, gen.AcceleratorEncoder{Codec: gen.EncoderCodec(e.Codec)})
		}
		for _, f := range a.Decoders {
			item.Decoders = append(item.Decoders, gen.AcceleratorFormat{Codec: string(f.Codec), BitDepth: f.BitDepth})
		}
		out.Items = append(out.Items, item)
	}
	for _, p := range caps.Problems {
		out.Items = append(out.Items, gen.Accelerator{
			ID: string(p.Accelerator), Backend: gen.AcceleratorBackend(p.Backend), Name: p.Name,
			Encoders: []gen.AcceleratorEncoder{}, Decoders: []gen.AcceleratorFormat{},
			Problem: &gen.AcceleratorProblem{Reason: gen.AcceleratorProblemReason(p.Reason), Detail: opt(p.Detail)},
		})
	}
	return gen.ListAccelerators200JSONResponse(out), nil
}
