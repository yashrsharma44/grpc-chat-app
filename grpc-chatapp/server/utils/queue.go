package container

import (
	"errors"
	"sync"

	"google.golang.org/api/chat/v1"
)

// TODO : yashrsharma
// Move this to a separate file
type Queue struct {
	container     []*chat.Message
	containerLock sync.RWMutex
}

func (q *Queue) len() int {

	var length int
	q.containerLock.RLock()
	length = len(q.container)
	q.containerLock.RUnlock()
	return length
}

func (q *Queue) enqueue(element *chat.Message) error {

	q.containerLock.RLock()
	q.container = append(q.container, element)
	q.containerLock.RUnlock()
	return nil
}

func (q *Queue) dequeue() (*chat.Message, error) {

	q.containerLock.RLock()
	if len(q.container) == 0 {
		defer q.containerLock.RUnlock()
		return nil, errors.New("Queue is empty!")
	}
	res := q.container[0]
	q.container = q.container[1:]
	q.containerLock.RUnlock()
	return res, nil
}
