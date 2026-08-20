package ai

import (
	"bytes"
	"encoding/json"
)

type optionalState uint8

const (
	optionalAbsent optionalState = iota
	optionalNull
	optionalValue
)

// Optional preserves the three states of an optional JSON property: absent,
// explicitly null, and present with a value. Its zero value is absent.
//
// Use the omitzero JSON tag when an absent Optional is a struct field.
type Optional[T any] struct {
	value T
	state optionalState
}

// Absent returns an optional property that was not provided.
func Absent[T any]() Optional[T] {
	return Optional[T]{}
}

// Null returns an optional property that was explicitly set to JSON null.
func Null[T any]() Optional[T] {
	return Optional[T]{state: optionalNull}
}

// Some returns an optional property containing value, including its zero value.
func Some[T any](value T) Optional[T] {
	return Optional[T]{value: value, state: optionalValue}
}

// IsSet reports whether the property is either explicitly null or contains a
// value.
func (o Optional[T]) IsSet() bool {
	return o.state != optionalAbsent
}

// IsNull reports whether the property was explicitly set to JSON null.
func (o Optional[T]) IsNull() bool {
	return o.state == optionalNull
}

// Value returns the contained value. ok is false for absent and null values.
func (o Optional[T]) Value() (value T, ok bool) {
	if o.state != optionalValue {
		return value, false
	}
	return o.value, true
}

// IsZero lets encoding/json's omitzero option omit an absent property while
// retaining explicit null and explicit zero values.
func (o Optional[T]) IsZero() bool {
	return o.state == optionalAbsent
}

// MarshalJSON preserves null and explicit values. An absent Optional marshaled
// outside an omitzero field is represented as null because JSON has no absent
// top-level value.
func (o Optional[T]) MarshalJSON() ([]byte, error) {
	if o.state != optionalValue {
		return []byte("null"), nil
	}
	return json.Marshal(o.value)
}

// UnmarshalJSON records null distinctly from a present zero value. A missing
// struct property does not invoke this method, leaving the zero value absent.
func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		var zero T
		o.value = zero
		o.state = optionalNull
		return nil
	}

	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.value = value
	o.state = optionalValue
	return nil
}
