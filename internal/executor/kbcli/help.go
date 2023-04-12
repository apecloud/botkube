package kbcli

import (
	"github.com/MakeNowJust/heredoc"
)

func help() string {
	return heredoc.Doc(`
		=============================================
		 __    __ _______   ______  __       ______ 
		|  \  /  \       \ /      \|  \     |      \
		| ▓▓ /  ▓▓ ▓▓▓▓▓▓▓\  ▓▓▓▓▓▓\ ▓▓      \▓▓▓▓▓▓
		| ▓▓/  ▓▓| ▓▓__/ ▓▓ ▓▓   \▓▓ ▓▓       | ▓▓  
		| ▓▓  ▓▓ | ▓▓    ▓▓ ▓▓     | ▓▓       | ▓▓  
		| ▓▓▓▓▓\ | ▓▓▓▓▓▓▓\ ▓▓   __| ▓▓       | ▓▓  
		| ▓▓ \▓▓\| ▓▓__/ ▓▓ ▓▓__/  \ ▓▓_____ _| ▓▓_ 
		| ▓▓  \▓▓\ ▓▓    ▓▓\▓▓    ▓▓ ▓▓     \   ▓▓ \
		 \▓▓   \▓▓\▓▓▓▓▓▓▓  \▓▓▓▓▓▓ \▓▓▓▓▓▓▓▓\▓▓▓▓▓▓
		
		=============================================
		A Command Line Interface for KubeBlocks
		
		Usage:
		  kbcli [flags] [options]

		Available Commands:
		  addon               Addon command.
		  alert               Manage alert receiver, include add, list and delete receiver.
		  backup-config       KubeBlocks backup config.
		  bench               Run a benchmark.
		  cluster             Cluster command.
		  clusterdefinition   ClusterDefinition command.
		  clusterversion      ClusterVersion command.
		  completion          Generate the autocompletion script for the specified shell
		  dashboard           List and open the KubeBlocks dashboards.
		  kubeblocks          KubeBlocks operation commands.
		  playground          Bootstrap a playground KubeBlocks in local host or cloud.
		  version             Print the version information, include kubernetes, KubeBlocks and kbcli version.
	`)
}
