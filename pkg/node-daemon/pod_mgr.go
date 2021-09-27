/**
 * Copyright (2021, ) Institute of Software, Chinese Academy of Sciences
 **/

package node_daemon

import (
	"github.com/kubesys/kube-alloc/pkg/util"
	jsonutil "github.com/kubesys/kubernetes-client-go/pkg/util"
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
	podMgr.muOfModify.Lock()
	podMgr.queueOfModified.Add(jsonutil.NewObjectNodeWithValue(obj))
	podMgr.muOfModify.Unlock()
}

func (podMgr *PodManager) DoDeleted(obj map[string]interface{}) {
	podMgr.muOfDelete.Lock()
	podMgr.queueOfDeleted.Add(jsonutil.NewObjectNodeWithValue(obj))
	podMgr.muOfDelete.Unlock()
}
