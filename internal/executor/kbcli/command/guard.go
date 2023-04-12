package command

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/utils/strings/slices"
)

// Resource represents a Kubernetes resource.
type Resource struct {
	// Name is always plural, e.g. "pods".
	Name       string
	Namespaced bool
}

// K8sDiscoveryInterface describes an interface for getting K8s server resources.
type K8sDiscoveryInterface interface {
	ServerPreferredResources() ([]*v1.APIResourceList, error)
}

// CommandGuard is responsible for getting allowed resources for a given command.
type CommandGuard struct {
	log          logrus.FieldLogger
	discoveryCli K8sDiscoveryInterface
}

var (
	// ErrCmdNotSupported is returned when the verb is not supported for the resource.
	ErrCmdNotSupported = errors.New("command not supported")

	// ErrResourceNotFound is returned when the resource is not found on the server.
	ErrResourceNotFound = errors.New("resource not found")

	// unsupportedGlobalCmds contains cmds which are not supported for interactive operations.
	unsupportedGlobalCmds = map[string]struct{}{
		"playground": {},
		"bench":      {},
	}

	// command resource map
	cmdResource = map[string]Resource{
		"cluster": {
			Name:       "cluster",
			Namespaced: true,
		},
	}

	cmdVerbs = map[string][]string{
		"cluster": {
			// Basic Cluster Commands:
			"create",          // Create a cluster.
			"connect",         // 	Connect to a cluster or instance.
			"describe",        // 	Show details of a specific cluster.
			"list",            // 	List clusters.
			"list-instances",  // 	List cluster instances.
			"list-components", // 	List cluster components.
			"list-events",     // 	List cluster events.
			"list-accounts",   // 	List cluster accounts.
			"delete",          // 	Delete clusters.

			// Cluster Operation Commands:
			"update",             // 	Update the cluster settings, such as enable or disable monitor or log.
			"restart",            // 	Restart the specified components in the cluster.
			"upgrade",            // 	Upgrade the cluster version.
			"volume-expand",      // 	Expand volume with the specified components and volumeClaimTemplates in the cluster.
			"vscale",             // 	Vertically scale the specified components in the cluster.
			"hscale",             // 	Horizontally scale the specified components in the cluster.
			"describe-ops",       // 	Show details of a specific OpsRequest.
			"list-ops",           // 	List all opsRequests.
			"delete-ops",         // 	Delete an OpsRequest.
			"configure",          // 	Reconfigure parameters with the specified components in the cluster.
			"expose",             // 	Expose a cluster.
			"describe-configure", // 	Show details of a specific reconfiguring.
			"explain-configure",  // 	List the constraint for supported configuration params.
			"diff-configure",     // 	Show the difference in parameters between the two submitted OpsRequest.
			"stop",               // 	Stop the cluster and release all the pods of the cluster.
			"start",              // 	Start the cluster if cluster is stopped.

			// Backup/Restore Commands:
			"backup",         // 	Create a backup.
			"list-backups",   // 	List backups.
			"delete-backup",  // 	Delete a backup.
			"restore",        // 	Restore a new cluster from backup.
			"list-restores",  // 	List all restore jobs.
			"delete-restore", // 	Delete a restore job.

			// Troubleshooting Commands:
			"logs",      // 	Access cluster log file.
			"list-logs", // 	List supported log files in cluster.},
		},

		"kubeblocks": {
			"install",       //	Install KubeBlocks.
			"list-versions", //	List KubeBlocks versions.
			"preflight",     //	Run and retrieve preflight checks for KubeBlocks.
			"status",        //	Show list of resource KubeBlocks uses or owns.
			"uninstall",     //	Uninstall KubeBlocks.
			"upgrade",       //	Upgrade KubeBlocks.
		},

		"clusterdefinition": {
			"list",
		},

		"clusterversion": {
			"list",
		},
	}
)

// NewCommandGuard creates a new CommandGuard instance.
func NewCommandGuard(log logrus.FieldLogger, discoveryCli K8sDiscoveryInterface) *CommandGuard {
	return &CommandGuard{log: log, discoveryCli: discoveryCli}
}

// FilterSupportedCmds filters out unsupported verbs by the interactive commands.
func (g *CommandGuard) FilterSupportedCmds(allVerbs []string) []string {
	return slices.Filter(nil, allVerbs, func(s string) bool {
		_, exists := unsupportedGlobalCmds[s]
		return !exists
	})
}

// GetAllowedVerbsForCmd returns a list of allowed verbs for a given cmd.
func (g *CommandGuard) GetAllowedVerbsForCmd(cmd string, verbs []string) ([]string, error) {
	verbs, ok := cmdVerbs[cmd]
	if !ok {
		return nil, nil
	}
	return verbs, nil
}

// GetResourceDetails returns a Resource struct for a given resource type and verb.
func (g *CommandGuard) GetResourceDetails(cmd string) (Resource, error) {
	res, ok := cmdResource[cmd]
	if !ok {
		return Resource{}, nil
	}
	return res, nil
}

// GetServerResourceMap returns a map of all resources available on the server.
// LIMITATION: This method ignores second occurrences of the same resource name.
func (g *CommandGuard) GetServerResourceMap() (map[string]v1.APIResource, error) {
	resList, err := g.discoveryCli.ServerPreferredResources()
	if err != nil {
		if !shouldIgnoreResourceListError(err) {
			return nil, fmt.Errorf("while getting resource list from K8s cluster: %w", err)
		}

		g.log.Warnf("Ignoring error while getting resource list from K8s cluster: %s", err.Error())
	}

	resourceMap := make(map[string]v1.APIResource)
	for _, item := range resList {
		for _, res := range item.APIResources {
			// TODO: Cmds should be provided with full group version to avoid collisions in names.
			// 	For example, "pods" and "nodes" are both in "v1" and "metrics.k8s.io/v1beta1".
			// 	Ignoring second occurrence for now.
			if _, exists := resourceMap[res.Name]; exists {
				g.log.Debugf("Skipping resource with the same name %q (%q)...", res.Name, item.GroupVersion)
				continue
			}

			resourceMap[res.Name] = res
		}
	}

	return resourceMap, nil
}

// GetResourceDetailsFromMap returns a Resource struct for a given resource type and verb based on the server resource map.
func (g *CommandGuard) GetResourceDetailsFromMap(resourceType string, resMap map[string]v1.APIResource) (Resource, error) {
	res, exists := resMap[resourceType]
	if !exists {
		return Resource{}, ErrResourceNotFound
	}

	return Resource{
		Name:       res.Name,
		Namespaced: res.Namespaced,
	}, nil
}

// shouldIgnoreResourceListError returns true if the error should be ignored. This is a workaround for client-go behavior,
// which reports error on empty resource lists. However, some components can register empty lists for their resources.
// See
// See: https://github.com/kyverno/kyverno/issues/2267
func shouldIgnoreResourceListError(err error) bool {
	groupDiscoFailedErr, ok := err.(*discovery.ErrGroupDiscoveryFailed)
	if !ok {
		return false
	}

	for _, currentErr := range groupDiscoFailedErr.Groups {
		// Unfortunately there isn't a nicer way to do this.
		// See https://github.com/kubernetes/client-go/blob/release-1.25/discovery/cached/memory/memcache.go#L228
		if strings.Contains(currentErr.Error(), "Got empty response for") {
			// ignore it as it isn't necessarily an error
			continue
		}

		return false
	}

	return true
}

func (g *CommandGuard) GetResourceTypeForCmd(cmd string) string {
	resource, ok := cmdResource[cmd]
	if !ok {
		return ""
	}
	return resource.Name
}
