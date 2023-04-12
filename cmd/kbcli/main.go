/*
Copyright ApeCloud, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"github.com/hashicorp/go-plugin"

	"github.com/kubeshop/botkube/internal/executor/kbcli"
	"github.com/kubeshop/botkube/pkg/api/executor"
)

// version is set via ldflags by GoReleaser.
var version = "dev"

func main() {
	executor.Serve(map[string]plugin.Plugin{
		kbcli.PluginName: &executor.Plugin{
			Executor: kbcli.NewExecutor(version, kbcli.NewBinaryRunner(), kbcli.NewBinaryRunner()),
		},
	})
}
