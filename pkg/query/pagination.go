package query

// Pagination is the legacy page and size query input retained during migration.
// Page defaults to 1 and Size to 20 when values are missing or integer conversion fails.
//
// Deprecated: Use input.Query[T] with a DTO that declares query tags for page and size.
// Apply defaults for missing values in the handler and declare constraints in the selected validator.
type Pagination struct {
	Page int
	Size int
}
