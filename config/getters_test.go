package config

import (
	"testing"
)

var testConfig = Config{
	Vars: map[string]interface{}{
		"stringKey": "value1",
		"intKey":    123,
		"boolKey":   true,
		"map[string]interface {}Key": map[string]interface{}{
			"key1": "value1",
		},
		"[]stringKey": []string{"value1", "value2"},
		"[]intKey":    []int{1, 2},
	},
}

var testData = []struct {
	name     string
	expected interface{}
	typeStr  string
}{
	{
		name:     "existing string key",
		expected: testConfig.Vars["stringKey"],
		typeStr:  "string",
	},
	{
		name:     "existing int key",
		expected: testConfig.Vars["intKey"],
		typeStr:  "int",
	},
	{
		name:     "existing bool key",
		expected: testConfig.Vars["boolKey"],
		typeStr:  "bool",
	},
	{
		name:     "existing string slice key",
		expected: testConfig.Vars["[]stringeKey"],
		typeStr:  "[]string",
	},
	{
		name:     "existing int slice key",
		expected: testConfig.Vars["[]intKey"],
		typeStr:  "[]int",
	},
	{
		name:     "existing map key",
		expected: testConfig.Vars["map[string]interface {}Key"],
		typeStr:  "map[string]interface {}",
	},
	{
		name:     "missing key",
		expected: nil,
		typeStr:  "missing",
	},
}

func TestGetVar(t *testing.T) {

	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			val, valType := testConfig.GetVar(tt.typeStr + "Key")

			// Leave the complex object validation for other tests
			if valType == "string" || valType == "int" || valType == "bool" {
				if val != tt.expected {
					t.Errorf("expected value %v, got %v", tt.expected, val)
				}
				if valType != tt.typeStr {
					t.Errorf("expected type %v, got %v", tt.typeStr, valType)
				}
			}
			if valType == "" {
				t.Errorf("expected a value type, got empty string")
			}
		})
	}
}

func TestGetString(t *testing.T) {
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			val := testConfig.GetString(tt.typeStr + "Key")

			if tt.typeStr == "string" {
				if val != tt.expected {
					t.Errorf("expected value %v, got %v", tt.expected, val)
				}
			} else {
				if val != "" {
					t.Errorf("expected empty string, got %v", val)
				}
			}
		})
	}
}

func TestGetInt(t *testing.T) {
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			val := testConfig.GetInt(tt.typeStr + "Key")

			if tt.typeStr == "int" {
				if val != tt.expected {
					t.Errorf("expected value %v, got %v", tt.expected, val)
				}
			} else {
				if val != 0 {
					t.Errorf("expected 0, got %v", val)
				}
			}
		})
	}
}

func TestGetBool(t *testing.T) {
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			val := testConfig.GetBool(tt.typeStr + "Key")

			if tt.typeStr == "bool" {
				if val != tt.expected {
					t.Errorf("expected value %v, got %v", tt.expected, val)
				}
			} else {
				if val != false {
					t.Errorf("expected false, got %v", val)
				}
			}
		})
	}
}

func TestGetMap(t *testing.T) {
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			val := testConfig.GetMap(tt.typeStr + "Key")

			if tt.typeStr == "map[string]interface {}" {
				for k, v := range val {
					if testConfig.Vars["map[string]interface {}Key"].(map[string]interface{})[k] != v {
						t.Errorf("expected value %v, got %v", testConfig.Vars["map[string]interface {}Key"].(map[string]interface{})[k], v)
					}
				}
			} else {
				if val != nil {
					t.Errorf("expected nil, got %v", val)
				}
			}
		})
	}
}

func TestGetStringSlice(t *testing.T) {
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			val := testConfig.GetStringSlice(tt.typeStr + "Key")

			if tt.typeStr == "[]string" {
				for i, v := range val {
					if testConfig.Vars["[]stringKey"].([]string)[i] != v {
						t.Errorf("expected value %v, got %v", testConfig.Vars["[]stringKey"].([]string)[i], v)
					}
				}
			} else {
				if val != nil {
					t.Errorf("expected nil, got %v", val)
				}
			}
		})
	}
}

func TestGetIntSlice(t *testing.T) {
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			val := testConfig.GetIntSlice(tt.typeStr + "Key")

			if tt.typeStr == "[]int" {
				for i, v := range val {
					if testConfig.Vars["[]intKey"].([]int)[i] != v {
						t.Errorf("expected value %v, got %v", testConfig.Vars["[]intKey"].([]int)[i], v)
					}
				}
			} else {
				if val != nil {
					t.Errorf("expected nil, got %v", val)
				}
			}
		})
	}
}
