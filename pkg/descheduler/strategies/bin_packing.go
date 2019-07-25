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
	"fmt"
	"github.com/golang/glog"
	"github.com/kubernetes-incubator/descheduler/cmd/descheduler/app/options"
	"github.com/kubernetes-incubator/descheduler/pkg/api"
	"github.com/kubernetes-incubator/descheduler/pkg/descheduler/evictions"
	podutil "github.com/kubernetes-incubator/descheduler/pkg/descheduler/pod"
	"k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/pkg/scheduler/algorithm/priorities"
	"k8s.io/kubernetes/pkg/scheduler/factory"
	taintutils "k8s.io/kubernetes/pkg/util/taints"
	priorities2 "k8s.io/kubernetes/plugin/pkg/scheduler/algorithm/priorities"
)

func RemovePodsForBinPacking(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, policyGroupVersion string, nodes []*v1.Node, nodepodCount nodePodEvictedCount) {
	if !strategy.Enabled {
		return
	}
	if err := binPacking(ds.Client, policyGroupVersion, nodes, strategy.Params.NodeResourceUtilizationThresholds.Thresholds, ds.DryRun, ds.EvictLocalStoragePods); err != nil {
		glog.Errorf("Bin packing failed. %v", err)
	}
}

// Find the least utilized node under the threshold and evict all the pods on it.
func binPacking(clientSet clientset.Interface, policyGroupVersion string, nodes []*v1.Node, threshold api.ResourceThresholds, dryRun bool, evictLocalStoragePods bool) error {
	// find least utilized node
	npm := createNodePodsMap(clientSet, nodes)
	nodeToEmpty := findNodeUnderUtilization(npm, threshold, evictLocalStoragePods)
	if nodeToEmpty == nil {
		glog.V(1).Infof("No node is underutilized, nothing to do here, you might want to lower your thresholds")
		return nil
	}
	// turn on most requested priority
	podsToEvict, err := mostRequested(clientSet, nodeToEmpty, evictLocalStoragePods)
	if err != nil {
		return fmt.Errorf("Error making most requested as a priority.")
	}
	// evict all the pods on that node
	for _, pod := range podsToEvict {
		succeed, err := evictions.EvictPod(clientSet, pod, policyGroupVersion, dryRun)
		if !succeed {
			return fmt.Errorf("Error when evicting pod: %v %v", pod.Name, err)
		}
	}
	return nil
}

func findNodeUnderUtilization(npm NodePodsMap, threshold api.ResourceThresholds, evictLocalStoragePods bool) *v1.Node {
	var leastUtilized *v1.Node
	for node, pods := range npm {

		usage, _, _, _, _, _ := NodeUtilization(node, pods, evictLocalStoragePods)
		if !isTainedMasterNode(node) && IsNodeWithLowUtilization(usage, threshold) {
			threshold = usage
			leastUtilized = node
		}
	}
	if (leastUtilized == &v1.Node{}) {
		return nil
	}
	return leastUtilized
}

func mostRequested(clientSet clientset.Interface, node *v1.Node, dryRun bool) ([]*v1.Pod, error) {
	pods, err := podutil.ListPodsOnANode(clientSet, node)
	if err != nil {
		return nil, fmt.Errorf("Error list evictable pods %v", err)
	}
	priorities2.MostRequestedPriorityMap()
	factory.RegisterPriorityFunction2(priorities.MostRequestedPriority, priorities.MostRequestedPriorityMap, nil, 1)
	if !factory.IsPriorityFunctionRegistered(priorities.MostRequestedPriority){
		glog.Errorf("Priority function registration failed.")
	}
	return pods, nil
}

// Distinguish Master Node by Taint "node-role.kubernetes.io/master=NoSchedule". Since without the taint, Master node will be scheduable anyway.
func isTainedMasterNode(node *v1.Node) bool {
	taint := &v1.Taint{
		Key:    kubeadmconstants.LabelNodeRoleMaster,
		Effect: v1.TaintEffectNoSchedule,
	}
	return taintutils.TaintExists(node.Spec.Taints, taint)
}
