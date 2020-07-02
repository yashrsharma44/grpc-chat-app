package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"
	chat "github.com/yashrsharma44/grpc-chat-app/grpc-chatapp/schema"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const tokenHeader = "x-chat-token"

type client struct {
	chat.ChatClient
	Name, Token string
}

func Client() *client {
	return &client{}
}

func (c *client) login() (string, error) {
	ctx := context.Background()

	res, err := c.ChatClient.Login(ctx, &chat.LoginRequest{
		Username: c.Name,
	})
	if err != nil {
		return "", nil
	}

	return res.Token, nil
}

func (c *client) logout() error {

	ctx := context.Background()
	_, err := c.ChatClient.Logout(ctx, &chat.LogoutRequest{
		Token: c.Token,
	})

	return err

}

func (c *client) send(client chat.Chat_StreamClient) {

	reader := bufio.NewReader(os.Stdin)

	for {

		select {
		case <-client.Context().Done():
			fmt.Println("client send loop disconnected")
		default:
			txt, _ := reader.ReadString('\n')
			message := strings.Trim(txt, "\n")
			err := client.Send(&chat.StreamRequest{Message: message, Name: c.Name})
			if err != nil {
				log.Fatalf("failed to send message %v", err)
			}
		}
	}

}

func (c *client) receive(client chat.Chat_StreamClient) {

	for {
		res, err := client.Recv()

		if err == io.EOF {
			fmt.Printf("Stream closed by server")
			return
		}

		if err != nil {
			log.Fatalf("Unknown Error at receive : %v", err)
			return
		}

		ts := res.Timestamp
		var tm time.Time
		t, err := ptypes.Timestamp(ts)
		if err != nil {
			tm = time.Now()
		}
		tm = t.In(time.Local)

		switch evnt := res.Event.(type) {
		case *chat.StreamResponse_ClientMessage:
			fmt.Printf("[%v|%v] %v\n", tm, evnt.ClientMessage.Name, evnt.ClientMessage.Message)
		case *chat.StreamResponse_ServerShutdown:
			fmt.Printf("%v --- the server is shutting down\n", tm)
		default:
			fmt.Println("Waiting for connection..")
		}

	}
}

func (c *client) stream() {

	md := metadata.New(map[string]string{tokenHeader: c.Token})
	ctx := context.Background()
	ctx = metadata.NewOutgoingContext(ctx, md)

	client, err := c.ChatClient.Stream(ctx)
	if err != nil {
		log.Fatalf("Error on stream : %v", err)
	}

	defer client.CloseSend()
	go c.send(client)
	c.receive(client)
}

func main() {

	fmt.Println("Hello, I'm a client")
	cc, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("could not connect: %v", err)
	}

	defer cc.Close()

	c := Client()
	c.ChatClient = chat.NewChatClient(cc)

	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Enter your username:")
	username, _ := reader.ReadString('\n')

	c.Name = strings.Trim(username, "\n")
	c.Token, err = c.login()
	if err != nil {
		log.Fatalf("failed to login %v", err)
	}
	fmt.Println("Start Chatting :D..")
	c.stream()
	fmt.Println("Logging out..")
	if err := c.logout(); err != nil {
		log.Fatalf("Failed to logout.. %v", err)
	}

}
