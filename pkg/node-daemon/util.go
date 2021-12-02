/**
 * Copyright (2021, ) Institute of Software, Chinese Academy of Sciences
 **/

package node_daemon

import (
	"fmt"
	jsonObj "github.com/kubesys/kubernetes-client-go/pkg/json"
	"path/filepath"
	"strings"
)

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

func getCgroupPath(pod *jsonObj.JsonObject, containerID string) (string, error) {
	meta := pod.GetJsonObject("metadata")
	podUID, err := meta.GetString("uid")
	if err != nil {
		return "", err
	}
	status := pod.GetJsonObject("status")
	qosClass, err := status.GetString("qosClass")
	if err != nil {
		return "", err
	}

	name := "kubepods"
	switch qosClass {
	case PodQOSGuaranteed:
	case PodQOSBurstable:
		name = filepath.Join(name, strings.ToLower(PodQOSBurstable))
	case PodQOSBestEffort:
		name = filepath.Join(name, strings.ToLower(PodQOSBestEffort))
	}

	name = filepath.Join(name, "pod"+podUID)
	return fmt.Sprintf("%s/%s", name, containerID), nil
}
