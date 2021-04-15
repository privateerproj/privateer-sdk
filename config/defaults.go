package config

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

// setEnvOrDefaults will set value from os.Getenv and default to the specified value
func setFromEnvOrDefaults(e *VarOptions) {

	SetVar(&e.Tags, "PROBR_TAGS", "")
	SetVar(&e.AuditEnabled, "PROBR_AUDIT_ENABLED", "true")
	SetVar(&e.OutputType, "PROBR_OUTPUT_TYPE", "IO")
	SetVar(&e.WriteDirectory, "PROBR_WRITE_DIRECTORY", "probr_output")
	SetVar(&e.LogLevel, "PROBR_LOG_LEVEL", "ERROR")
	SetVar(&e.OverwriteHistoricalAudits, "OVERWRITE_AUDITS", "true")
	SetVar(&e.WriteConfig, "PROBR_LOG_CONFIG", "true")
	SetVar(&e.ResultsFormat, "PROBR_RESULTS_FORMAT", "cucumber")

	SetVar(&e.ServicePacks.Kubernetes.KeepPods, "PROBR_KEEP_PODS", "false")
	SetVar(&e.ServicePacks.Kubernetes.KubeConfigPath, "KUBE_CONFIG", getDefaultKubeConfigPath())
	SetVar(&e.ServicePacks.Kubernetes.KubeContext, "KUBE_CONTEXT", "")
	SetVar(&e.ServicePacks.Kubernetes.SystemClusterRoles, "", []string{"system:", "aks", "cluster-admin", "policy-agent"})
	SetVar(&e.ServicePacks.Kubernetes.AuthorisedContainerRegistry, "PROBR_AUTHORISED_REGISTRY", "")
	SetVar(&e.ServicePacks.Kubernetes.UnauthorisedContainerRegistry, "PROBR_UNAUTHORISED_REGISTRY", "")
	SetVar(&e.ServicePacks.Kubernetes.ProbeImage, "PROBR_PROBE_IMAGE", "citihub/probr-probe")
	SetVar(&e.ServicePacks.Kubernetes.ContainerRequiredDropCapabilities, "PROBR_REQUIRED_DROP_CAPABILITIES", []string{"NET_RAW"})
	SetVar(&e.ServicePacks.Kubernetes.ContainerAllowedAddCapabilities, "PROBR_ALLOWED_ADD_CAPABILITIES", []string{""})
	SetVar(&e.ServicePacks.Kubernetes.ApprovedVolumeTypes, "PROBR_APPROVED_VOLUME_TYPES", []string{"configmap", "emptydir", "persistentvolumeclaim"})
	SetVar(&e.ServicePacks.Kubernetes.UnapprovedHostPort, "PROBR_UNAPPROVED_HOSTPORT", "22")
	SetVar(&e.ServicePacks.Kubernetes.SystemNamespace, "PROBR_K8S_SYSTEM_NAMESPACE", "kube-system")
	SetVar(&e.ServicePacks.Kubernetes.DashboardPodNamePrefix, "PROBR_K8S_DASHBOARD_PODNAMEPREFIX", "kubernetes-dashboard")
	SetVar(&e.ServicePacks.Kubernetes.ProbeNamespace, "PROBR_K8S_PROBE_NAMESPACE", "probr-general-test-ns")
	SetVar(&e.ServicePacks.Kubernetes.Azure.DefaultNamespaceAIB, "DEFAULT_NS_AZURE_IDENTITY_BINDING", "probr-aib")
	SetVar(&e.ServicePacks.Kubernetes.Azure.IdentityNamespace, "PROBR_K8S_AZURE_IDENTITY_NAMESPACE", "kube-system")

	SetVar(&e.CloudProviders.Azure.TenantID, "AZURE_TENANT_ID", "")
	SetVar(&e.CloudProviders.Azure.SubscriptionID, "AZURE_SUBSCRIPTION_ID", "")
	SetVar(&e.CloudProviders.Azure.ClientID, "AZURE_CLIENT_ID", "")
	SetVar(&e.CloudProviders.Azure.ClientSecret, "AZURE_CLIENT_SECRET", "")
	SetVar(&e.CloudProviders.Azure.ResourceGroup, "AZURE_RESOURCE_GROUP", "")
	SetVar(&e.CloudProviders.Azure.ResourceLocation, "AZURE_RESOURCE_LOCATION", "")
}

func getDefaultKubeConfigPath() string {
	return filepath.Join(homeDir(), ".kube", "config")
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

// set fetches the env var or sets the default value as needed for the specified field from VarOptions
func SetVar(field interface{}, varName string, defaultValue interface{}) {
	switch v := field.(type) {
	default:
		log.Fatalf("Unexpected value type provided for '%v', should be %T", varName, v)
	case *string:
		setStringVar(field.(*string), varName, defaultValue.(string))
	case *[]string:
		setStringSliceVar(field.(*[]string), varName, defaultValue.([]string))
	}
}

func setStringVar(field *string, varName string, defaultValue string) {
	value := *field
	if value == "" { // if field was empty, get value from env var
		value = os.Getenv(varName)
	}
	if value == "" { // if still empty, use default value provided
		value = defaultValue
	}
	field = &value
}

func setStringSliceVar(field *[]string, varName string, defaultValue []string) {
	value := *field
	if len(value) == 0 { // if field was empty, get value from env var
		t := os.Getenv(varName) // for []string, env var should be comma separated values
		if len(t) > 0 {
			value = append(value, strings.Split(t, ",")...)
		}
	}
	if len(value) == 0 { // if still empty, use default value provided
		value = defaultValue
	}
	field = &value
}
