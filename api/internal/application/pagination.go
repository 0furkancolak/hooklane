package application

const (
	defaultPageSize = 50
	maxPageSize     = 100
)

// PageRequest is a client's requested window into a list. Out-of-range values
// are clamped rather than rejected.
type PageRequest struct {
	Limit  int32
	Offset int32
}

// Page is one window of a list. NextOffset is nil on the last page.
type Page[T any] struct {
	Items      []T
	Limit      int32
	Offset     int32
	NextOffset *int32
}

func clampPage(limit, offset int32) (int32, int32) {
	if limit <= 0 || limit > maxPageSize {
		limit = defaultPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// paginate runs fetch with one extra row so it can tell whether another page
// follows without a separate count query.
func paginate[T any](req PageRequest, fetch func(limit, offset int32) ([]T, error)) (Page[T], error) {
	limit, offset := clampPage(req.Limit, req.Offset)
	items, err := fetch(limit+1, offset)
	if err != nil {
		return Page[T]{}, err
	}
	page := Page[T]{Items: items, Limit: limit, Offset: offset}
	if int32(len(items)) > limit {
		page.Items = items[:limit]
		next := offset + limit
		page.NextOffset = &next
	}
	if page.Items == nil {
		page.Items = []T{}
	}
	return page, nil
}
