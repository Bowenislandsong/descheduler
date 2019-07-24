/*
Copyright 2017 The Kubernetes Authors.

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

package strategies

import (
	"github.com/golang/glog"
	"github.com/kubernetes-incubator/descheduler/cmd/descheduler/app/options"
	"github.com/kubernetes-incubator/descheduler/pkg/api"
	"github.com/kubernetes-incubator/descheduler/pkg/descheduler/evictions"
	nodeutil "github.com/kubernetes-incubator/descheduler/pkg/descheduler/node"
	"k8s.io/api/core/v1"
)

// Find the least utilized node under the threshold and evict all the pods on it.
func BinPacking(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, evictionPolicyGroupVersion string, nodes []*v1.Node, nodepodCount nodePodEvictedCount) {
	if !strategy.Enabled {
		return
	}
	// find least utilized node
	npm:=createNodePodsMap(ds.Client,nodes)
	nodeToEmpty := findNodeUnderUtilization(npm,strategy.Params.NodeResourceUtilizationThresholds.Thresholds,ds.EvictLocalStoragePods)
	// turn on most requested priority
	podsToEvict := mostRequested(nodeToEmpty)
	// evict all the pods on that node
	for _, pod := range podsToEvict{
		succeed, err:=evictions.EvictPod(ds.Client,pod,evictionPolicyGroupVersion,ds.DryRun)
		if !succeed {
			glog.Warningf("Error when evicting pod: %#v (%#v)", pod.Name, err)
		}
	}
}

func findNodeUnderUtilization(npm NodePodsMap, threshold api.ResourceThresholds, evictLocalStoragePods bool) *v1.Node {
	var leastUtilized *v1.Node
	for node, pods := range npm {
		usage, _, _, _, _, _ := NodeUtilization(node, pods, evictLocalStoragePods)
		if !nodeutil.IsNodeUschedulable(node) && IsNodeWithLowUtilization(usage, threshold) {
			threshold = usage
			leastUtilized = node
		}
	}
	if (leastUtilized == &v1.Node{}) {
		return nil
	}
	return leastUtilized
}

func mostRequested(node *v1.Node) [] *v1.Pod {

}
