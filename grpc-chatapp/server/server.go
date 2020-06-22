package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/golang/protobuf/ptypes"
	chat "github.com/yashrsharma44/grpc-chat-app/grpc-chatapp/schema"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	tokenHeader         = "x-chat-token"
	tokenSize           = 4
	responseChannelSize = 20
	streamChannelSize   = 100
	grpcAddress         = "0.0.0.0:50051"
)

type server struct {
	CommonChannel          chan chat.StreamResponse
	ClientName             map[string]string
	ClientStream           map[string]chan chat.StreamResponse
	nameMutex, streamMutex sync.RWMutex
	logger                 log.Logger
}

func (s *server) generateToken() (string, error) {

	level.Debug(s.logger).Log("message", "started generating token")
	txt := make([]byte, tokenSize)
	_, err := rand.Read(txt)
	if err != nil {
		level.Error(s.logger).Log("error", "error while generating the token")
		return "", err
	}
	level.Debug(s.logger).Log("message", "finished generating token")
	return fmt.Sprintf("%x", txt), nil
}

func (s *server) addClientName(username string, tkn string) {

	s.nameMutex.RLock()
	defer s.nameMutex.RUnlock()
	level.Debug(s.logger).Log("message", "adding the client name", "client", username, "token", tkn)
	s.ClientName[tkn] = username

}

func (s *server) getClientName(tkn string) (string, bool) {

	s.nameMutex.RLock()
	defer s.nameMutex.RUnlock()
	level.Debug(s.logger).Log("message", "getting the client name", "token", tkn)
	name, ok := s.ClientName[tkn]
	return name, ok
}

func (s *server) removeClientName(tkn string) string {

	s.nameMutex.RLock()
	defer s.nameMutex.RUnlock()
	level.Debug(s.logger).Log("message", "removing the client token", "token", tkn)
	username := s.ClientName[tkn]
	delete(s.ClientName, tkn)

	return username
}

func (s *server) Login(ctx context.Context, req *chat.LoginRequest) (*chat.LoginResponse, error) {

	// TODO: handle same name people in the chat
	// Generate a token
	level.Info(s.logger).Log("message", "new client login request", "req", req)
	tkn, err := s.generateToken()
	if err != nil {
		level.Error(s.logger).Log("error", "login failed for the request", req)
	}
	// Add the token in the client name
	s.addClientName(req.Username, tkn)
	// Send in a notif that broadcast is successful
	level.Info(s.logger).Log("message", "login is successful", "req", req)
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

	level.Info(s.logger).Log("message", "new client logout request", "req", req)
	tkn := req.Token
	// Remove the name from the Client Name map
	username := s.removeClientName(tkn)
	// Send in a broadcast that the client has been removed
	level.Info(s.logger).Log("message", "logout is successful", "req", req)
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
	stream := make(chan chat.StreamResponse, streamChannelSize)
	s.streamMutex.RLock()
	defer s.streamMutex.RUnlock()
	level.Debug(s.logger).Log("message", "opening the stream", "token", tkn)
	s.ClientStream[tkn] = stream
	return stream
}

func (s *server) CloseStream(tkn string) {
	s.streamMutex.RLock()
	defer s.streamMutex.RUnlock()
	level.Debug(s.logger).Log("message", "closing the stream", "token", tkn)
	delete(s.ClientStream, tkn)
}

func (s *server) extractToken(ctx context.Context) (string, bool) {

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok || len(md[tokenHeader]) == 0 {
		return "", false
	}
	tkn := md[tokenHeader][0]
	level.Debug(s.logger).Log("message", "successfully extracted the token", "token", tkn)
	return tkn, true
}

func (s *server) broadcastAll(srv_stream chat.Chat_StreamServer, tkn string) {

	stream := s.OpenStream(tkn)
	defer s.CloseStream(tkn)

	s.nameMutex.RLock()
	level.Info(s.logger).Log("message", "started the broadcast for the given client", "username", s.ClientName[tkn])
	s.nameMutex.RUnlock()

	for {

		select {
		case <-srv_stream.Context().Done():
			s.nameMutex.RLock()
			level.Info(s.logger).Log("message", "closing the broadcast for the given client", "username", s.ClientName[tkn])
			s.nameMutex.RUnlock()
			return

		case res := <-stream:
			err := srv_stream.Send(&res)
			if err != nil {
				level.Error(s.logger).Log("error", "error while sending the stream", err)
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
			s.nameMutex.RLock()
			level.Info(s.logger).Log("message", "client disconnected, closing..", "username", s.ClientName[tkn])
			s.nameMutex.RUnlock()
			break
		}
		if err != nil {
			level.Error(s.logger).Log("error", "error while receiving ", "err", err)
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

func handleSigterm(c chan os.Signal, cancel context.CancelFunc) {
	<-c
	cancel()
}

func main() {

	// Initialise the initial setup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// sigterm handler
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go handleSigterm(c, cancel)

	// Initialise the logger
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout))
	logger = level.NewFilter(logger, level.AllowInfo())
	logger = log.With(logger, "ts", time.Now().Format(time.RFC1123), "caller", log.DefaultCaller)

	level.Info(logger).Log("message", "server started listening")

	lis, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		level.Error(logger).Log("error", "failed to listen the server, exiting..")
		os.Exit(1)
	}

	s := grpc.NewServer()

	customServer := server{
		CommonChannel: make(chan chat.StreamResponse, responseChannelSize),
		ClientName:    make(map[string]string),
		ClientStream:  make(map[string]chan chat.StreamResponse),
		logger:        logger,
	}
	chat.RegisterChatServer(s, &customServer)
	level.Debug(logger).Log("message", "registered the server")
	// Have a go routine that would have a map of all channels and push all the messages from the commonChannel
	// to the individual specific client channel
	level.Debug(logger).Log("message", "started the broadcast of messages")
	go customServer.broadcast()

	go func() {
		if err := s.Serve(lis); err != nil {
			level.Error(logger).Log("error", "failed to listen the server, exiting..")
			cancel()
		}
	}()

	<-ctx.Done()
	level.Info(logger).Log("message", "sending shutdown notification")
	customServer.CommonChannel <- chat.StreamResponse{
		Timestamp: ptypes.TimestampNow(),
		Event:     &chat.StreamResponse_ServerShutdown{},
	}
	level.Info(logger).Log("message", "closing the channel")
	close(customServer.CommonChannel)
	level.Info(logger).Log("message", "graceful shutdown")
	s.GracefulStop()
}
