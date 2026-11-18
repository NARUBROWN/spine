package path

// Int is the legacy integer path wrapper retained during migration.
//
// Deprecated: Use input.Path[int64] or input.Path[T] with a DTO that declares path tags.
type Int struct {
	Value int64
}

// String is the legacy string path wrapper retained during migration.
//
// Deprecated: Use input.Path[string] or input.Path[T] with a DTO that declares path tags.
type String struct {
	Value string
}

// Boolean is the legacy boolean path wrapper retained during migration.
//
// Deprecated: Use input.Path[bool] or input.Path[T] with a DTO that declares path tags.
type Boolean struct {
	Value bool
}
