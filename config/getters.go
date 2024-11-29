package config

import (
	"fmt"
)

// GetVar retrieves the value associated with the given key from the Config's Vars map.
// It returns the value as an interface{} and a string representing the type of the value.
// Useful for debugging in the event that a value is not being retrieved as expected.
//
// Parameters:
//   - key: The key name in the config vars.
//
// Returns:
//   - interface{}: The value associated with the key.
//   - string: The type of the value associated with the key. If the key does not exist, it returns "missing".
func (c *Config) GetVar(key string) (interface{}, string) {
	val, ok := c.Vars[key]
	if !ok {
		return nil, "missing"
	}
	return val, fmt.Sprintf("%T", val)
}

// GetString retrieves the value associated with the given key from the Config object.
// If the value is not of type string, it returns an empty string.
//
// Parameters:
//   - key: The key name in the config vars.
//
// Returns:
//   - string: The value associated with the key, or an empty string if the value is not of type string.
func (c *Config) GetString(key string) string {
	val, valType := c.GetVar(key)
	if valType != "string" {
		return ""
	}
	return val.(string)
}

// GetInt retrieves the value associated with the given key as an integer.
// If the value is not of type int, it returns 0.
//
// Parameters:
//   - key: The key name in the config vars.
//
// Returns:
//   - int: The value associated with the key, or 0 if the value is not of type int.
func (c *Config) GetInt(key string) int {
	val, valType := c.GetVar(key)
	if valType != "int" {
		return 0
	}
	return val.(int)
}

// GetBool retrieves the value associated with the given key as a boolean.
// If the value is not of type boolean, it returns false.
//
// Parameters:
//   - key: The key name in the config vars.
//
// Returns:
//   - bool: The boolean value associated with the key, or false if the value is not a boolean.
func (c *Config) GetBool(key string) bool {
	val, valType := c.GetVar(key)
	if valType != "bool" {
		return false
	}
	return val.(bool)
}

// GetMap retrieves a value from the configuration as a map[string]interface{}.
// It takes a key as a string and returns the corresponding value if it is of type map[string]interface{}.
// If the value is not of the expected type, it returns nil.
//
// Parameters:
//   - key: The key name in the config vars.
//
// Returns:
//   - map[string]interface{}: The value associated with the key if it is of the correct type
//   - nil: If the value is not of type map[string]interface{} or the key does not exist.
func (c *Config) GetMap(key string) map[string]interface{} {
	val, valType := c.GetVar(key)
	if valType != "map[string]interface {}" {
		return nil
	}
	return val.(map[string]interface{})
}

// GetStringSlice retrieves the value associated with the given key as a slice of strings.
// If the value is not of type []string, it returns nil.
//
// Parameters:
//   - key: The key name in the config vars.
//
// Returns:
//   - []string: The value associated with the key as a slice of strings
//   - nil: If the value is not of type []string or the key does not exist.
func (c *Config) GetStringSlice(key string) []string {
	val, valType := c.GetVar(key)
	if valType != "[]string" {
		return nil
	}
	return val.([]string)
}

// GetIntSlice retrieves the value associated with the given key as a slice of integers.
// If the value is not of type []int, it returns nil.
//
// Parameters:
//   - key: The key name in the config vars.
//
// Returns:
//   - []int: The value associated with the key as a slice of integers
//   - nil: If the value is not of type []int or the key does not exist.
func (c *Config) GetIntSlice(key string) []int {
	val, valType := c.GetVar(key)
	if valType != "[]int" {
		return nil
	}
	return val.([]int)
}

// GetBoolSlice retrieves a slice of boolean values from the configuration
// based on the provided key. If the key does not exist or the value is not
// of type []bool, it returns nil.
//
// Parameters:
//   - key: The key name in the config vars.
//
// Returns:
//   - []bool: A slice of boolean values if the key exists and the value is of type []bool.
//   - nil: If the key does not exist or the value is not of type []bool.
func (c *Config) GetBoolSlice(key string) []bool {
	val, valType := c.GetVar(key)
	if valType != "[]bool" {
		return nil
	}
	return val.([]bool)
}
