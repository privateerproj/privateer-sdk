package azureconfig

import "github.com/privateerproj/privateer-sdk/config/setter"

// Azure config options that may be required by any service pack
type Azure struct {
	Excluded         string `yaml:"Excluded"`
	TenantID         string `yaml:"TenantID"`
	SubscriptionID   string `yaml:"SubscriptionID"`
	ClientID         string `yaml:"ClientID"`
	ClientSecret     string `yaml:"ClientSecret"`
	ResourceGroup    string `yaml:"ResourceGroup"`
	ResourceLocation string `yaml:"ResourceLocation"`
	ManagementGroup  string `yaml:"ManagementGroup"`
}

// SetEnvAndDefaults will associate ENV variables and default values to each Azure field
func (ctx *Azure) SetEnvAndDefaults() {
	setter.SetVar(&ctx.TenantID, "Privateer_AZURE_TENANT_ID", "")
	setter.SetVar(&ctx.SubscriptionID, "Privateer_AZURE_SUBSCRIPTION_ID", "")
	setter.SetVar(&ctx.ClientID, "Privateer_AZURE_CLIENT_ID", "")
	setter.SetVar(&ctx.ClientSecret, "Privateer_AZURE_CLIENT_SECRET", "")
	setter.SetVar(&ctx.ResourceGroup, "Privateer_AZURE_RESOURCE_GROUP", "")
	setter.SetVar(&ctx.ResourceLocation, "Privateer_AZURE_RESOURCE_LOCATION", "")
}
