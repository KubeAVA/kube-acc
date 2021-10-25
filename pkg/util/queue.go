/**
 * Copyright (2021, ) Institute of Software, Chinese Academy of Sciences
 **/

package util

import (
	jsonObj "github.com/kubesys/kubernetes-client-go/pkg/json"
)

type Elem struct {
	value *jsonObj.JsonObject
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

func (queue *LinkedQueue) Remove() *jsonObj.JsonObject {
	if queue.size == 0 {
		return nil
	}
	elem := queue.head
	queue.head = elem.next
	queue.size--
	return elem.value
}

func (queue *LinkedQueue) Add(value *jsonObj.JsonObject) {
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