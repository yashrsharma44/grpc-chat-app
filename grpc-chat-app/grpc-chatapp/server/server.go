package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	chat "github.com/yashrsharma44/grpc-chat-app/grpc-chatapp/schema"
	"google.golang.org/grpc"
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

type server struct {
	container chan *chat.Message
}

func (s *server) SubscribeMessage(req *chat.EmptyRequest, stream chat.Chat_SubscribeMessageServer) error {

	fmt.Println("Inside")
	for {

		res := <-s.container
		fmt.Printf("Popped - %v\n", res)

		newReq := chat.Message{
			Username:  res.Username,
			Timestamp: res.Timestamp,
			Content:   res.Content,
		}
		stream.Send(&newReq)
	}

}

func (s *server) SendMessage(ctx context.Context, req *chat.Message) (*chat.EmptyResponse, error) {

	s.container <- req
	fmt.Printf("Pushed - %v\n", req)
	return &chat.EmptyResponse{}, nil

}

func main() {
	fmt.Println("Hello World")
	lis, err := net.Listen("tcp", "0.0.0.0:50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	chat.RegisterChatServer(s, &server{
		container: make(chan *chat.Message, 20),
	})

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}

}
