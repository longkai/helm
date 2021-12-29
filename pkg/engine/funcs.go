/*
Copyright The Helm Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"text/template"

	"github.com/BurntSushi/toml"
	"github.com/Masterminds/sprig/v3"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

// funcMap returns a mapping of all of the functions that Engine has.
//
// Because some functions are late-bound (e.g. contain context-sensitive
// data), the functions may not all perform identically outside of an Engine
// as they will inside of an Engine.
//
// Known late-bound functions:
//
//	- "include"
//	- "tpl"
//
// These are late-bound in Engine.Render().  The
// version included in the FuncMap is a placeholder.
//
func funcMap() template.FuncMap {
	f := sprig.TxtFuncMap()
	delete(f, "env")
	delete(f, "expandenv")

	// Add some extra functionality
	extra := template.FuncMap{
		"toToml":           toTOML,
		"toYaml":           toYAML,
		"fromYaml":         fromYAML,
		"fromYamlArray":    fromYAMLArray,
		"fromYamlDocument": fromYamlDocument,
		"toJson":           toJSON,
		"fromJson":         fromJSON,
		"fromJsonArray":    fromJSONArray,
		"filter":           filter,
		"mustFilter":       mustFilter,

		// This is a placeholder for the "include" function, which is
		// late-bound to a template. By declaring it here, we preserve the
		// integrity of the linter.
		"include":  func(string, interface{}) string { return "not implemented" },
		"tpl":      func(string, interface{}) interface{} { return "not implemented" },
		"required": func(string, interface{}) (interface{}, error) { return "not implemented", nil },
		// Provide a placeholder for the "lookup" function, which requires a kubernetes
		// connection.
		"lookup": func(string, string, string, string) (map[string]interface{}, error) {
			return map[string]interface{}{}, nil
		},
	}

	for k, v := range extra {
		f[k] = v
	}

	return f
}

// toYAML takes an interface, marshals it to yaml, and returns a string. It will
// always return a string, even on marshal error (empty string).
//
// This is designed to be called from a template.
func toYAML(v interface{}) string {
	data, err := yaml.Marshal(v)
	if err != nil {
		// Swallow errors inside of a template.
		return ""
	}
	return strings.TrimSuffix(string(data), "\n")
}

// fromYAML converts a YAML document into a map[string]interface{}.
//
// This is not a general-purpose YAML parser, and will not parse all valid
// YAML documents. Additionally, because its intended use is within templates
// it tolerates errors. It will insert the returned error message string into
// m["Error"] in the returned map.
func fromYAML(str string) map[string]interface{} {
	m := map[string]interface{}{}

	if err := yaml.Unmarshal([]byte(str), &m); err != nil {
		m["Error"] = err.Error()
	}
	return m
}

// fromYAMLArray converts a YAML array into a []interface{}.
//
// This is not a general-purpose YAML parser, and will not parse all valid
// YAML documents. Additionally, because its intended use is within templates
// it tolerates errors. It will insert the returned error message string as
// the first and only item in the returned array.
func fromYAMLArray(str string) []interface{} {
	a := []interface{}{}

	if err := yaml.Unmarshal([]byte(str), &a); err != nil {
		a = []interface{}{err.Error()}
	}
	return a
}

// fromYamlDocument converts a YAML document into a []map[string]interface{}.
//
// Unlike fromYaml, this is a gernal-purpose YAML parser, and will parse all
// valid YAML documents. Additionally, because its intended use is within templates
// it tolerates errors. It will insert the returned error message string as
// the first and only item in the returned array.
func fromYamlDocument(str string) []map[string]interface{} {
	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(str), 256)
	var li []map[string]interface{}
	for {
		m := map[string]interface{}{}
		err := decoder.Decode(&m)
		if err != nil {
			if err == io.EOF {
				return li
			}
			m["Error"] = err.Error()
			return []map[string]interface{}{m}
		}
		li = append(li, m)
	}
}

func doFilter(item interface{}, k string, v interface{}) bool {
	rv := reflect.ValueOf(item)
	if rv.Kind() == reflect.Map {
		for _, key := range rv.MapKeys() {
			if key.String() == k {
				return reflect.DeepEqual(rv.MapIndex(key).Interface(), v)
			}
		}
		return false
	}

	var el reflect.Value
	if rv.Kind() == reflect.Ptr {
		el = rv.Elem().FieldByName(k)
	} else {
		el = rv.FieldByName(k)
	}
	return reflect.DeepEqual(el.String(), v)
}

func filter(k, v interface{}, list interface{}) []interface{} {
	l, err := mustFilter(k, v, list)
	if err != nil {
		panic(err)
	}
	return l
}

// mustFilter filters a list of items by a key/value pair.
// The item could be a struct field or a map key/value.
func mustFilter(k, v interface{}, list interface{}) ([]interface{}, error) {
	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)

		l := l2.Len()
		res := []interface{}{}
		var item interface{}
		for i := 0; i < l; i++ {
			item = l2.Index(i).Interface()
			if key := k.(string); doFilter(item, key, v) {
				res = append(res, item)
			}
		}

		return res, nil
	case reflect.Map:
		if key := k.(string); doFilter(list, key, v) {
			return []interface{}{list}, nil
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("cannot filter on type %s", tp)
	}
}

// toTOML takes an interface, marshals it to toml, and returns a string. It will
// always return a string, even on marshal error (empty string).
//
// This is designed to be called from a template.
func toTOML(v interface{}) string {
	b := bytes.NewBuffer(nil)
	e := toml.NewEncoder(b)
	err := e.Encode(v)
	if err != nil {
		return err.Error()
	}
	return b.String()
}

// toJSON takes an interface, marshals it to json, and returns a string. It will
// always return a string, even on marshal error (empty string).
//
// This is designed to be called from a template.
func toJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		// Swallow errors inside of a template.
		return ""
	}
	return string(data)
}

// fromJSON converts a JSON document into a map[string]interface{}.
//
// This is not a general-purpose JSON parser, and will not parse all valid
// JSON documents. Additionally, because its intended use is within templates
// it tolerates errors. It will insert the returned error message string into
// m["Error"] in the returned map.
func fromJSON(str string) map[string]interface{} {
	m := make(map[string]interface{})

	if err := json.Unmarshal([]byte(str), &m); err != nil {
		m["Error"] = err.Error()
	}
	return m
}

// fromJSONArray converts a JSON array into a []interface{}.
//
// This is not a general-purpose JSON parser, and will not parse all valid
// JSON documents. Additionally, because its intended use is within templates
// it tolerates errors. It will insert the returned error message string as
// the first and only item in the returned array.
func fromJSONArray(str string) []interface{} {
	a := []interface{}{}

	if err := json.Unmarshal([]byte(str), &a); err != nil {
		a = []interface{}{err.Error()}
	}
	return a
}
