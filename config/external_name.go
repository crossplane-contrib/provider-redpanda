package config

import (
	"github.com/crossplane/upjet/v2/pkg/config"
)

var terraformPluginFrameworkExternalNameConfigs = map[string]config.ExternalName{
	"redpanda_acl":                 config.IdentifierFromProvider,
	"redpanda_cluster":             config.IdentifierFromProvider,
	"redpanda_network":             config.IdentifierFromProvider,
	"redpanda_resource_group":      config.IdentifierFromProvider,
	"redpanda_role_assignment":     config.IdentifierFromProvider,
	"redpanda_schema":              config.IdentifierFromProvider,
	"redpanda_schema_registry_acl": config.IdentifierFromProvider,
	"redpanda_serverless_cluster":  config.IdentifierFromProvider,
	"redpanda_topic":               config.IdentifierFromProvider,
	"redpanda_user":                config.IdentifierFromProvider,
}

// ExternalNameConfigurations applies all external name configs listed in the
// table ExternalNameConfigs and sets the version of those resources to v1beta1
// assuming they will be tested.
func ExternalNameConfigurations() config.ResourceOption {
	return func(r *config.Resource) {
		if e, ok := terraformPluginFrameworkExternalNameConfigs[r.Name]; ok {
			r.ExternalName = e
		}
	}
}

// ExternalNameConfigured returns the list of all resources whose external name
// is configured manually.
func ExternalNameConfigured() []string {
	l := make([]string, len(terraformPluginFrameworkExternalNameConfigs))
	i := 0
	for name := range terraformPluginFrameworkExternalNameConfigs {
		// $ is added to match the exact string since the format is regex.
		l[i] = name + "$"
		i++
	}
	return l
}
