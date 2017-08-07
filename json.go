package main

import (
	"encoding/json"
)

// JSON provides validation of JSON files.
type JSON struct {
}

func (j *JSON) validate(endpoint *Endpoint, response *EndpointResult) ValidationResult {

	res := ValidationResult{EndpointResult: response, Name: "json"}

	var jsonData interface{}
	err := json.Unmarshal(response.Body, &jsonData)
	if err != nil {
		res.Errors = append(res.Errors, "JSON is not well-formed. "+err.Error())
		return res
	}

	res.Data = jsonData
	res.Valid = true
	return res
}
