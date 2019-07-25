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
	"github.com/kubernetes-incubator/descheduler/pkg/api"
	"github.com/kubernetes-incubator/descheduler/test"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/kubernetes/pkg/scheduler/factory"
	"strings"
	"testing"
)

func TestBinPacking(t *testing.T) {
	n_lo := test.BuildTestNode("n_lo", 1000, 3000, 10)
	n_mid := test.BuildTestNode("n_mid", 1000, 3000, 10)
	n_hi := test.BuildTestNode("n_hi", 1000, 3000, 10)
	p1 := test.BuildTestPod("p1", 300, 0, n_lo.Name)
	p2 := test.BuildTestPod("p2", 300, 0, n_mid.Name)
	p3 := test.BuildTestPod("p3", 300, 0, n_mid.Name)
	p4 := test.BuildTestPod("p4", 300, 0, n_hi.Name)
	p5 := test.BuildTestPod("p5", 300, 0, n_hi.Name)
	p6 := test.BuildTestPod("p6", 300, 0, n_hi.Name)
	ownerRef := test.GetReplicaSetOwnerRefList()
	p1.ObjectMeta.OwnerReferences = ownerRef
	p2.ObjectMeta.OwnerReferences = ownerRef
	p3.ObjectMeta.OwnerReferences = ownerRef
	p4.ObjectMeta.OwnerReferences = ownerRef
	p5.ObjectMeta.OwnerReferences = ownerRef
	p6.ObjectMeta.OwnerReferences = ownerRef
	// set threshold
	var thresholds = make(api.ResourceThresholds)
	thresholds[v1.ResourceCPU] = 70
	thresholds[v1.ResourcePods] = 20
	// create fake client
	fakeClient := &fake.Clientset{}
	fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
		list := action.(core.ListAction)
		fieldString := list.GetListRestrictions().Fields.String()
		if strings.Contains(fieldString, n_lo.Name) {
			return true, &v1.PodList{Items: []v1.Pod{*p1}}, nil
		}
		if strings.Contains(fieldString, n_mid.Name) {
			return true, &v1.PodList{Items: []v1.Pod{*p2, *p3}}, nil
		}
		if strings.Contains(fieldString, n_hi.Name) {
			return true, &v1.PodList{Items: []v1.Pod{*p4, *p5, *p6}}, nil
		}
		return true, nil, fmt.Errorf("Failed to list: %v", list)
	})
	fakeClient.Fake.AddReactor("get", "nodes", func(action core.Action) (bool, runtime.Object, error) {
		getAction := action.(core.GetAction)
		switch getAction.GetName() {
		case n_lo.Name:
			return true, n_lo, nil
		case n_mid.Name:
			return true, n_mid, nil
		case n_hi.Name:
			return true, n_hi, nil
		}
		return true, nil, fmt.Errorf("Wrong node: %v", getAction.GetName())
	})
	nodes := []*v1.Node{n_mid, n_lo, n_hi}
	npm := createNodePodsMap(fakeClient, nodes)
	evictLocalStoragePods, dryRun := false, false
	// Try to find the node with the lowest utilization
	if node := findNodeUnderUtilization(npm, thresholds, evictLocalStoragePods); node.Name != n_lo.Name {
		glog.Fatalf("Failed: unit-test for find nodes with the lowest utilization")
	}
	// Try to add most requested as a priority function
	if pods, err := mostRequested(fakeClient, n_hi, dryRun); len(pods) != 3 || err != nil || len(factory.ListRegisteredPriorityFunctions()) != 1 {
		glog.Fatalf("Failed: unit-test for registering most requested priority function")
	}
	// full test to evict all pods on n_lo
	if err := binPacking(fakeClient, "v1", nodes, thresholds, dryRun, evictLocalStoragePods); err != nil {
		glog.Fatalf("Failed: unit-test for evicting all pods on node with lowest utilization failed. %v", err)
	}
}
