package main

import (
	"reflect"
	"testing"
)

func TestValidateJSONDataMissingKeySimple(t *testing.T) {

	j := &ValidateJSONData{}

	key1 := map[interface{}]interface{}{
		"key2": "< 10",
	}
	key2 := map[interface{}]interface{}{
		"basekey": "> 0",
	}

	keys := []interface{}{
		key1,
		key2,
	}

	config := make(map[string]interface{})
	config["keys"] = keys

	j.initialize(config)

	endpoint := &Endpoint{Name: "Test Endpoint"}
	endpointResult := &EndpointResult{}
	endpointResult.Body = []byte("{}")

	_, res := j.validate(endpoint, endpointResult, nil)
	if res.Valid {
		t.Errorf("Invalid data sent to ValidateJSONData but recieved an error for data: %v", string(endpointResult.Body))
	}

	endpointResult.Body = []byte(`{"key2": 9, "basekey": 3}`)
	_, res = j.validate(endpoint, endpointResult, nil)
	if !res.Valid {
		t.Errorf("Valid data sent to ValidateJSONData but recieved an error for data: %v with errors: %v", string(endpointResult.Body), res.Errors)
	}
}

func TestValidateJSONDataMissingKeyComplex(t *testing.T) {

	j := &ValidateJSONData{}

	key1 := map[interface{}]interface{}{
		"[].key1": "< 10",
	}

	keys := []interface{}{
		key1,
	}

	config := make(map[string]interface{})
	config["keys"] = keys

	j.initialize(config)

	endpoint := &Endpoint{Name: "Test Endpoint"}
	endpointResult := &EndpointResult{}
	endpointResult.Body = []byte(`[{"key2":4}]`)

	_, res := j.validate(endpoint, endpointResult, nil)
	if res.Valid {
		t.Errorf("Invalid data sent to ValidateJSONData but recieved an error for data: %v", string(endpointResult.Body))
	}

	endpointResult.Body = []byte(`[{"key1":4},{"key1":5}]`)
	_, res = j.validate(endpoint, endpointResult, nil)
	if !res.Valid {
		t.Errorf("Valid data sent to ValidateJSONData but recieved an error for data: %v with errors: %v", string(endpointResult.Body), res.Errors)
	}
}

// TestValidateJSONDataValidateValueWithBool Tests the ValidateJSONData.validateValues method with booleans.
func TestValidateJSONDataValidateValueWithBool(t *testing.T) {

	j := &ValidateJSONData{}

	keys := []string{"test", "key", "set"}

	validate(t, j, keys, "= true", true, true)
	validate(t, j, keys, "= true", false, false)
	validate(t, j, keys, "= false", false, true)
	validate(t, j, keys, "= false", true, false)
	validate(t, j, keys, "type bool", true, true)
	validate(t, j, keys, "type string", true, false)
	validate(t, j, keys, "type number", false, false)
	validate(t, j, keys, "type array", false, false)
}

// TestValidateJSONDataValidateValueWithNumber Tests the ValidateJSONData.validateValues method with numbers.
func TestValidateJSONDataValidateValueWithNumber(t *testing.T) {

	j := &ValidateJSONData{}

	keys := []string{"test", "key", "set"}

	validate(t, j, keys, "= 3.65", 3.65, true)
	validate(t, j, keys, "= 3.65", 3.6, false)
	validate(t, j, keys, "!= 3.65", 3.64, true)
	validate(t, j, keys, "!= 3.65", 3.65, false)
	validate(t, j, keys, "> 3.65", 3.66, true)
	validate(t, j, keys, "> 3.65", 3.65, false)
	validate(t, j, keys, "> 3.65", 3.64, false)
	validate(t, j, keys, ">= 3.65", 3.66, true)
	validate(t, j, keys, ">= 3.65", 3.65, true)
	validate(t, j, keys, ">= 3.65", 3.64, false)
	validate(t, j, keys, "< 3.65", 3.64, true)
	validate(t, j, keys, "< 3.65", 3.65, false)
	validate(t, j, keys, "< 3.65", 3.66, false)
	validate(t, j, keys, "<= 3.65", 3.64, true)
	validate(t, j, keys, "<= 3.65", 3.65, true)
	validate(t, j, keys, "<= 3.65", 3.66, false)

	validate(t, j, keys, "type bool", 3.65, false)
	validate(t, j, keys, "type string", 3.65, false)
	validate(t, j, keys, "type number", 3.65, true)
	validate(t, j, keys, "type array", 3.65, false)
}

// TestValidateJSONDataValidateValueWithString Tests the ValidateJSONData.validateValues method with strings.
func TestValidateJSONDataValidateValueWithString(t *testing.T) {

	j := &ValidateJSONData{}

	keys := []string{"test", "key", "set"}

	validate(t, j, keys, "= Test", "Test", true)
	validate(t, j, keys, "!= Test", "Test", false)
	validate(t, j, keys, "= Test", "not Test", false)
	validate(t, j, keys, "!= Test", "not Test", true)

	validate(t, j, keys, "type bool", "test", false)
	validate(t, j, keys, "type string", "test", true)
	validate(t, j, keys, "type number", "test", false)
	validate(t, j, keys, "type array", "test", false)
}

// TestValidateJSONDataValidateValueWithArray Tests the ValidateJSONData.validateValues method with arrays.
func TestValidateJSONDataValidateValueWithArray(t *testing.T) {

	j := &ValidateJSONData{}

	keys := []string{"test", "key", "set"}

	a := make([]interface{}, 3)
	a[0] = "test"
	a[1] = "key"
	a[2] = "set"

	validate(t, j, keys, "= 3", a, true)
	validate(t, j, keys, "= 4", a, false)
	validate(t, j, keys, "!= 3", a, false)
	validate(t, j, keys, "!= 4", a, true)
	validate(t, j, keys, "> 2", a, true)
	validate(t, j, keys, "> 3", a, false)
	validate(t, j, keys, ">= 2", a, true)
	validate(t, j, keys, ">= 3", a, true)
	validate(t, j, keys, ">= 4", a, false)
	validate(t, j, keys, "< 4", a, true)
	validate(t, j, keys, "< 3", a, false)
	validate(t, j, keys, "<= 4", a, true)
	validate(t, j, keys, "<= 3", a, true)
	validate(t, j, keys, "<= 2", a, false)

	validate(t, j, keys, "type bool", a, false)
	validate(t, j, keys, "type string", a, false)
	validate(t, j, keys, "type number", a, false)
	validate(t, j, keys, "type array", a, true)
}

func shouldBe(t *testing.T, res string, shouldBe bool) {

	if shouldBe && len(res) > 0 {
		t.Error("Expected empty string, got", res)
	} else if !shouldBe && len(res) == 0 {
		t.Error("Expected errors string but got empty string.")
	}
}

func validate(t *testing.T, j *ValidateJSONData, keys []string, command string, value interface{}, shouldPass bool) {

	res := j.validateValue(keys, command, value)
	if shouldPass && len(res) > 0 {
		t.Errorf("For Command '%v' and Value '%v' of type %v, expected empty string, got %v", command, value, reflect.TypeOf(value), res)
	} else if !shouldPass && len(res) == 0 {
		t.Errorf("For Command '%v' and Value '%v' of type %v, expected errors string but got empty string.", command, value, reflect.TypeOf(value))
	}

}
