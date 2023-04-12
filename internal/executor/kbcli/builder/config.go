package builder

type (
	// Config holds kbcli builder configuration parameters.
	Config struct {
		Allowed AllowedResources `yaml:"allowed,omitempty"`
	}

	// AllowedResources describes interactive builder "building blocks". It's needed to populate dropdowns with proper values.
	AllowedResources struct {
		// Namespaces if not specified, builder needs to have proper permissions to list all namespaces in the cluster.
		Namespaces []string `yaml:"namespaces,omitempty"`
		// Cmds holds allowed sub-commands, at least one sub-command MUST be specified.
		Cmds []string `yaml:"resources,omitempty"`
		// Verbs holds allowed verbs.
		Verbs []string `yaml:"verbs,omitempty"`
	}
)

// DefaultConfig returns default configuration for command builder.
func DefaultConfig() Config {
	return Config{
		Allowed: AllowedResources{
			Cmds: []string{
				"cluster", "kubeblocks", "clusterdefinition", "clusterversion", "playground",
			},
			Verbs: []string{
				"list",
			},
		},
	}
}
