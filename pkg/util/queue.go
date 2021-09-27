/**
 * Copyright (2021, ) Institute of Software, Chinese Academy of Sciences
 **/

package util

import (
	"github.com/kubesys/kubernetes-client-go/pkg/util"
)

type Elem struct {
	value *util.ObjectNode
	prev *Elem
	next *Elem
}

type LinkedQueue struct {
	head *Elem
	tail *Elem
	size int
}

func NewLinkedQueue() *LinkedQueue {
	return &LinkedQueue{nil, nil, 0}
}

func (queue *LinkedQueue) Len() int {
	return queue.size
}

func (queue *LinkedQueue) Remove() *util.ObjectNode {
	if queue.size == 0 {
		return nil
	}
	elem := queue.head
	queue.head = elem.next
	queue.size--
	return elem.value
}

func (queue *LinkedQueue) Add(value *util.ObjectNode) {
	elem := &Elem{value, queue.tail, nil}
	if queue.size == 0 {
		queue.head = elem
		queue.tail = elem
	} else {
		queue.tail.next = elem
		queue.tail = elem
	}
	queue.size++
}