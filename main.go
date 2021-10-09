/**
 * Copyright (2021, ) Institute of Software, Chinese Academy of Sciences
 **/

package main

import (
	"flag"
	"github.com/NVIDIA/gpu-monitoring-tools/bindings/go/nvml"
	"github.com/fsnotify/fsnotify"
	node_daemon "github.com/kubesys/kube-alloc/pkg/node-daemon"
	"github.com/kubesys/kube-alloc/pkg/nvidia"
	"github.com/kubesys/kube-alloc/pkg/util"
	"github.com/kubesys/kubernetes-client-go/pkg/kubesys"
	log "github.com/sirupsen/logrus"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	"os"
	"syscall"
)

const (
	GPUNamespace      = "default"
	EnvDaemonNodeName = "DAEMON_NODE_NAME"
)

var (
	masterUrl = flag.String("masterUrl", "", "Kubernetes master url.")
	token     = flag.String("token", "", "Kubernetes client token.")
)

func main() {
	flag.Parse()
	if *masterUrl == "" || *token == "" {
		log.Fatalln("Error masterUrl or token.")
	}

	client := kubesys.NewKubernetesClient(*masterUrl, *token)
	client.Init()

	nodeName := os.Getenv(EnvDaemonNodeName)
	if nodeName == "" {
		log.Fatalln("Failed to get env DaemonNodeName.")
	}

	log.Infoln("Loading NVML...")
	if err := nvml.Init(); err != nil {
		log.Infof("Failed to initialize NVML: %s.", err)
		log.Infof("If this is a GPU node, did you set the docker default runtime to `nvidia`?")
		log.Fatalln("Failed to discover gpus.")
	}
	defer func() {
		log.Infof("Shutdown of NVML returned: %s.", nvml.Shutdown())
	}()

	// node daemon
	log.Infof("Starting node daemon on %s.", nodeName)

	podMgr := node_daemon.NewPodManager(util.NewLinkedQueue(), util.NewLinkedQueue())
	daemon := node_daemon.NewNodeDaemon(client, podMgr, nodeName)
	daemon.Listen(podMgr)

	go daemon.Run(nodeName)

	// device plugin
	log.Infof("Starting device plugin on %s.", nodeName)
	sigChan := nvidia.NewOSWatcher(syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	watcher, err := nvidia.NewFileWatcher(pluginapi.DevicePluginPath)
	if err != nil {
		log.Fatalf("Failed to created file watcher: %s.", err)
	}

	devicePlugin := nvidia.NewNvidiaDevicePlugin(client, nodeName)

	go func() {
		select {
		case sig := <-sigChan:
			devicePlugin.Stop()
			for _, gpu := range daemon.GpuNameByUuid {
				daemon.Client.DeleteResource("GPU", GPUNamespace, gpu)
			}
			log.Fatalf("Received signal %v, shutting down.", sig)
		}
	}()

restart:

	devicePlugin.Stop()

	if _, m := nvidia.GetDevices(); len(m) == 0 {
		log.Warningln("There is no device, try to restart.")
		goto restart
	}

	if err := devicePlugin.Start(); err != nil {
		log.Warningf("Device plugin failed to start due to %s.", err)
		goto restart
	}

	for {
		select {
		case event := <-watcher.Events:
			if event.Name == pluginapi.KubeletSocket && event.Op&fsnotify.Create == fsnotify.Create {
				log.Infof("Inotify: %s created, restarting.", pluginapi.KubeletSocket)
				goto restart
			}
		case err := <-watcher.Errors:
			log.Warningf("Inotify: %s", err)
		}
	}

}
