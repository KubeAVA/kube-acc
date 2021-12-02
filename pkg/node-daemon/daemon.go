/**
 * Copyright (2021, ) Institute of Software, Chinese Academy of Sciences
 **/

package node_daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/NVIDIA/gpu-monitoring-tools/bindings/go/nvml"
	v1 "github.com/kubesys/kube-alloc/pkg/apis/doslab.io/v1"
	"github.com/kubesys/kube-alloc/pkg/apis/runtime/vcuda"
	jsonObj "github.com/kubesys/kubernetes-client-go/pkg/json"
	"github.com/kubesys/kubernetes-client-go/pkg/kubesys"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// #include <stdint.h>
// #include <sys/types.h>
// #include <sys/stat.h>
// #include <fcntl.h>
// #include <string.h>
// #include <sys/file.h>
// #include <time.h>
// #include <stdlib.h>
// #include <unistd.h>
//
// #ifndef NVML_DEVICE_PCI_BUS_ID_BUFFER_SIZE
// #define NVML_DEVICE_PCI_BUS_ID_BUFFER_SIZE 16
// #endif
//
// #ifndef FILENAME_MAX
// #define FILENAME_MAX 4096
// #endif
//
// struct version_t {
//  int major;
//  int minor;
// } __attribute__((packed, aligned(8)));
//
// struct resource_data_t {
//  char pod_uid[48];
//  int limit;
//  char occupied[4044];
//  char container_name[FILENAME_MAX];
//  char bus_id[NVML_DEVICE_PCI_BUS_ID_BUFFER_SIZE];
//  uint64_t gpu_memory;
//  int utilization;
//  int hard_limit;
//  struct version_t driver_version;
//  int enable;
// } __attribute__((packed, aligned(8)));
//
// int setting_to_disk(const char* filename, struct resource_data_t* data) {
//  int fd = 0;
//  int wsize = 0;
//  int ret = 0;
//
//  fd = open(filename, O_CREAT | O_TRUNC | O_WRONLY, 00777);
//  if (fd == -1) {
//    return 1;
//  }
//
//  wsize = (int)write(fd, (void*)data, sizeof(struct resource_data_t));
//  if (wsize != sizeof(struct resource_data_t)) {
//    ret = 2;
//	goto DONE;
//  }
//
// DONE:
//  close(fd);
//
//  return ret;
// }
//
// int pids_to_disk(const char* filename, int* data, int size) {
//  int fd = 0;
//  int wsize = 0;
//  struct timespec wait = {
//	.tv_sec = 0, .tv_nsec = 100 * 1000 * 1000,
//  };
//  int ret = 0;
//
//  fd = open(filename, O_CREAT | O_TRUNC | O_WRONLY, 00777);
//  if (fd == -1) {
//    return 1;
//  }
//
//  while (flock(fd, LOCK_EX)) {
//    nanosleep(&wait, NULL);
//  }
//
//  wsize = (int)write(fd, (void*)data, sizeof(int) * size);
//  if (wsize != sizeof(int) * size) {
//	ret = 2;
//    goto DONE;
//  }
//
// DONE:
//  flock(fd, LOCK_UN);
//  close(fd);
//
//  return ret;
// }
import "C"

type NodeDaemon struct {
	Client                *kubesys.KubernetesClient
	PodMgr                *PodManager
	NodeName              string
	VCUDAServers          map[string]*grpc.Server
	ContainerByUID        map[string]*jsonObj.JsonObject
	ContainerUIDInPodUID  map[string][]string
	PodVisitedByPodName   map[string]bool
	coreRequestByPodUID   map[string]int64
	memoryRequestByPodUID map[string]int64
	GpuNameByUuid         map[string]string
	mu                    sync.Mutex
}

func NewNodeDaemon(client *kubesys.KubernetesClient, podMgr *PodManager, nodeName string) *NodeDaemon {
	return &NodeDaemon{
		Client:                client,
		PodMgr:                podMgr,
		NodeName:              nodeName,
		VCUDAServers:          make(map[string]*grpc.Server),
		ContainerByUID:        make(map[string]*jsonObj.JsonObject),
		ContainerUIDInPodUID:  make(map[string][]string),
		PodVisitedByPodName:   make(map[string]bool),
		coreRequestByPodUID:   make(map[string]int64),
		memoryRequestByPodUID: make(map[string]int64),
		GpuNameByUuid:         make(map[string]string),
	}
}

func (daemon *NodeDaemon) Run(hostname string) {
	if err := os.MkdirAll(VirtualManagerPath, 0777); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Failed to create %s, %s.", VirtualManagerPath, err)
	}

	daemon.Client.DeleteResource("GPU", GPUCRDNamespace, "")

	n, err := nvml.GetDeviceCount()
	if err != nil {
		log.Fatalf("Failed to get device count, %s.", err)
	}

	for index := uint(0); index < n; index++ {
		device, err := nvml.NewDevice(index)
		if err != nil {
			log.Fatalf("Failed to get device %d, %s.", index, err)
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
		if daemon.PodMgr.queueOfAdded.Len() > 0 {
			daemon.PodMgr.muOfAdd.Lock()
			pod := daemon.PodMgr.queueOfAdded.Remove()
			daemon.PodMgr.muOfAdd.Unlock()
			time.Sleep(5 * time.Millisecond)
			go daemon.addPod(pod)
		}

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

func (daemon *NodeDaemon) addPod(pod *jsonObj.JsonObject) {
	meta := pod.GetJsonObject("metadata")
	if !meta.HasKey("annotations") {
		return
	}
	annotations := meta.GetJsonObject("annotations")
	if !annotations.HasKey(AnnAssumeTime) {
		return
	}

	podUID, err := meta.GetString("uid")
	if err != nil {
		log.Errorln("Failed to get pod podUID.")
		return
	}

	status := pod.GetJsonObject("status")
	containers := status.GetJsonArray("containerStatuses")
	for _, c := range containers.Values() {
		container := c.JsonObject()
		uidStr, err := container.GetString("containerID")
		if err != nil {
			log.Errorln("Failed to get containerUID.")
			return
		}
		uid := strings.Split(uidStr, "docker://")[1]
		daemon.mu.Lock()
		daemon.ContainerByUID[uid] = container
		daemon.ContainerUIDInPodUID[podUID] = append(daemon.ContainerUIDInPodUID[podUID], uid)
		daemon.mu.Unlock()
	}
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
		log.Errorln("Failed to get pod name.")
		return
	}
	namespace, err := meta.GetString("namespace")
	if err != nil {
		log.Errorln("Failed to get pod namespace.")
		return
	}
	podUID, err := meta.GetString("uid")
	if err != nil {
		log.Errorln("Failed to get pod podUID.")
		return
	}

	daemon.mu.Lock()
	if daemon.PodVisitedByPodName[namespace+"/"+podName] {
		daemon.mu.Unlock()
		return
	}
	daemon.PodVisitedByPodName[namespace+"/"+podName] = true
	daemon.mu.Unlock()

	// Create VCUDA gRPC server
	log.Infof("Creating VCUDA gRPC server for pod %s.", podUID)
	baseDir := filepath.Join(VirtualManagerPath, podUID)
	if err := os.MkdirAll(baseDir, 0777); err != nil && !os.IsExist(err) {
		log.Errorf("Failed to create %s, %s.", baseDir, err)
		return
	}

	sockfile := filepath.Join(baseDir, VCUDASocket)
	err = syscall.Unlink(sockfile)
	if err != nil && !os.IsNotExist(err) {
		log.Errorf("Failed to remove %s, %s.", sockfile, err)
		return
	}

	l, err := net.Listen("unix", sockfile)
	if err != nil {
		log.Errorf("Failed to listen for %s, %s.", sockfile, err)
		return
	}

	err = os.Chmod(sockfile, 0777)
	if err != nil {
		log.Errorf("Failed to chmod for %s, %s.", sockfile, err)
		return
	}

	server := grpc.NewServer([]grpc.ServerOption{}...)
	vcuda.RegisterVCUDAServiceServer(server, daemon)
	go server.Serve(l)

	daemon.mu.Lock()
	daemon.VCUDAServers[podUID] = server
	daemon.mu.Unlock()
	log.Infof("Success to create VCUDA gRPC server for pod %s.", podUID)

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

	daemon.mu.Lock()
	daemon.coreRequestByPodUID[podUID] = requestCore
	daemon.memoryRequestByPodUID[podUID] = requestMemory
	daemon.mu.Unlock()

	// Update annotation
	time.Sleep(time.Second)
	copyPodBytes, err := daemon.Client.GetResource("Pod", namespace, podName)
	if err != nil {
		log.Errorf("Failed to get copy pod %s on ns %s, %s.", podName, namespace, err)
		return
	}
	copyPod := kubesys.ToJsonObject(copyPodBytes)
	copyMeta := copyPod.GetJsonObject("metadata")
	copyAnnotations := copyMeta.GetJsonObject("annotations")

	copyAnnotations.Put(AnnVCUDAReady, "yes")
	copyMeta.Put("annotations", copyAnnotations.ToInterface())
	copyPod.Put("metadata", copyMeta.ToInterface())

	_, err = daemon.Client.UpdateResource(copyPod.ToString())
	if err != nil {
		log.Errorf("Failed to set pod %s's annotations, %s.", podName, err)
		return
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
		log.Errorln("Failed to get pod name.")
		return
	}
	namespace, err := meta.GetString("namespace")
	if err != nil {
		log.Errorln("Failed to get pod namespace.")
		return
	}
	podUID, err := meta.GetString("uid")
	if err != nil {
		log.Errorln("Failed to get pod podUID.")
		return
	}

	log.Infof("Clean for pod %s.", podUID)
	daemon.mu.Lock()
	if !daemon.PodVisitedByPodName[namespace+"/"+podName] {
		daemon.mu.Unlock()
		return
	}
	daemon.PodVisitedByPodName[namespace+"/"+podName] = false
	delete(daemon.coreRequestByPodUID, podUID)
	delete(daemon.memoryRequestByPodUID, podUID)
	daemon.VCUDAServers[podUID].Stop()
	delete(daemon.VCUDAServers, podUID)
	if val, ok := daemon.ContainerUIDInPodUID[podUID]; ok {
		for _, id := range val {
			delete(daemon.ContainerByUID, id)
		}
	}
	delete(daemon.ContainerUIDInPodUID, podUID)
	daemon.mu.Unlock()

	os.RemoveAll(filepath.Clean(filepath.Join(VirtualManagerPath, podUID)))

}

func (daemon *NodeDaemon) RegisterVDevice(ctx context.Context, req *vcuda.VDeviceRequest) (*vcuda.VDeviceResponse, error) {
	podUID := req.PodUid
	containerID := req.ContainerId
	containerName := req.ContainerName

	log.Infof("Pod %s, container %s call rpc.", podUID, containerID)
	baseDir := filepath.Join(VirtualManagerPath, podUID, containerID)

	if err := os.MkdirAll(baseDir, 0777); err != nil && !os.IsNotExist(err) {
		log.Errorf("Failed to create %s, %s.", baseDir, err)
		return nil, err
	}

	pidFileName := filepath.Join(baseDir, PidsConfig)
	vcudaFileName := filepath.Join(baseDir, VCUDAConfig)

	// Create pids.config file

	err := daemon.createPidsFile(pidFileName, containerID, pod)
	if err != nil {
		log.Errorf("Failed to create %s, %s.", pidFileName, err)
		return nil, err
	}

	// Create vcuda.config file
	err = daemon.createVCUDAFile(vcudaFileName, podUID, containerName)
	if err != nil {
		log.Errorf("Failed to create %s, %s.", vcudaFileName, err)
		return nil, err
	}

	return &vcuda.VDeviceResponse{}, nil
}

func (daemon *NodeDaemon) createPidsFile(pidFileName, containerID string, pod *jsonObj.JsonObject) error {
	log.Infof("Write %s", pidFileName)
	cFileName := C.CString(pidFileName)
	defer C.free(unsafe.Pointer(cFileName))

	cgroupPath, err := getCgroupPath(pod, containerID)
	if err != nil {
		return err
	}

	if C.pids_to_disk(cFileName, &fakePid, 4) != 0 {
		return errors.New("create pids.config file error")
	}

	return nil
}

func (daemon *NodeDaemon) createVCUDAFile(vcudaFileName, podUID, containerName string) error {
	log.Infof("Write %s", vcudaFileName)
	requestMemory, requestCore := int64(0), int64(0)
	daemon.mu.Lock()
	requestMemory = daemon.memoryRequestByPodUID[podUID]
	requestCore = daemon.coreRequestByPodUID[podUID]
	daemon.mu.Unlock()

	var vcudaConfig C.struct_resource_data_t

	cPodUID := C.CString(podUID)
	cContName := C.CString(containerName)
	cFileName := C.CString(vcudaFileName)

	defer C.free(unsafe.Pointer(cPodUID))
	defer C.free(unsafe.Pointer(cContName))
	defer C.free(unsafe.Pointer(cFileName))

	C.strcpy(&vcudaConfig.pod_uid[0], (*C.char)(unsafe.Pointer(cPodUID)))
	C.strcpy(&vcudaConfig.container_name[0], (*C.char)(unsafe.Pointer(cContName)))
	vcudaConfig.gpu_memory = C.uint64_t(requestMemory) * MemoryBlockSize
	vcudaConfig.utilization = C.int(requestCore)
	vcudaConfig.hard_limit = 1
	vcudaConfig.driver_version.major = C.int(DriverVersionMajor)
	vcudaConfig.driver_version.minor = C.int(DriverVersionMinor)
	vcudaConfig.enable = 1

	if C.setting_to_disk(cFileName, &vcudaConfig) != 0 {
		return errors.New("create vcuda.config file error")
	}

	return nil

}
