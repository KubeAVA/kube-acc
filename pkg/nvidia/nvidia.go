/**
 * Copyright (2021, ) Institute of Software, Chinese Academy of Sciences
 **/

package nvidia

import (
	"fmt"
	"github.com/NVIDIA/gpu-monitoring-tools/bindings/go/nvml"
	log "github.com/sirupsen/logrus"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

func check(err error) {
	if err != nil {
		log.Fatalf("Fatal: %s", err)
	}
}

func GetDevices() ([]*pluginapi.Device, map[uint]string) {
	n, err := nvml.GetDeviceCount()
	check(err)

	log.Infoln("Getting devices...")
	var devices []*pluginapi.Device
	deviceMap := map[uint]string{}
	for idx := uint(0); idx < n; idx++ {
		d, err := nvml.NewDevice(idx)
		check(err)
		deviceMap[idx] = d.UUID

		memory := *d.Memory
		for i := uint64(0); i < memory; i++ {
			fakeID := fmt.Sprintf("fakeID---%d---%d", idx, i)
			devices = append(devices, &pluginapi.Device{
				ID:     fakeID,
				Health: pluginapi.Healthy,
			})
		}

	}
	log.Infof("Get %d physical devices and %d virtual devices.", n, len(devices))
	return devices, deviceMap
}
