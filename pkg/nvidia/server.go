/**
 * Copyright (2021, ) Institute of Software, Chinese Academy of Sciences
 **/

package nvidia

import (
	"context"
	"errors"
	"fmt"
	"github.com/kubesys/kubernetes-client-go/pkg/kubesys"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	"math"
	"net"
	"os"
	"path"
	"sort"
	"strconv"
	"time"
)

// NvidiaDevicePlugin implements the Kubernetes device plugin API
type NvidiaDevicePlugin struct {
	devices                []*pluginapi.Device
	physicalDeviceNameById map[uint]string
	server                 *grpc.Server
	socket                 string
	stop                   chan struct{}
	messenger              *KubeMessenger
	nodeName               string
}

func NewNvidiaDevicePlugin(client *kubesys.KubernetesClient, nodeName string) *NvidiaDevicePlugin {
	messenger := NewKubeMessenger(client, nodeName)

	return &NvidiaDevicePlugin{
		devices:                nil,
		physicalDeviceNameById: nil,
		server:                 nil,
		socket:                 ServerSock,
		stop:                   nil,
		messenger:              messenger,
		nodeName:               nodeName,
	}
}

// Start mainly starts the gRPC server and register the device plugin to Kubelet
func (p *NvidiaDevicePlugin) Start() error {
	log.Infoln("Starting the device plugin")

	p.devices, p.physicalDeviceNameById = GetDevices()
	p.server = grpc.NewServer([]grpc.ServerOption{}...)
	p.stop = make(chan struct{})

	err := p.messenger.PatchGPUCount(uint(len(p.physicalDeviceNameById)), uint(100*len(p.physicalDeviceNameById)))
	if err != nil {
		p.cleanup()
		return err
	}

	err = p.Serve()
	if err != nil {
		p.cleanup()
		log.Warningf("Could not serve: %s.", err)
		return err
	}

	err = p.Register(ResourceName)
	if err != nil {
		p.cleanup()
		log.Warningf("Could not register device plugin: %s.", err)
		return err
	}
	log.Infoln("Registered device plugin with Kubelet.")

	return nil
}

func (p *NvidiaDevicePlugin) Stop() error {
	log.Infoln("Stopping the device plugin")

	if p == nil || p.server == nil {
		return nil
	}
	p.server.Stop()
	p.cleanup()
	if err := p.messenger.PatchGPUCount(0, 0); err != nil {
		return err
	}
	if err := os.Remove(p.socket); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (p *NvidiaDevicePlugin) Serve() error {
	sock, err := net.Listen("unix", p.socket)
	if err != nil {
		return err
	}
	pluginapi.RegisterDevicePluginServer(p.server, p)

	go p.server.Serve(sock)

	conn, err := dial(p.socket, 5*time.Second)
	if err != nil {
		return err
	}
	conn.Close()

	return nil
}

func (p *NvidiaDevicePlugin) Register(resourceName string) error {
	conn, err := dial(pluginapi.KubeletSocket, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pluginapi.NewRegistrationClient(conn)
	req := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     path.Base(p.socket),
		ResourceName: resourceName,
	}

	_, err = client.Register(context.Background(), req)
	if err != nil {
		return err
	}
	return nil
}

func (p *NvidiaDevicePlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	err := s.Send(&pluginapi.ListAndWatchResponse{Devices: p.devices})
	if err != nil {
		log.Fatalln("Failed to send devices.")
	}
	log.Infof("Send %d virtual devices.", len(p.devices))

	select {
	case <-p.stop:
		return nil
	}
}

func (p *NvidiaDevicePlugin) Allocate(ctx context.Context, reqs *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	log.Infoln("Allocating GPU...")
	responses := pluginapi.AllocateResponse{}

	var (
		podReqGPUCount uint
		found          bool
		assumePod      *v1.Pod
	)

	for _, req := range reqs.ContainerRequests {
		podReqGPUCount += uint(len(req.DevicesIDs))
	}
	log.Infof("Pod request GPU count is %d.", podReqGPUCount)

	pendingPods := p.messenger.GetPendingPodsOnNode()
	var candidatePods []*v1.Pod
	for _, pod := range pendingPods {
		if isCandidatePod(&pod) {
			candidatePods = append(candidatePods, &pod)
		}
	}

	if candidatePods == nil || len(candidatePods) == 0 {
		log.Warningln("There is no candidate pods.")
		return nil, errors.New("not found candidate pod")
	}

	candidatePods = sortPodByAssumeTime(candidatePods)

	for _, pod := range candidatePods {
		var resourceTotal uint = 0
		for _, container := range pod.Spec.Containers {
			if val, ok := container.Resources.Limits[ResourceName]; ok {
				resourceTotal += uint(val.Value())
			}
		}
		if resourceTotal == podReqGPUCount {
			assumePod = pod
			found = true
			break
		}
	}

	if !found {
		log.Warningln("There is no assume pod.")
		return nil, errors.New("not found assume pod")
	}

	gpuId := getGPUIDFromPodAnnotation(assumePod)

	if gpuId == "" {
		log.Warningf("Failed to get gpu id for pod %s in ns %s.", assumePod.Name, assumePod.Namespace)
	}

	gemSchedulerIp := ""
	gemPodManagerPort := ""
	isOk := false
	for i := 0; i < 100; i++ {
		pod := p.messenger.GetPodOnNode(assumePod.Name, assumePod.Namespace)
		if pod == nil {
			log.Warningf("Failed to get pod %s, on ns %s.", assumePod.Name, assumePod.Namespace)
			time.Sleep(time.Millisecond * 100)
			continue
		}
		gemSchedulerIp = getGemSchedulerIpFromPodAnnotation(pod)
		gemPodManagerPort = getGemPodManagerPortFromPodAnnotation(pod)
		if gemSchedulerIp != "" && gemPodManagerPort != "" {
			isOk = true
			assumePod = pod
			break
		}
		time.Sleep(time.Millisecond * 100)
	}

	if !isOk {
		return nil, errors.New("no gem-scheduler-ip, gem-podmanager-port or gem-file")
	}

	for _, req := range reqs.ContainerRequests {
		reqGPU := uint(len(req.DevicesIDs))
		response := pluginapi.ContainerAllocateResponse{
			Envs: map[string]string{
				EnvNvidiaDriverCapabilities: "compute,utility",
				EnvLDPreload:                KubeShareLibraryPath + "/libgemhook.so.1",
				EnvPodManagerIp:             gemSchedulerIp,
				EnvPodManagerPort:           gemPodManagerPort,
				EnvPodName:                  assumePod.Name,
				EnvNvidiaGPU:                gpuId,
				EnvResourceUUID:             gpuId,
				EnvResourceUsedByPod:        fmt.Sprintf("%d", podReqGPUCount),
				EnvResourceUsedByContainer:  fmt.Sprintf("%d", reqGPU),
				EnvResourceTotal:            fmt.Sprintf("%d", len(p.devices)),
			},
		}
		responses.ContainerResponses = append(responses.ContainerResponses, &response)
	}

	newPod := assumePod.DeepCopy()
	newPod.Annotations[AnnAssignedFlag] = "true"

	err := p.messenger.UpdatePodAnnotations(newPod)
	if err != nil {
		log.Warningln("Failed to update pod annotation.")
		return nil, errors.New("failed to update pod annotation")
	}

	log.Infof("Pod %s in ns %s allocate gpu successed.", newPod.Name, newPod.Namespace)

	return &responses, nil
}

func (p *NvidiaDevicePlugin) cleanup() {
	close(p.stop)
	p.devices = nil
	p.physicalDeviceNameById = nil
	p.server = nil
	p.stop = nil
}

func dial(unixSocketPath string, timeout time.Duration) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c, err := grpc.DialContext(ctx, unixSocketPath, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)

	if err != nil {
		return nil, err
	}

	return c, nil
}

func (p *NvidiaDevicePlugin) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

func (p *NvidiaDevicePlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (p *NvidiaDevicePlugin) GetPreferredAllocation(context.Context, *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	return &pluginapi.PreferredAllocationResponse{}, nil
}

func isCandidatePod(pod *v1.Pod) bool {
	var resourceTotal uint = 0
	for _, container := range pod.Spec.Containers {
		if val, ok := container.Resources.Limits[ResourceName]; ok {
			resourceTotal += uint(val.Value())
		}
	}
	if resourceTotal <= uint(0) {
		return false
	}

	if _, ok := pod.ObjectMeta.Annotations[AnnResourceAssumeTime]; !ok {
		return false
	}

	if assigned, ok := pod.ObjectMeta.Annotations[AnnAssignedFlag]; ok {
		if assigned == "false" {
			return true
		} else {
			return false
		}
	} else {
		return false
	}
}

func getGPUIDFromPodAnnotation(pod *v1.Pod) (uuid string) {
	uuid = ""
	if len(pod.ObjectMeta.Annotations) > 0 {
		value, found := pod.ObjectMeta.Annotations[AnnResourceUUID]
		if found {
			uuid = value
		} else {
			log.Warningf("Failed to get dev id for pod %s in ns %s.", pod.Name, pod.Namespace)
		}
	}
	return uuid
}

func getGemSchedulerIpFromPodAnnotation(pod *v1.Pod) string {
	id := ""
	if len(pod.ObjectMeta.Annotations) > 0 {
		value, found := pod.ObjectMeta.Annotations[AnnGemSchedulerIp]
		if found {
			id = value
		} else {
			log.Warningf("Failed to get gem-scheduler-ip for pod %s in ns %s.", pod.Name, pod.Namespace)
		}
	}
	return id
}

func getGemPodManagerPortFromPodAnnotation(pod *v1.Pod) string {
	port := ""
	if len(pod.ObjectMeta.Annotations) > 0 {
		value, found := pod.ObjectMeta.Annotations[AnnGemPodManagerPort]
		if found {
			port = value
		} else {
			log.Warningf("Failed to get gem-podmanager-port for pod %s in ns %s.", pod.Name, pod.Namespace)
		}
	}
	return port
}

func sortPodByAssumeTime(pods []*v1.Pod) []*v1.Pod {
	podList := make(orderedPodByAssumeTime, 0, len(pods))
	for _, v := range pods {
		podList = append(podList, v)
	}
	sort.Sort(podList)
	return []*v1.Pod(podList)
}

type orderedPodByAssumeTime []*v1.Pod

func (this orderedPodByAssumeTime) Len() int {
	return len(this)
}

func (this orderedPodByAssumeTime) Less(i, j int) bool {
	return getAssumeTimeFromPodAnnotation(this[i]) <= getAssumeTimeFromPodAnnotation(this[j])
}

func (this orderedPodByAssumeTime) Swap(i, j int) {
	this[i], this[j] = this[j], this[i]
}
func getAssumeTimeFromPodAnnotation(pod *v1.Pod) uint64 {
	if assumeTime, ok := pod.Annotations[AnnResourceAssumeTime]; ok {
		predicateTime, err := strconv.ParseUint(assumeTime, 10, 64)
		if err == nil {
			return predicateTime
		}
	}
	return math.MaxUint64
}
