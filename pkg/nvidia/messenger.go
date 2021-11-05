/**
 * Copyright (2021, ) Institute of Software, Chinese Academy of Sciences
 **/

package nvidia

import (
	"encoding/json"
	"errors"
	"github.com/kubesys/kubernetes-client-go/pkg/kubesys"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// KubeMessenger is used to communicate with api server
type KubeMessenger struct {
	client   *kubesys.KubernetesClient
	nodeName string
}

func NewKubeMessenger(client *kubesys.KubernetesClient, nodeName string) *KubeMessenger {
	return &KubeMessenger{
		client:   client,
		nodeName: nodeName,
	}
}

func (m *KubeMessenger) getNode() *v1.Node {
	node, err := m.client.GetResource("Node", "", m.nodeName)
	if err != nil {
		return nil
	}
	var out v1.Node
	err = json.Unmarshal(node, &out)
	if err != nil {
		return nil
	}
	return &out
}

func (m *KubeMessenger) PatchGPUCount(count, core uint) error {
	node := m.getNode()
	if node == nil {
		log.Warningln("Failed to get node.")
		return errors.New("node not found")
	}

	newNode := node.DeepCopy()
	newNode.Status.Capacity[ResourceCount] = *resource.NewQuantity(int64(count), resource.DecimalSI)
	newNode.Status.Allocatable[ResourceCount] = *resource.NewQuantity(int64(count), resource.DecimalSI)
	newNode.Status.Capacity[ResourceCore] = *resource.NewQuantity(int64(core), resource.DecimalSI)
	newNode.Status.Allocatable[ResourceCore] = *resource.NewQuantity(int64(core), resource.DecimalSI)

	err := m.updateNodeStatus(newNode)
	if err != nil {
		log.Warningln("Failed to update Capacity gpu-count %s and core %s.", ResourceCount, ResourceCore)
		return errors.New("patch node status fail")
	}

	return nil
}

func (m* KubeMessenger) updateNodeStatus(node *v1.Node) error {
	nodeJson, err := json.Marshal(node)
	if err != nil {
		return err
	}
	_, err = m.client.UpdateResourceStatus(string(nodeJson))
	if err != nil {
		return err
	}
	return nil
}

func (m *KubeMessenger) GetPendingPodsOnNode() []*v1.Pod {
	var pods []*v1.Pod
	podList, _ := m.client.ListResources("Pod", "")
	var podListObject v1.PodList
	json.Unmarshal(podList, &podListObject)
	for _, pod := range podListObject.Items {
		if pod.Spec.NodeName == m.nodeName && pod.Status.Phase == "Pending" {
			podCopy := pod.DeepCopy()
			podCopy.Kind = "Pod"
			podCopy.APIVersion = "v1"
			pods = append(pods, podCopy)
		}
	}
	return pods
}

func (m *KubeMessenger) UpdatePodAnnotations(pod *v1.Pod) error {
	podJson, err := json.Marshal(pod)
	if err != nil {
		return err
	}
	_, err = m.client.UpdateResource(string(podJson))
	if err != nil {
		return err
	}
	return nil
}

func (m *KubeMessenger) GetPodOnNode(podName, namespace string) *v1.Pod {
	pod, err := m.client.GetResource("Pod", namespace, podName)
	if err != nil {
		return nil
	}
	var out v1.Pod
	err = json.Unmarshal(pod, &out)
	if err != nil {
		return nil
	}
	return &out
}