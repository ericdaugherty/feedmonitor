package main

import (
	"encoding/json"
	"fmt"
)

// ValidateSizeStatus validates the size of the result returned and the status codes.
type ValidateSizeStatus struct {
	ValidStatusCodes []int
	MinimumSize      int64
	MaximumSize      int64
}

func (v *ValidateSizeStatus) validate(e *Endpoint, er *EndpointResult, data map[string]interface{}) (bool, *ValidationResult) {

	vr := ValidationResult{EndpointResult: er, Name: "SizeStatus"}

	statusValid := false
	for _, status := range v.ValidStatusCodes {
		if er.Status == status {
			statusValid = true
			break
		}
	}
	if !statusValid {
		vr.Errors = append(vr.Errors, fmt.Sprintf("Status Code %d does not match expected Status Code(s): %v", er.Status, v.ValidStatusCodes))
	}

	sizeValid := true
	if v.MinimumSize > 0 && er.Size < v.MinimumSize {
		sizeValid = false
		vr.Errors = append(vr.Errors, fmt.Sprintf("Size of body (%d) was smaller than the minimum size (%d).", er.Size, v.MinimumSize))
	}
	if v.MaximumSize > 0 && er.Size > v.MaximumSize {
		sizeValid = false
		vr.Errors = append(vr.Errors, fmt.Sprintf("Size of body (%d) was larger than the maximum size (%d).", er.Size, v.MaximumSize))
	}

	return statusValid && sizeValid, &vr
}

// ValidateJSON provides validation of JSON files.
type ValidateJSON struct {
}

func (j *ValidateJSON) validate(endpoint *Endpoint, response *EndpointResult, data map[string]interface{}) (bool, *ValidationResult) {

	res := ValidationResult{EndpointResult: response, Name: "json"}

	var jsonData interface{}
	err := json.Unmarshal(response.Body, &jsonData)
	if err != nil {
		res.Errors = append(res.Errors, "JSON is not well-formed. "+err.Error())
		return false, &res
	}

	res.Valid = true
	log.Debugf("Setting Data")
	data["data"] = jsonData

	return true, &res
}
