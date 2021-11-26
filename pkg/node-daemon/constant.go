/**
 * Copyright (2021, ) Institute of Software, Chinese Academy of Sciences
 **/

package node_daemon

const (
	GemSchedulerIpPath                 = "/kubeshare/library/schedulerIP.txt"
	GemSchedulerGPUConfigPath          = "/kubeshare/scheduler/config/"
	GemSchedulerGPUPodManagerPortPath  = "/kubeshare/scheduler/podmanagerport/"
	GemSchedulerGPUPodManagerPortStart = 50050
	GemSchedulerGPUPodManagerPortEnd   = 50550

	EnvGemSchedulerIp = "GEM_SCHEDULER_IP"

	GPUCRDAPIVersion = "doslab.io/v1"
	GPUCRDNamespace  = "default"

	VCUDASocket = "vcuda.sock"

	VirtualManagerPath = "/etc/kube-gpu/vm"

	PidsConfig  = "pids.config"
	VCUDAConfig = "vcuda.config"

	AnnAssumeTime        = "doslab.io/gpu-assume-time"
	AnnGemSchedulerIp    = "doslab.io/gem-scheduler-ip"
	AnnGemPodManagerPort = "doslab.io/gem-podmanager-port"

	ResourceMemory = "doslab.io/gpu-memory"
	ResourceCore   = "doslab.io/gpu-core"
	ResourceUUID   = "doslab.io/gpu-uuid"
)
