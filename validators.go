package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// ValidateStatus validates that the HTTP Status code is one of a set of expected values.
type ValidateStatus struct {
	Name             string
	ValidStatusCodes []int
}

func (v *ValidateStatus) initialize(name string, data map[string]interface{}) {
	v.Name = name
	s := data["status"]
	switch ts := s.(type) {
	case int:
		v.ValidStatusCodes = []int{ts}
	case []interface{}:
		status := make([]int, len(ts))
		for i, v := range ts {
			status[i] = v.(int)
		}
		v.ValidStatusCodes = status
	}
}

func (v *ValidateStatus) validate(e *Endpoint, er *EndpointResult, data map[string]interface{}) (bool, *ValidationResult) {

	res := ValidationResult{Name: v.Name}

	for _, status := range v.ValidStatusCodes {
		if er.Status == status {
			res.Valid = true
			break
		}
	}
	if !res.Valid {
		res.Errors = append(res.Errors, fmt.Sprintf("Status Code %d does not match expected Status Code(s): %v", er.Status, v.ValidStatusCodes))
	}

	return true, &res
}

// ValidateSize validates the size of the result returned.
type ValidateSize struct {
	Name        string
	MinimumSize int64
	MaximumSize int64
}

func (v *ValidateSize) initialize(name string, data map[string]interface{}) {
	v.Name = name
	v.MinimumSize = int64(data["minsize"].(int))
	v.MaximumSize = int64(data["maxsize"].(int))
}

func (v *ValidateSize) validate(e *Endpoint, er *EndpointResult, data map[string]interface{}) (bool, *ValidationResult) {

	res := ValidationResult{Name: v.Name}

	res.Valid = true
	if v.MinimumSize > 0 && er.Size < v.MinimumSize {
		res.Valid = false
		res.Errors = append(res.Errors, fmt.Sprintf("Size of body (%d) was smaller than the minimum size (%d).", er.Size, v.MinimumSize))
	}
	if v.MaximumSize > 0 && er.Size > v.MaximumSize {
		res.Valid = false
		res.Errors = append(res.Errors, fmt.Sprintf("Size of body (%d) was larger than the maximum size (%d).", er.Size, v.MaximumSize))
	}

	return true, &res
}

// ValidateJSON provides validation of JSON files.
type ValidateJSON struct {
	Name string
}

func (j *ValidateJSON) initialize(name string, data map[string]interface{}) {
	j.Name = name
}

func (j *ValidateJSON) validate(endpoint *Endpoint, response *EndpointResult, data map[string]interface{}) (bool, *ValidationResult) {

	res := ValidationResult{Name: j.Name}

	var jsonData interface{}
	err := json.Unmarshal(response.Body, &jsonData)
	if err != nil {
		res.Errors = append(res.Errors, "JSON is not well-formed. "+err.Error())
		return false, &res
	}

	res.Valid = true
	data["data"] = jsonData

	return res.Valid, &res
}

// ValidateJSONData provides validation of specific values in JSON files.
type ValidateJSONData struct {
	Name       string
	config     []map[interface{}]interface{}
	arrayRegex *regexp.Regexp
}

func (j *ValidateJSONData) initialize(name string, data map[string]interface{}) {
	j.Name = name
	var c []map[interface{}]interface{}
	for _, v := range data["keys"].([]interface{}) {
		c = append(c, v.(map[interface{}]interface{}))
	}
	j.config = c

	j.arrayRegex = regexp.MustCompile(`\Q[\E(\d+)\Q]\E`)
}

func (j *ValidateJSONData) validate(endpoint *Endpoint, response *EndpointResult, data map[string]interface{}) (bool, *ValidationResult) {

	res := ValidationResult{Name: j.Name}

	var jsonData interface{}
	jsonData, ok := data["data"]
	if !ok {
		err := json.Unmarshal(response.Body, &jsonData)
		if err != nil {
			res.Errors = append(res.Errors, "JSON is not well-formed. "+err.Error())
			return false, &res
		}
	}

	var errors []string
	for _, av := range j.config {
		for k, v := range av {
			errors = append(errors, j.naviagateTree(strings.Split(k.(string), "."), 0, v.(string), jsonData)...)
		}
	}

	res.Errors = errors
	if len(errors) == 0 {
		res.Valid = true
	}

	return true, &res
}

func (j *ValidateJSONData) naviagateTree(keys []string, keyIndex int, command string, json interface{}) []string {

	var errors []string

	optional := strings.HasPrefix(command, "?")

	if len(keys) <= keyIndex || (len(keys) == 1 && keys[0] == "[]") {
		res := j.validateValue(keys, command, json)
		if len(res) > 0 {
			return []string{res}
		}
		return []string{}
	}

	key := keys[keyIndex]

	switch v := json.(type) {
	case []interface{}:
		match, _ := regexp.Match(`\Q[\E\d+\Q]\E`, []byte(key))
		if match {
			arrayIndexString := j.arrayRegex.FindStringSubmatch(key)[1]
			arrayIndex, err := strconv.ParseInt(arrayIndexString, 10, 64)
			if err != nil {
				return append(errors, fmt.Sprintf("Array access failed for key %v. Unable to convert index value: %v to a number.", key, arrayIndexString))
			}
			if arrayIndex >= int64(len(v)) {
				errors = append(errors, fmt.Sprintf("Array at key %v does not have an element at index %v. Array size is: %d", key, arrayIndex, len(v)))
				return errors
			}
			errors = append(errors, j.naviagateTree(keys, keyIndex+1, command, v[arrayIndex])...)
			return errors
		}

		if key != "[]" {
			errors = append(errors, fmt.Sprintf("Key %v at index %d not defined as array, but json data is of type array.", key, keyIndex))
			return errors
		}
		for _, v1 := range v {
			errors = append(errors, j.naviagateTree(keys, keyIndex+1, command, v1)...)
		}
		return errors
	case map[string]interface{}:
		if key == "[]" {
			errors = append(errors, fmt.Sprintf("Key element at index %d defined as array, but json element is an object.", keyIndex))
			return errors
		}
		v1, ok := v[keys[keyIndex]]
		if !ok {
			if !optional {
				errors = append(errors, fmt.Sprintf("Key element %v not found in JSON.", keys[keyIndex]))
			}
			return errors
		}
		errors := j.naviagateTree(keys, keyIndex+1, command, v1)
		return errors
	default:
		return append(errors, fmt.Sprintf("Error processing validation for key %v. Element has type %v", key, reflect.TypeOf(v)))
	}
}

func (j *ValidateJSONData) validateValue(keys []string, command string, value interface{}) string {

	key := strings.Join(keys, ".")

	optional := strings.HasPrefix(command, "?")
	if optional {
		command = strings.TrimPrefix(command, "?")
	}

	c1 := strings.SplitN(command, " ", 2)
	if len(c1) != 2 {
		return fmt.Sprintf("Error parsing JSONData comparison value %v for key %v. Should be a space between the comparison type and expected value.", command, key)
	}
	c := strings.ToLower(c1[0])
	v := c1[1]

	switch tv := value.(type) {
	case bool:
		switch c {
		case "type":
			if !(strings.EqualFold(v, "bool") || strings.EqualFold(v, "boolean")) {
				return fmt.Sprintf("Type comparison failed for key %v. Actual value: %v was a number but expected type %v", key, tv, v)
			}
		case "=":
			if (tv && !strings.EqualFold(v, "true")) || (!tv && !strings.EqualFold(v, "false")) {
				return fmt.Sprintf("Boolean comparison failed for key %v. Expected value of %v did not match actual value %v", key, v, tv)
			}
		default:
			return fmt.Sprintf("Unknown comparison  %v for boolean type for key %v.", c, key)
		}
	case float64:
		if strings.EqualFold(c, "type") {
			if !(strings.EqualFold(v, "number") || strings.EqualFold(v, "int")) {
				return fmt.Sprintf("Type comparison failed for key %v. Actual value: %v was a number but expected type %v", key, tv, v)
			}
			return ""
		}

		cv, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Sprintf("Number comparison failed for key %v. Unable to convert expected value: %v to a number.", key, v)
		}
		return j.compareNumbers(key, c, tv, cv, "Number")
	case string:

		if strings.HasPrefix(c, "len") {
			lenC := strings.TrimPrefix(c, "len")
			cv, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return fmt.Sprintf("String Length comparison failed for key %v. Unable to convert expected value: %v to a number.", key, v)
			}
			return j.compareNumbers(key, lenC, float64(len(tv)), cv, "String Length")
		}

		switch c {
		case "type":
			if !strings.EqualFold(v, "string") {
				return fmt.Sprintf("Type comparison failed for key %v. Actual value: %v was a string but expected type %v", key, tv, v)
			}
		case "=":
			if tv != v {
				return fmt.Sprintf("String comparison failed for key %v. Actual value %v is not equal to comparison value %v", key, tv, v)
			}
		case "!=":
			if tv == v {
				return fmt.Sprintf("String comparison failed for key %v. Actual value %v is equal to comparison value %v", key, tv, v)
			}
		default:
			return fmt.Sprintf("Unknown comparison %v for string type for key %v.", c, key)
		}
	case []interface{}:
		if strings.EqualFold(c, "type") {
			if !(strings.EqualFold(v, "[]") || strings.EqualFold(v, "array")) {
				return fmt.Sprintf("Type comparison failed for key %v. Actual value: %v was an array but expected type %v", key, tv, v)
			}
			return ""
		}

		if strings.HasPrefix(c, "len") {
			lenC := strings.TrimPrefix(c, "len")
			cv, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return fmt.Sprintf("Array Length comparison failed for key %v. Unable to convert expected value: %v to a number.", key, v)
			}
			return j.compareNumbers(key, lenC, float64(len(tv)), cv, "Array Length")
		}
		return fmt.Sprintf("Unknown comparison %v for array type for key %v.", c, key)
	default:
		return fmt.Sprintf("Unexpected type %v encountered for key %v of value %v.", reflect.TypeOf(tv), key, tv)
	}

	return ""
}

func (j *ValidateJSONData) compareNumbers(key string, c string, v1 float64, v2 float64, comparisonType string) string {
	switch c {
	case ">":
		if v1 <= v2 {
			return fmt.Sprintf("%v comparison failed for key %v. Actual value %v was not greater than comparison value %v", comparisonType, key, v1, v2)
		}
	case ">=":
		if v1 < v2 {
			return fmt.Sprintf("%v comparison failed for key %v. Actual value %v was not greater than or equal to comparison value %v", comparisonType, key, v1, v2)
		}
	case "=":
		if v1 != v2 {
			return fmt.Sprintf("%v comparison failed for key %v. Actual value %v was not equal to comparison value %v", comparisonType, key, v1, v2)
		}
	case "!=":
		if v1 == v2 {
			return fmt.Sprintf("%v comparison failed for key %v. Actual value %v was equal to comparison value %v", key, comparisonType, v1, v2)
		}
	case "<":
		if v1 >= v2 {
			return fmt.Sprintf("%v comparison failed for key %v. Actual value %v was not less than comparison value %v", comparisonType, key, v1, v2)
		}
	case "<=":
		if v1 > v2 {
			return fmt.Sprintf("%v comparison failed for key %v. Actual value %v was not less than or equal to comparison value %v", comparisonType, key, v1, v2)
		}
	default:
		return fmt.Sprintf("Unknown comparison %v for %v type for key %v.", c, comparisonType, key)
	}
	return ""
}
