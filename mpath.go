package main

import (
	"errors"
	"reflect"
	"strings"
)

//getValueAtPath returns the value that is obtained as the map is traversed
//following the dot-delimited path
//
//Asumption: all keys traversed at of string type
//for a json map {"en": {"us": "greetings": ["hello", "hi"]} }"
//and a path "en.us.greetings" would return the list ["hello", "hi"]
//TODO: should we make export this?
//Note: require client to pass
func getValueAtPath(m map[string]interface{}, path string) interface{} {
	var v interface{}
	for _, k := range strings.Split(path, ".") { //each key
		var ok bool
		if v, ok = m[k]; !ok {
			//we could no longer follow key in running map
			return nil
		}
		if tm, ok := v.(map[string]interface{}); ok {
			m = tm
		}
	}
	return v
}

//Intersect returns true iff the slice at pa in A has some member
//a that is also a member of slice at pb in B
//Note: Interface values are comparable. Two interface values are equal if
//they have identical dynamic types and equal dynamic values or if both have value nil.
//note: prefer deep equal and use configurable input
func Intersect(A, B map[string]interface{}, pa, pb string) (bool, error) {
	valueAtA := getValueAtPath(A, pa)
	valueAtB := getValueAtPath(B, pb)

	if valueAtA == nil || valueAtB == nil {
		return false, nil

	}

	if reflect.TypeOf(valueAtA).Kind() != reflect.Slice ||
		reflect.TypeOf(valueAtB).Kind() != reflect.Slice {
		return false, errors.New("Only slices are supported as values")
	}

	m := make(map[interface{}]struct{})

	a := reflect.ValueOf(valueAtA)
	for i := 0; i < a.Len(); i++ {
		m[a.Index(i).Interface()] = struct{}{}
	}

	b := reflect.ValueOf(valueAtB)
	for i := 0; i < b.Len(); i++ {
		if _, ok := m[b.Index(i).Interface()]; ok {
			return true, nil
		}
	}

	return false, nil

}
