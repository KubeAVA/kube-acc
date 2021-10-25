/**
 * Copyright (2021, ) Institute of Software, Chinese Academy of Sciences
 **/

package node_daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/NVIDIA/gpu-monitoring-tools/bindings/go/nvml"
	v1 "github.com/kubesys/kube-alloc/pkg/apis/doslab.io/v1"
	jsonObj "github.com/kubesys/kubernetes-client-go/pkg/json"
	"github.com/kubesys/kubernetes-client-go/pkg/kubesys"
	log "github.com/sirupsen/logrus"
	"io"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type NodeDaemon struct {
	Client         *kubesys.KubernetesClient
	PodMgr         *PodManager
	NodeName       string
	PortMap        map[int]bool
	PortUseByPod   map[string]int
	PodVisited     map[string]bool
	GpuNameByUuid  map[string]string
	GemSchedulerIp string
	mu             sync.Mutex
}

func NewNodeDaemon(client *kubesys.KubernetesClient, podMgr *PodManager, nodeName string) *NodeDaemon {
	portMap := make(map[int]bool)
	for i := GemSchedulerGPUPodManagerPortStart; i <= GemSchedulerGPUPodManagerPortEnd; i++ {
		portMap[i] = false
	}
	return &NodeDaemon{
		Client:        client,
		PodMgr:        podMgr,
		NodeName:      nodeName,
		PortMap:       portMap,
		PortUseByPod:  make(map[string]int),
		PodVisited:    make(map[string]bool),
		GpuNameByUuid: make(map[string]string),
	}
}

func (daemon *NodeDaemon) Run(hostname string) {
	err := os.MkdirAll(GemSchedulerGPUConfigPath, os.ModePerm)
	if err != nil {
		log.Fatalf("Failed to create fir %s, %s.", GemSchedulerGPUConfigPath, err)
	}
	err = os.MkdirAll(GemSchedulerGPUPodManagerPortPath, os.ModePerm)
	if err != nil {
		log.Fatalf("Failed to create fir %s, %s.", GemSchedulerGPUPodManagerPortPath, err)
	}

	f, err := os.Create(GemSchedulerIpPath)
	if err != nil {
		log.Fatalf("Failed to create file %s, %s.", GemSchedulerIpPath, err)
	}
	ip := os.Getenv(EnvGemSchedulerIp)
	if ip == "" {
		log.Fatalf("Failed to get env GemSchedulerIp, %s.", err)
	}
	daemon.GemSchedulerIp = ip
	f.WriteString(ip + "\n")
	f.Sync()
	f.Close()

	n, err := nvml.GetDeviceCount()
	if err != nil {
		log.Fatalf("Failed to get device count, %s.", err)
	}

	for index := uint(0); index < n; index++ {
		device, err := nvml.NewDevice(index)
		if err != nil {
			log.Fatalf("Failed to new device, %s.", err)
		}

		_, err = os.Create(GemSchedulerGPUConfigPath + device.UUID)
		if err != nil {
			log.Fatalf("Failed to create file %s, %s", GemSchedulerGPUConfigPath+device.UUID, err)
		}

		_, err = os.Create(GemSchedulerGPUPodManagerPortPath + device.UUID)
		if err != nil {
			log.Fatalf("Failed to create file %s, %s", GemSchedulerGPUPodManagerPortPath+device.UUID, err)
		}

		gpu := v1.GPU{
			TypeMeta: metav1.TypeMeta{
				Kind:       "GPU",
				APIVersion: GPUCRDAPIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-gpu-%d", hostname, index),
				Namespace: GPUCRDNamespace,
			},
			Spec: v1.GPUSpec{
				UUID:   device.UUID,
				Model:  *device.Model,
				Family: getArchFamily(*device.CudaComputeCapability.Major, *device.CudaComputeCapability.Minor),
				Capacity: v1.R{
					Core:   "100",
					Memory: strconv.Itoa(int(*device.Memory)),
				},
				Node: hostname,
			},
			Status: v1.GPUStatus{
				Allocated: v1.R{
					Core:   "0",
					Memory: "0",
				},
			},
		}
		jb, err := json.Marshal(gpu)
		if err != nil {
			log.Fatalf("Failed to marshal gpu struct, %s.", err)
		}
		_, err = daemon.Client.CreateResource(string(jb))
		if err != nil && err.Error() != "request status 201 Created" {
			log.Fatalf("Failed to create gpu %s, %s.", gpu.Name, err)
		}
		daemon.GpuNameByUuid[device.UUID] = fmt.Sprintf("%s-gpu-%d", hostname, index)
	}

	for {
		if daemon.PodMgr.queueOfModified.Len() > 0 {
			daemon.PodMgr.muOfModify.Lock()
			pod := daemon.PodMgr.queueOfModified.Remove()
			daemon.PodMgr.muOfModify.Unlock()
			time.Sleep(5 * time.Millisecond)
			go daemon.modifyPod(pod)
		}

		if daemon.PodMgr.queueOfDeleted.Len() > 0 {
			daemon.PodMgr.muOfDelete.Lock()
			pod := daemon.PodMgr.queueOfDeleted.Remove()
			daemon.PodMgr.muOfDelete.Unlock()
			time.Sleep(5 * time.Millisecond)
			go daemon.deletePod(pod)
		}
	}
}

func (daemon *NodeDaemon) Listen(podMgr *PodManager) {
	podWatcher := kubesys.NewKubernetesWatcher(daemon.Client, podMgr)
	go daemon.Client.WatchResources("Pod", "", podWatcher)
}

func (daemon *NodeDaemon) modifyPod(pod *jsonObj.JsonObject) {
	meta := pod.GetJsonObject("metadata")
	if !meta.HasKey("annotations") {
		return
	}
	annotations := meta.GetJsonObject("annotations")
	if !annotations.HasKey(AnnAssumeTime) {
		return
	}

	podName, err := meta.GetString("name")
	if err != nil {
		log.Fatalln("Failed to get pod name.")
	}
	namespace, err := meta.GetString("namespace")
	if err != nil {
		log.Fatalln("Failed to get pod namespace.")
	}

	daemon.mu.Lock()
	if daemon.PodVisited[namespace+"/"+podName] {
		daemon.mu.Unlock()
		return
	}
	daemon.PodVisited[namespace+"/"+podName] = true
	daemon.mu.Unlock()

	var str1 []string
	str1 = append(str1, namespace+"/"+podName)

	spec := pod.GetJsonObject("spec")
	requestMemory, requestCore := int64(0), int64(0)
	containers := spec.GetJsonArray("containers")
	for _, c := range containers.Values() {
		container := c.JsonObject()
		if !container.HasKey("resources") {
			continue
		}
		resources := container.GetJsonObject("resources")
		if !resources.HasKey("limits") {
			continue
		}
		limits := resources.GetJsonObject("limits")
		if val, err := limits.GetString(ResourceMemory); err == nil {
			m, _ := strconv.ParseInt(val, 10, 64)
			requestMemory += m
		}
		if val, err := limits.GetString(ResourceCore); err == nil {
			m, _ := strconv.ParseInt(val, 10, 64)
			requestCore += m
		}
	}

	if requestCore != 0 {
		str1 = append(str1, strconv.FormatFloat(float64(requestCore)/100, 'f', 6, 64))
		str1 = append(str1, strconv.FormatFloat(float64(requestCore)/100, 'f', 6, 64))
	}
	if requestMemory != 0 {
		str1 = append(str1, strconv.FormatInt(1024*1024*requestMemory, 10))
	}
	str1[len(str1)-1] += "\n"

	fmt.Println(str1)

	gpu, err := annotations.GetString(ResourceUUID)
	if err != nil {
		log.Fatalln("Failed to get gpu uuid.")
	}

	// Update gem-gpu-config file
	err = daemon.updateFile(str1, GemSchedulerGPUConfigPath, gpu)
	if err != nil {
		log.Fatalf("Failed to update gem-gpu-config file, %s.", err)
	}

	daemon.mu.Lock()
	port := 0
	for i := GemSchedulerGPUPodManagerPortStart; i <= GemSchedulerGPUPodManagerPortEnd; i++ {
		if !daemon.PortMap[i] {
			port = i
			daemon.PortMap[i] = true
			daemon.PortUseByPod[namespace+"/"+podName] = i
			break
		}
	}
	daemon.mu.Unlock()
	if port == 0 {
		log.Warningf("There is no enough port for pod %s on ns %s, try later.", podName, namespace)
		daemon.mu.Lock()
		daemon.PodVisited[namespace+"/"+podName] = false
		daemon.mu.Unlock()
		daemon.PodMgr.muOfModify.Lock()
		daemon.PodMgr.queueOfModified.Add(pod)
		daemon.PodMgr.muOfModify.Unlock()
	}

	var str2 []string
	str2 = append(str2, namespace+"/"+podName)
	str2 = append(str2, strconv.Itoa(port))
	str2[len(str2)-1] += "\n"

	// Update gem-gpu-pod-manager-port file
	err = daemon.updateFile(str2, GemSchedulerGPUPodManagerPortPath, gpu)
	if err != nil {
		log.Fatalf("Failed to update gem-gpu-port file, %s.", err)
	}

	time.Sleep(time.Second)
	copyPodBytes, err := daemon.Client.GetResource("Pod", namespace, podName)
	if err != nil {
		log.Fatalf("Failed to get copy pod %s on ns %s, %s.", podName, namespace, err)
	}
	copyPod := kubesys.ToJsonObject(copyPodBytes)
	copyMeta := copyPod.GetJsonObject("metadata")
	copyAnnotations := copyMeta.GetJsonObject("annotations")

	copyAnnotations.Put(AnnGemSchedulerIp, daemon.GemSchedulerIp)
	copyAnnotations.Put(AnnGemPodManagerPort, strconv.Itoa(port))
	copyMeta.Put("annotations", copyAnnotations.ToInterface())
	copyPod.Put("metadata", copyMeta.ToInterface())

	_, err = daemon.Client.UpdateResource(copyPod.ToString())
	if err != nil {
		log.Fatalf("Failed to set pod %s's annotations, %s.", podName, err)
	}
}

func (daemon *NodeDaemon) deletePod(pod *jsonObj.JsonObject) {
	meta := pod.GetJsonObject("metadata")
	if !meta.HasKey("annotations") {
		return
	}
	annotations := meta.GetJsonObject("annotations")
	if !annotations.HasKey(AnnAssumeTime) {
		return
	}

	podName, err := meta.GetString("name")
	if err != nil {
		log.Fatalln("Failed to get pod name.")
	}
	namespace, err := meta.GetString("namespace")
	if err != nil {
		log.Fatalln("Failed to get pod namespace.")
	}

	daemon.mu.Lock()
	if !daemon.PodVisited[namespace+"/"+podName] {
		daemon.mu.Unlock()
		return
	}
	daemon.PodVisited[namespace+"/"+podName] = false
	port := daemon.PortUseByPod[namespace+"/"+podName]
	daemon.PortMap[port] = false
	delete(daemon.PortUseByPod, namespace+"/"+podName)
	daemon.mu.Unlock()

	gpu, err := annotations.GetString(ResourceUUID)
	if err != nil {
		log.Fatalln("Failed to get gpu uuid.")
	}

	// Update gem-gpu-config file
	err = daemon.removeFile(namespace+"/"+podName, GemSchedulerGPUConfigPath, gpu)
	if err != nil {
		log.Fatalf("Failed to remove gem-gpu-config file, %s.", err)
	}

	// Update gem-gpu-pod-manager-port file
	err = daemon.removeFile(namespace+"/"+podName, GemSchedulerGPUPodManagerPortPath, gpu)
	if err != nil {
		log.Fatalf("Failed to remove gem-gpu-port file, %s.", err)
	}

}

func (daemon *NodeDaemon) updateFile(str []string, dir, gpu string) error {
	fileName := dir + gpu
	f, err := os.OpenFile(fileName, os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	if err != nil {
		return err
	}

	lines := make(map[string][]string)
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		words := strings.Split(line, " ")
		if len(words) == 1 {
			continue
		}
		lines[words[0]] = words[1:]
	}
	lines[str[0]] = str[1:]

	f, err = os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(strconv.Itoa(len(lines)) + "\n")
	if err != nil {
		return err
	}
	for k, v := range lines {
		s := k
		for i := 0; i < len(v); i++ {
			s += " "
			s += v[i]
		}
		_, err := f.WriteString(s)
		if err != nil {
			return err
		}
	}

	f.Sync()

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	if err != nil {
		return err
	}

	log.Infof("Success to update file %s.", fileName)

	return nil
}

func (daemon *NodeDaemon) removeFile(pod, dir, gpu string) error {
	fileName := dir + gpu
	f, err := os.OpenFile(fileName, os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	if err != nil {
		return err
	}

	lines := make(map[string][]string)
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		words := strings.Split(line, " ")
		if len(words) == 1 {
			continue
		}
		lines[words[0]] = words[1:]
	}
	delete(lines, pod)

	f, err = os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(strconv.Itoa(len(lines)) + "\n")
	if err != nil {
		return err
	}

	for k, v := range lines {
		s := k
		for i := 0; i < len(v); i++ {
			s += " "
			s += v[i]
		}
		_, err := f.WriteString(s)
		if err != nil {
			return err
		}
	}

	f.Sync()

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	if err != nil {
		return err
	}

	log.Infof("Success to remove file %s.", fileName)

	return nil
}

func getArchFamily(computeMajor, computeMinor int) string {
	switch computeMajor {
	case 1:
		return "Tesla"
	case 2:
		return "Fermi"
	case 3:
		return "Kepler"
	case 5:
		return "Maxwell"
	case 6:
		return "Pascal"
	case 7:
		if computeMinor < 5 {
			return "volta"
		}
		return "Turing"
	case 8:
		return "Ampere"
	}
	return "Unknown"
}
