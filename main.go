/**
 * Copyright (2021, ) Institute of Software, Chinese Academy of Sciences
 **/

package main

import (
	"flag"
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
	GPUNamespace     = "default"
	DefaultMasterUrl = "https://133.133.135.42:6443"
	DefaultToken     = "eyJhbGciOiJSUzI1NiIsImtpZCI6IjN2U3VZUk16R3ZfZGNaMkw4bVktVGlRWnJGZFB2NWprU1lrd0hObnNBVFEifQ.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJrdWJlLXN5c3RlbSIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VjcmV0Lm5hbWUiOiJrdWJlcm5ldGVzLWNsaWVudC10b2tlbi10Z202ZyIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VydmljZS1hY2NvdW50Lm5hbWUiOiJrdWJlcm5ldGVzLWNsaWVudCIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VydmljZS1hY2NvdW50LnVpZCI6IjI0MjNlMDJmLTdmYzAtNDEzYi04ODczLTc0YTM3MTFkMzdkOSIsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDprdWJlLXN5c3RlbTprdWJlcm5ldGVzLWNsaWVudCJ9.KVJ7NC4NAWViLy2YkFFzzg0G4NcKnAZzw8VYooyXaLQlyfJWysR0giU8QLcSRs5BqIagff2EcVBuVHmSE4o1Zt3AMayStk-stwdtQre28adKYwR4aJLtfa1Wqmw--RiBHZmOjOmzynDdtWEe_sJPl4bGSxMvjFEKy6OepXOctnqZjUq4x2mMK-FID5hmeoHY6oAcfrRuAJsHRuLEAJQzLiMAf9heTuRNxcv3OTyfGtLOOj9risr59wilC_JWVPC5DC5TkEe4-8OeWg_mKA-lwSss_nyGMCsBqPIdPeyd3RQQ9ADPDq-JP2Nci0zoqOEwgZu3nQ3wOovR7lFBbRxsQQ"
)

var (
	masterUrl = flag.String("masterUrl", DefaultMasterUrl, "Kubernetes master url.")
	token     = flag.String("token", DefaultToken, "Kubernetes client token.")
)

func main() {
	client := kubesys.NewKubernetesClient(*masterUrl, *token)
	client.Init()

	nodeName, err := os.Hostname()
	if err != nil {
		log.Fatalf("Failed to get node name, %s.", err)
	}

	// node daemon
	log.Infof("Starting node daemon on %s.", nodeName)

	podMgr := node_daemon.NewPodManager(util.NewLinkedQueue(), util.NewLinkedQueue())
	daemon := node_daemon.NewNodeDaemon(client, podMgr, nodeName)
	daemon.Listen(podMgr)

	go daemon.Run()

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
			for _, gpu := range daemon.GpuUuidByName {
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
