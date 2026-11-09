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
		return nil, invalid("invalid query", problemFields("/query", bad.Problems)...)
	}
	if err != nil {
		return nil, err
	}
	return gen.QueryWorks200JSONResponse(workPage(page)), nil
}

// problemFields is a query's problems as field errors under prefix.
func problemFields(prefix string, problems []query.QueryProblem) []gen.FieldError {
	out := make([]gen.FieldError, len(problems))
	for i, p := range problems {
		out[i] = field(prefix+p.Pointer, p.Code)
	}
	return out
}

// wireQuery converts a query for the wire.
func wireQuery(q query.WorkQuery) gen.WorkQuery {
	out := gen.WorkQuery{Version: &q.Version, Kinds: each(q.Kinds, func(k catalog.WorkKind) gen.WorkKind { return gen.WorkKind(k) }), MaxWorks: q.MaxWorks}
	if q.Collapse != "" {
		out.Collapse = ptr(gen.WorkQueryCollapse(q.Collapse))
	}
	if q.Where != nil {
		g := wireGroup(*q.Where)
		out.Where = &g
	}
	if len(q.Sort) > 0 {
		sort := each(q.Sort, func(k query.SortKey) gen.SortKey {
			key := gen.SortKey{Field: k.Field}
			if k.Direction != "" {
				key.Direction = ptr(gen.SortDirection(k.Direction))
			}
			return key
		})
		out.Sort = &sort
	}
	return out
}

func wireGroup(g query.RuleGroup) gen.RuleGroup {
	out := gen.RuleGroup{Match: gen.RuleGroupMatch(g.Match)}
	if len(g.Rules) > 0 {
		rules := each(g.Rules, func(r query.Rule) gen.Rule {
			rule := gen.Rule{Field: r.Field, Op: r.Op}
			if len(r.Values) > 0 {
				rule.Values = &r.Values
			}
			return rule
		})
		out.Rules = &rules
	}
	if len(g.Groups) > 0 {
		groups := each(g.Groups, wireGroup)
		out.Groups = &groups
	}
	return out
}

// workQuery converts a query from the wire.
func workQuery(q gen.WorkQuery) query.WorkQuery {
	out := query.WorkQuery{Version: deref(q.Version), MaxWorks: q.MaxWorks}
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
		out.DefaultDirection = ptr(gen.SortDirection(f.DefaultDirection))
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
