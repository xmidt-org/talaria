package main

type Equality func(interface{}, interface{}) bool

//getValueAtPath returns the value that is obtained as the map is traversed
//following the dot-delimited path
//
//Asumption: all keys traversed at of string type
//for a json map {"en": {"us": "greetings": ["hello", "hi"]} }"
//and a path "en.us.greetings" would return the list ["hello", "hi"]
//TODO: should we make export this?
//Note: require client to pass

//setValueAtPath writes the value at the given path, creating/overwriting
//intermediary values. Returns true if the value was written, false otherwise
// func setValueAtPath(m map[string]interface{}, keyPath []string, value interface{}) bool {
// 	if m == nil {
// 		return false
// 	}
// 	//
// 	var v interface{}
// 	for _, k := range keyPath {
// 		var ok bool
// 		if v, ok = m[k]; !ok {
// 			m[k] = make(map[string]interface{})
// 			m[k].(map[string]interface{})
// 			return nil //we could no longer follow key from path in map
// 		}

// 		if tm, ok := v.(map[string]interface{}); ok {
// 			m = tm
// 			continue
// 		}
// 		m = nil //we reached non-map value
// 	}
// 	return v
// }

//map, path, pathParser)

//Intersect returns true iff the slice at pa in A has some member
//a that is also a member of slice at pb in B
//Note: Interface values are comparable. Two interface values are equal if
//they have identical dynamic types and equal dynamic values or if both have value nil.
//note: prefer deep equal and use configurable input
// func Intersect(A, B map[string]interface{}, pa, pb string, pathParser PathParser) (bool, error) {
// 	if pathParser == nil {
// 		pathParser = defaultPathParser
// 	}

// 	pathA, err := pathParser(pa, ".")
// 	if err != nil {
// 		return false, err
// 	}
// 	pathB, err := pathParser(pb, ".")
// 	if err != nil {
// 		return false, err
// 	}

// 	valueAtA := getValueAtPath(A, pathA)
// 	valueAtB := getValueAtPath(B, pathB)

// 	if valueAtA == nil || valueAtB == nil {
// 		return false, nil
// 	}

// 	if reflect.TypeOf(valueAtA).Kind() != reflect.Slice ||
// 		reflect.TypeOf(valueAtB).Kind() != reflect.Slice {
// 		return false, errors.New("Only slices are supported as values")
// 	}

// 	m := make(map[interface{}]struct{})

// 	a := reflect.ValueOf(valueAtA)
// 	for i := 0; i < a.Len(); i++ {
// 		m[a.Index(i).Interface()] = struct{}{}
// 	}

// 	b := reflect.ValueOf(valueAtB)
// 	for i := 0; i < b.Len(); i++ {
// 		if _, ok := m[b.Index(i).Interface()]; ok {
// 			return true, nil
// 		}
// 	}

// 	return false, nil

// }
