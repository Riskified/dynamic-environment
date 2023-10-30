package names

const (
	DynamicEnvLabel             = "dynamic-env"
	DefaultVersionLabel         = "version"
	DefaultVersion              = "shared"
	DeleteDeployments           = "DeleteDeployments"
	DeleteDestinationRules      = "DeleteDestinationRules"
	CleanupVirtualServices      = "CleanupVirtualServices"
	VirtualServiceRoutePrefix   = "dynamic-environment"
	MainServiceLabelKey         = "purpose"
	MainServiceLabelValue       = "main"
	DynamicEnvHeadersLabelKey   = "dynamic-env-headers"
	DynamicEnvHeadersLabelValue = "true"
	IstioSideCarName            = "istio-proxy"
	IstioSideCarImage           = "auto"
	IstioSideCarHeaderEnvName   = "EXACT_HEADERS_SERIALIZED"
)
