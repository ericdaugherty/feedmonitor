package main

import (
	"encoding/json"
	"fmt"
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
