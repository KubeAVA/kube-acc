/**
 * Copyright (2021, ) Institute of Software, Chinese Academy of Sciences
 **/

package node_daemon

import (
	"encoding/json"
	"github.com/kubesys/kube-alloc/pkg/util"
	"github.com/kubesys/kubernetes-client-go/pkg/kubesys"
	"sync"
)

type PodManager struct {
	queueOfModified *util.LinkedQueue
	queueOfDeleted  *util.LinkedQueue
	muOfModify      sync.Mutex
	muOfDelete      sync.Mutex
}

func NewPodManager(queueOfModified, queueOfDeleted *util.LinkedQueue) *PodManager {
	return &PodManager{queueOfModified: queueOfModified, queueOfDeleted: queueOfDeleted}
}

func (podMgr *PodManager) DoAdded(obj map[string]interface{}) {
}

func (podMgr *PodManager) DoModified(obj map[string]interface{}) {
	bytes, _ := json.Marshal(obj)
	podMgr.muOfModify.Lock()
	podMgr.queueOfModified.Add(kubesys.ToJsonObject(bytes))
	podMgr.muOfModify.Unlock()
}

func (podMgr *PodManager) DoDeleted(obj map[string]interface{}) {
	bytes, _ := json.Marshal(obj)
	podMgr.muOfDelete.Lock()
	podMgr.queueOfDeleted.Add(kubesys.ToJsonObject(bytes))
	podMgr.muOfDelete.Unlock()
}
