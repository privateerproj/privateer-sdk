package config

import (
	"fmt"
	"io"

	"github.com/spf13/viper"
)

// fileSettings belongs to the active global Viper, just like targetFlags.
// Configuration loading and mutation must finish before concurrent consumers run.
var fileSettings struct {
	owner  *viper.Viper
	source *fileSettingsDecoderRegistry
}

// ReadInConfig loads Viper's selected file and retains its unshadowed settings.
// Use this instead of viper.ReadInConfig before NewConfig: AutomaticEnv hides
// the file values needed for true-wins ai_skip and config-only ai_api_key_env.
func ReadInConfig() error {
	return readWithFileSettings(viper.ReadInConfig)
}

// ReadConfig loads a replacement configuration from a reader, using Viper's
// configured format. Like ReadInConfig, it preserves unshadowed file settings.
// Configure overrides with viper.Set; use another replacement read to reload.
// Viper's MergeConfig and MergeConfigMap bypass this source-tracking contract.
// These helpers use Viper's built-in codecs, replacing any custom decoder registry.
func ReadConfig(in io.Reader) error {
	return readWithFileSettings(func() error { return viper.ReadConfig(in) })
}

func readWithFileSettings(read func() error) error {
	registry := &fileSettingsDecoderRegistry{DecoderRegistry: viper.NewCodecRegistry()}
	if fileSettings.owner == viper.GetViper() && fileSettings.source != nil {
		registry.raw = fileSettings.source.raw
	}
	viper.SetOptions(viper.WithDecoderRegistry(registry))
	err := read()
	fileSettings.owner = viper.GetViper()
	fileSettings.source = registry
	return err
}

type fileSettingsDecoderRegistry struct {
	viper.DecoderRegistry
	raw *viper.Viper
}

func (r *fileSettingsDecoderRegistry) Decoder(format string) (viper.Decoder, error) {
	decoder, err := r.DecoderRegistry.Decoder(format)
	if err != nil {
		return nil, err
	}
	return fileSettingsDecoder{decoder, r}, nil
}

type fileSettingsDecoder struct {
	viper.Decoder
	registry *fileSettingsDecoderRegistry
}

func (d fileSettingsDecoder) Decode(data []byte, values map[string]interface{}) error {
	if err := d.Decoder.Decode(data, values); err != nil {
		return err
	}
	raw := viper.New()
	if err := raw.MergeConfigMap(cloneFileMap(values)); err != nil {
		return fmt.Errorf("capture file settings: %w", err)
	}
	d.registry.raw = raw
	return nil
}

func cloneFileMap(values map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(values))
	for key, value := range values {
		result[key] = cloneFileValue(value)
	}
	return result
}

func cloneFileValue(value interface{}) interface{} {
	switch value := value.(type) {
	case map[string]interface{}:
		return cloneFileMap(value)
	case map[interface{}]interface{}:
		result := make(map[interface{}]interface{}, len(value))
		for key, item := range value {
			result[key] = cloneFileValue(item)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(value))
		for i, item := range value {
			result[i] = cloneFileValue(item)
		}
		return result
	default:
		return value
	}
}

func rawFileSettings() *viper.Viper {
	if fileSettings.owner == viper.GetViper() && fileSettings.source != nil && fileSettings.source.raw != nil {
		return fileSettings.source.raw
	}
	return nil
}

// Reject potentially masked file values only when enabled AI needs that source.
// Viper does not expose provenance for values equal to an environment override.
func requireUnshadowedFileSetting(fileConfig *viper.Viper, key string) error {
	if fileConfig == nil && viper.InConfig(key) && isAIEnvironmentValue(key, viper.Get(key)) {
		return fmt.Errorf("load configuration with config.ReadConfig or config.ReadInConfig to preserve file %s", key)
	}
	return nil
}
