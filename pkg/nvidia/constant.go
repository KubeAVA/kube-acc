/**
 * Copyright (2021, ) Institute of Software, Chinese Academy of Sciences
 **/

package nvidia

import pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

const (
	ServerSock    = pluginapi.DevicePluginPath + "doslab.sock"
	ResourceName  = "doslab.io/gpu-memory"
	ResourceCount = "doslab.io/gpu-count"
	ResourceCore  = "doslab.io/gpu-core"

	AnnResourceAssumeTime = "doslab.io/gpu-assume-time"
	AnnGemSchedulerIp     = "doslab.io/gem-scheduler-ip"
	AnnAssignedFlag       = "doslab.io/gpu-assigned"
	AnnResourceUUID       = "doslab.io/gpu-uuid"
	AnnGemPodManagerPort  = "doslab.io/gem-podmanager-port"

	EnvResourceUUID            = "DOSLAB_IO_GPU_UUID"
	EnvResourceUsedByPod       = "DOSLAB_IO_GPU_RESOURCE_USED_BY_POD"
	EnvResourceUsedByContainer = "DOSLAB_IO_GPU_RESOURCE_USED_BY_CONTAINER"
	EnvResourceTotal           = "DOSLAB_IO_GPU_RESOURCE_TOTAL"

	EnvNvidiaGPU                = "NVIDIA_VISIBLE_DEVICES"
	EnvPodName                  = "POD_NAME"
	EnvPodManagerPort           = "POD_MANAGER_PORT"
	EnvPodManagerIp             = "POD_MANAGER_IP"
	EnvLDPreload                = "LD_PRELOAD"
	EnvNvidiaDriverCapabilities = "NVIDIA_DRIVER_CAPABILITIES"

	KubeShareLibraryPath = "/kubeshare/library"
)
