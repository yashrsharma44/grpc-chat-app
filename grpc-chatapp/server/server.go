package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"github.com/golang/protobuf/ptypes"
	chat "github.com/yashrsharma44/grpc-chat-app/grpc-chatapp/schema"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const tokenHeader = "x-chat-token"
const tokenSize = 4
const responseChannelSize = 20
const streamChannelSize = 100

type server struct {
	CommonChannel          chan chat.StreamResponse
	ClientName             map[string]string
	ClientStream           map[string]chan chat.StreamResponse
	nameMutex, streamMutex sync.RWMutex
}

func (s *server) generateToken() string {

	txt := make([]byte, tokenSize)
	rand.Read(txt)

	return fmt.Sprintf("%x", txt)
}

func (s *server) addClientName(username string, tkn string) {

	s.nameMutex.RLock()
	defer s.nameMutex.RUnlock()
	s.ClientName[tkn] = username

}

func (s *server) getClientName(tkn string) (string, bool) {

	s.nameMutex.RLock()
	defer s.nameMutex.RUnlock()

	name, ok := s.ClientName[tkn]
	return name, ok
}

func (s *server) removeClientName(tkn string) string {

	s.nameMutex.RLock()
	defer s.nameMutex.RUnlock()

	username := s.ClientName[tkn]
	delete(s.ClientName, tkn)

	return username
}

func (s *server) Login(ctx context.Context, req *chat.LoginRequest) (*chat.LoginResponse, error) {

	// TODO: handle same name people in the chat
	// Generate a token
	tkn := s.generateToken()
	// Add the token in the client name
	s.addClientName(req.Username, tkn)
	// Send in a notif that broadcast is successful
	s.CommonChannel <- chat.StreamResponse{
		Timestamp: ptypes.TimestampNow(),
		Event: &chat.StreamResponse_ClientLogin{
			&chat.StreamResponse_Login{
				Name: req.Username,
			},
		},
	}

	// Return a response
	return &chat.LoginResponse{
		Token: tkn,
	}, nil

}

func (s *server) Logout(ctx context.Context, req *chat.LogoutRequest) (*chat.LogoutResponse, error) {

	tkn := req.Token
	// Remove the name from the Client Name map
	username := s.removeClientName(tkn)
	// Send in a broadcast that the client has been removed
	s.CommonChannel <- chat.StreamResponse{
		Timestamp: ptypes.TimestampNow(),
		Event: &chat.StreamResponse_ClientLogout{
			&chat.StreamResponse_Logout{
				Name: username,
			},
		},
	}
	// Return a response
	return &chat.LogoutResponse{}, nil
}

func (s *server) broadcast() {

	for res := range s.CommonChannel {

		s.streamMutex.RLock()
		for _, stream := range s.ClientStream {
			// Push in common message into specific client channel
			stream <- res
		}
		s.streamMutex.RUnlock()
	}
}

func (s *server) OpenStream(tkn string) chan chat.StreamResponse {
	stream := make(chan chat.StreamResponse, responseChannelSize)
	s.streamMutex.RLock()
	defer s.streamMutex.RUnlock()

	s.ClientStream[tkn] = stream
	return stream
}

func (s *server) CloseStream(tkn string) {
	s.streamMutex.RLock()
	defer s.streamMutex.RUnlock()

	if _, ok := s.ClientStream[tkn]; ok {
		delete(s.ClientStream, tkn)
	}
}

func (s *server) extractToken(ctx context.Context) (string, bool) {

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok || len(md[tokenHeader]) == 0 {
		return "", false
	}

	return md[tokenHeader][0], true
}

func (s *server) broadcastAll(srv_stream chat.Chat_StreamServer, tkn string) {

	stream := s.OpenStream(tkn)
	defer s.CloseStream(tkn)

	for {

		select {
		case <-srv_stream.Context().Done():
			return
		case res := <-stream:
			err := srv_stream.Send(&res)
			if err != nil {
				log.Fatalf("Broadcast to all is not working for %v", err)
			}
		}
	}
}

func (s *server) Stream(srv_stream chat.Chat_StreamServer) error {

	tkn, ok := s.extractToken(srv_stream.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "missing token header")
	}
	name, ok := s.getClientName(tkn)
	if !ok {
		return status.Error(codes.InvalidArgument, "username not found!")
	}

	// go routine for sending all individual client messages from individual clientQueue to the client
	go s.broadcastAll(srv_stream, tkn)
	// Receive messages and push it to the common queue
	for {

		req, err := srv_stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		s.CommonChannel <- chat.StreamResponse{
			Timestamp: ptypes.TimestampNow(),
			Event: &chat.StreamResponse_ClientMessage{
				&chat.StreamResponse_Message{
					Name:    name,
					Message: req.Message,
				},
			},
		}
	}

	<-srv_stream.Context().Done()
	return srv_stream.Context().Err()
}

func main() {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("Hello World")
	lis, err := net.Listen("tcp", "0.0.0.0:50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	customServer := server{
		CommonChannel: make(chan chat.StreamResponse, responseChannelSize),
		ClientName:    make(map[string]string),
		ClientStream:  make(map[string]chan chat.StreamResponse),
	}
	chat.RegisterChatServer(s, &customServer)

	// Have a go routine that would have a map of all channels and push all the messages from the commonChannel
	// to the individual specific client channel
	go customServer.broadcast()

	go func() {
		if err := s.Serve(lis); err != nil {
			log.Printf("failed to serve: %v", err)
			cancel()
		}
	}()
	<-ctx.Done()

	customServer.CommonChannel <- chat.StreamResponse{
		Timestamp: ptypes.TimestampNow(),
		Event:     &chat.StreamResponse_ServerShutdown{},
	}
	close(customServer.CommonChannel)
	s.GracefulStop()
}
