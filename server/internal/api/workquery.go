package api

import (
	"context"
	"errors"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/query"
)

func (h *handlers) QueryWorks(ctx context.Context, req gen.QueryWorksRequestObject) (gen.QueryWorksResponseObject, error) {
	o, err := gridOptionsOf(req.Body.Offset, req.Body.Limit)
	if err != nil {
		return nil, err
	}
	page, err := h.deps.Query.QueryWorks(ctx, callerOf(ctx).viewer, workQuery(req.Body.Query), o)
	var bad *query.InvalidQueryError
	if errors.As(err, &bad) {
		fields := make([]gen.FieldError, len(bad.Problems))
		for i, p := range bad.Problems {
			fields[i] = field("/query"+p.Pointer, p.Code)
		}
		return nil, invalid("invalid query", fields...)
	}
	if err != nil {
		return nil, err
	}
	return gen.QueryWorks200JSONResponse(workPage(page)), nil
}

// workQuery converts a query from the wire.
func workQuery(q gen.WorkQuery) query.WorkQuery {
	out := query.WorkQuery{Version: deref(q.Version), MaxWorks: deref(q.MaxWorks)}
	if q.Collapse != nil {
		out.Collapse = string(*q.Collapse)
	}
	for _, k := range q.Kinds {
		out.Kinds = append(out.Kinds, catalog.WorkKind(k))
	}
	if q.Where != nil {
		g := ruleGroup(*q.Where)
		out.Where = &g
	}
	for _, k := range deref(q.Sort) {
		out.Sort = append(out.Sort, query.SortKey{Field: k.Field, Direction: string(deref(k.Direction))})
	}
	return out
}

func ruleGroup(g gen.RuleGroup) query.RuleGroup {
	out := query.RuleGroup{Match: string(g.Match)}
	for _, r := range deref(g.Rules) {
		out.Rules = append(out.Rules, query.Rule{Field: r.Field, Op: r.Op, Values: deref(r.Values)})
	}
	for _, sub := range deref(g.Groups) {
		out.Groups = append(out.Groups, ruleGroup(sub))
	}
	return out
}

func (h *handlers) ListWorkFields(_ context.Context, _ gen.ListWorkFieldsRequestObject) (gen.ListWorkFieldsResponseObject, error) {
	return gen.ListWorkFields200JSONResponse{Items: each(query.WorkFields(), workField)}, nil
}

func workField(f query.WorkField) gen.WorkField {
	out := gen.WorkField{
		Field: f.Field, Name: f.Name, Type: gen.WorkFieldType(f.Type), Sortable: f.Sortable,
		Kinds: each(f.Kinds, func(k catalog.WorkKind) gen.WorkKind { return gen.WorkKind(k) }),
		Ops:   each(f.Ops, func(o query.FieldOp) gen.WorkFieldOp { return gen.WorkFieldOp{Op: o.Op, Arity: o.Arity} }),
		Unit:  opt(f.Unit),
	}
	if f.DefaultDirection != "" {
		out.DefaultDirection = ptr(gen.WorkFieldDefaultDirection(f.DefaultDirection))
	}
	if f.ValueSource != "" {
		out.ValueSource = ptr(gen.WorkFieldValueSource(f.ValueSource))
	}
	return out
}

func (h *handlers) ListWorkFieldValues(ctx context.Context, req gen.ListWorkFieldValuesRequestObject) (gen.ListWorkFieldValuesResponseObject, error) {
	values, err := h.deps.Query.FieldValues(ctx, callerOf(ctx).viewer, req.Field, deref(req.Params.Q), deref(req.Params.Ids))
	if err != nil {
		return nil, err
	}
	return gen.ListWorkFieldValues200JSONResponse{Items: each(values, func(v query.FieldValue) gen.WorkFieldValue {
		return gen.WorkFieldValue{Value: v.Value, Name: v.Name}
	})}, nil
}
