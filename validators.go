package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// ValidateStatus validates that the HTTP Status code is one of a set of expected values.
type ValidateStatus struct {
	ValidStatusCodes []int
}

func (v *ValidateStatus) initialize(data map[string]interface{}) {
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

	res := ValidationResult{Name: "Status"}

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
	MinimumSize int64
	MaximumSize int64
}

func (v *ValidateSize) initialize(data map[string]interface{}) {
	v.MinimumSize = int64(data["minsize"].(int))
	v.MaximumSize = int64(data["maxsize"].(int))
}

func (v *ValidateSize) validate(e *Endpoint, er *EndpointResult, data map[string]interface{}) (bool, *ValidationResult) {

	res := ValidationResult{Name: "Size"}

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
}

func (j *ValidateJSON) initialize(data map[string]interface{}) {
}

func (j *ValidateJSON) validate(endpoint *Endpoint, response *EndpointResult, data map[string]interface{}) (bool, *ValidationResult) {

	res := ValidationResult{Name: "json"}

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
	config map[string]interface{}
}

func (j *ValidateJSONData) initialize(data map[string]interface{}) {
	j.config = data
}

func (j *ValidateJSONData) validate(endpoint *Endpoint, response *EndpointResult, data map[string]interface{}) (bool, *ValidationResult) {

	res := ValidationResult{Name: "json"}

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
	for k, v := range j.config {
		errors = append(errors, j.naviagateTree(strings.Split(k, "."), 0, v.(string), jsonData)...)
	}

	res.Errors = errors
	if len(errors) == 0 {
		res.Valid = true
	}

	return true, &res
}

func (j *ValidateJSONData) naviagateTree(keys []string, keyIndex int, command string, json interface{}) []string {

	var errors []string

	if len(keys) <= keyIndex {
		res := j.validateValue(keys, command, json)
		if len(res) > 0 {
			return []string{res}
		}
		return []string{}
	}

	key := keys[keyIndex]

	switch v := json.(type) {
	case []interface{}:
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
			errors = append(errors, fmt.Sprintf("Key element %v not found in JSON.", keys[keyIndex]))
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

	c1 := strings.SplitN(command, " ", 2)
	if len(c1) != 2 {
		return fmt.Sprintf("Error parsing JSONData comparison value %v for key %v. Should be a space between the comparison type and expected value.", command, key)
	}
	c := c1[0]
	v := c1[1]

	switch tv := value.(type) {
	case bool:
		if (tv && !strings.EqualFold(v, "true")) || (!tv && !strings.EqualFold(v, "false")) {
			return fmt.Sprintf("Boolean comparison failed for key %v. Expected value of %v did not match actual value %v", key, v, tv)
		}
	case float64:
		cv, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Sprintf("Number comparison failed for key %v. Unable to convert expected value: %v to a number.", key, v)
		}
		switch c {
		// TODO: Use compare() to shorten cases?
		case ">":
			if tv <= cv {
				return fmt.Sprintf("Number comparison failed for key %v. Actual value %v was not greater than comparison value %v", key, tv, cv)
			}
		case ">=":
			if tv < cv {
				return fmt.Sprintf("Number comparison failed for key %v. Actual value %v was not greater than or equal to comparison value %v", key, tv, cv)
			}
		case "=":
			if tv != cv {
				return fmt.Sprintf("Number comparison failed for key %v. Actual value %v was not equal to comparison value %v", key, tv, cv)
			}
		case "!=":
			if tv == cv {
				return fmt.Sprintf("Number comparison failed for key %v. Actual value %v was equal to comparison value %v", key, tv, cv)
			}
		case "<":
			if tv >= cv {
				return fmt.Sprintf("Number comparison failed for key %v. Actual value %v was not less than comparison value %v", key, tv, cv)
			}
		case "<=":
			if tv > cv {
				return fmt.Sprintf("Number comparison failed for key %v. Actual value %v was not less than or equal to comparison value %v", key, tv, cv)
			}
		}
	case string:
		switch c {
		case "=":
			if tv != v {
				return fmt.Sprintf("String comparsison failed for key %v. Actual value %v is not equal to comparison value %v", key, tv, v)
			}
		}
	case []interface{}:
		av := int64(len(tv))
		cv, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Sprintf("Array length comparison failed for key %v. Unable to convert expected value: %v to a number.", key, v)
		}
		switch c {
		case ">":
			if av <= cv {
				return fmt.Sprintf("Array length comparison failed for key %v. Actual length %v was not greater than comparison value %v", key, av, cv)
			}
		case ">=":
			if av < cv {
				return fmt.Sprintf("Array length comparison failed for key %v. Actual value %v was not greater than or equal to comparison value %v", key, av, cv)
			}
		case "=":
			if av != cv {
				return fmt.Sprintf("Array length comparison failed for key %v Actual value %v was not equal to comparison value %v", key, av, cv)
			}
		case "!=":
			if av == cv {
				return fmt.Sprintf("Number comparison failed for key %v. Actual value %v was equal to comparison value %v", key, av, cv)
			}
		case "<":
			if av >= cv {
				return fmt.Sprintf("Array length comparison failed for key %v. Actual value %v was not less than comparison value %v", key, av, cv)
			}
		case "<=":
			if av > cv {
				return fmt.Sprintf("Array length comparison failed for key %v. Actual value %v was not less than or equal to comparison value %v", key, av, cv)
			}
		}
	}

	return ""
}
