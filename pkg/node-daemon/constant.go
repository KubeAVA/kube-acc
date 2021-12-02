/**
 * Copyright (2021, ) Institute of Software, Chinese Academy of Sciences
 **/

package node_daemon

const (
	GPUCRDAPIVersion = "doslab.io/v1"
	GPUCRDNamespace  = "default"

	VCUDASocket = "vcuda.sock"

	VirtualManagerPath = "/etc/kube-gpu/vm"

	PidsConfig  = "pids.config"
	VCUDAConfig = "vcuda.config"

	DriverVersionMajor = 465
	DriverVersionMinor = 31

	MemoryBlockSize = 1024 * 1024

	AnnAssumeTime = "doslab.io/gpu-assume-time"
	AnnVCUDAReady = "doslab.io/vcuda"

	PodQOSGuaranteed = "Guaranteed"
	PodQOSBurstable  = "Burstable"
	PodQOSBestEffort = "BestEffort"

	ResourceMemory = "doslab.io/gpu-memory"
	ResourceCore   = "doslab.io/gpu-core"
)
