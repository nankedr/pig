package protocol

// Optional represents an optional wire property without conflating absence
// with the property's zero value.
type Optional[T any] struct {
	Value   T
	Present bool
}

// Some returns a present optional property.
func Some[T any](value T) Optional[T] {
	return Optional[T]{Value: value, Present: true}
}

// None returns an absent optional property.
func None[T any]() Optional[T] {
	return Optional[T]{}
}
