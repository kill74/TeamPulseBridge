// Package storeutil contains small helpers shared by store wrappers.
package storeutil

import "reflect"

// IsNil reports whether v is a nil interface or a typed nil value, such as a
// nil *PostgresStore assigned to a Store interface after a failed constructor.
// A plain `v == nil` check misses the latter and leads to nil dereferences in
// wrapper methods.
func IsNil(v any) bool {
	if v == nil {
		return true
	}
	value := reflect.ValueOf(v)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
