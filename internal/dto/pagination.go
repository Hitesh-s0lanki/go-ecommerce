package dto

// Pagination bounds for list endpoints.
const (
	defaultPage  = 1
	defaultLimit = 20
	// MaxLimit caps a page. Without a ceiling, `?limit=1000000` is a
	// one-request denial of service: the reference clamps nothing, so a
	// single caller can ask the database for the whole table.
	MaxLimit = 100
)

// ListQuery is the pagination query string shared by list endpoints.
//
// Bound with ShouldBindQuery rather than parsed with strconv.Atoi: a discarded
// parse error turns `?limit=abc` into limit 0, which then silently becomes the
// default. A caller who sent nonsense should hear about it.
//
// The defaults live in the form tags rather than in an omitempty rule, so an
// absent limit and an explicit `?limit=0` stay distinguishable: the first gets
// the default, the second is the nonsense it looks like and earns a 400. The
// literals must match the constants above — gin's tag cannot reference them.
type ListQuery struct {
	Page  int `form:"page,default=1"   binding:"min=1"`
	Limit int `form:"limit,default=20" binding:"min=1,max=100"`
}

// Normalize fills in the defaults for omitted values and enforces MaxLimit.
//
// The binding tags already reject an out-of-range limit at the HTTP edge; this
// repeats the ceiling so a non-HTTP caller cannot bypass it.
func (q *ListQuery) Normalize() {
	if q.Page < 1 {
		q.Page = defaultPage
	}

	if q.Limit < 1 {
		q.Limit = defaultLimit
	}

	if q.Limit > MaxLimit {
		q.Limit = MaxLimit
	}
}

// Offset is the SQL OFFSET for this page. Normalize must have run.
func (q ListQuery) Offset() int {
	return (q.Page - 1) * q.Limit
}

// PageMeta describes the page a list endpoint returned.
type PageMeta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// NewPageMeta computes the metadata for a page of total rows.
func NewPageMeta(q ListQuery, total int64) PageMeta {
	q.Normalize()

	// Ceiling division: 21 rows at 20 per page is 2 pages, not 1.
	totalPages := int((total + int64(q.Limit) - 1) / int64(q.Limit))

	return PageMeta{
		Page:       q.Page,
		Limit:      q.Limit,
		Total:      total,
		TotalPages: totalPages,
	}
}
