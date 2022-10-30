package azure

import (
	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/utils"
)

// TenantID returns the azure Tenant in which the tests should be executed, configured by the user and may be set by the environment variable AZURE_TENANT_ID.
func TenantID() (string, error) {
	if config.GlobalConfig.CloudProviders.Azure.TenantID == "" {
		return "", utils.ReformatError("Required config var not set: CloudProviders.Azure.TenantID")
	}
	return config.GlobalConfig.CloudProviders.Azure.TenantID, nil
}

// ClientID returns the client (typically a service principal) that must be authorized for performing operations within the azure tenant, configured by the user and may be set by the environment variable AZURE_CLIENT_ID.
func ClientID() (string, error) {
	if config.GlobalConfig.CloudProviders.Azure.ClientID == "" {
		return "", utils.ReformatError("Required config var not set: CloudProviders.Azure.ClientID")
	}
	return config.GlobalConfig.CloudProviders.Azure.ClientID, nil
}

// ClientSecret returns the client secret to allow client authetication and authorization, configured by the user and may be set by the environment variable AZURE_CLIENT_SECRET.
func ClientSecret() (string, error) {
	if config.GlobalConfig.CloudProviders.Azure.ClientSecret == "" {
		return "", utils.ReformatError("Required config var not set: CloudProviders.Azure.ClientSecret")
	}
	return config.GlobalConfig.CloudProviders.Azure.ClientSecret, nil
}

// SubscriptionID returns the azure Subscription in which the tests should be executed, configured by the user and may be set by the environment variable AZURE_SUBSCRIPTION_ID.
func SubscriptionID() (string, error) {
	if config.GlobalConfig.CloudProviders.Azure.SubscriptionID == "" {
		return "", utils.ReformatError("Required config var not set: CloudProviders.Azure.SubscriptionID")
	}
	return config.GlobalConfig.CloudProviders.Azure.SubscriptionID, nil
}

// ResourceGroup returns the Privateer user's azure resource group in which resurces should be created fpr testing, configured by the user and may be set by the environment variable AZURE_RESOURCE_GROUP.
func ResourceGroup() (string, error) {
	if config.GlobalConfig.CloudProviders.Azure.ResourceGroup == "" {
		return "", utils.ReformatError("Required config var not set: CloudProviders.Azure.ResourceGroup")
	}
	return config.GlobalConfig.CloudProviders.Azure.ResourceGroup, nil
}

// ResourceLocation returns the default location in which azure resources should be created, configured by the user and may be set by the environment variable AZURE_LOCATION.
func ResourceLocation() (string, error) {
	if config.GlobalConfig.CloudProviders.Azure.ResourceLocation == "" {
		return "", utils.ReformatError("Required config var not set: CloudProviders.Azure.ResourceLocation")
	}
	return config.GlobalConfig.CloudProviders.Azure.ResourceLocation, nil
}

// ManagementGroup returns an Azure Management Group which may be used for policy assignment, configured by the user and may be set by the environment variable AZURE_MANAGEMENT_GROUP.
func ManagementGroup() string {
	return config.GlobalConfig.CloudProviders.Azure.ManagementGroup
}
