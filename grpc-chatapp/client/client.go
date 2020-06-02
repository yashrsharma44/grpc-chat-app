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

	chat "github.com/yashrsharma44/grpc-chat-app/grpc-chatapp/schema"
	"google.golang.org/grpc"
)

func doBiDiStreaming(c chat.ChatClient) {

	fmt.Println("Starting with a BiDi streaming...")

	// we create a stream by invoking the server
	stream, err := c.SubscribeMessage(context.Background(), new(chat.EmptyRequest))
	if err != nil {
		log.Fatalf("Error while creating Subscribe Stream: %v", err)
		return
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Enter your username :")
	username, _ := reader.ReadString('\n')
	username = strings.TrimSuffix(username, "\n")

	fmt.Println("Start entering your messages :P...")
	waitc := make(chan struct{})

	// we send a bunch of messages to the server(go routine)
	go func() {
		// function to send a bunch of messages

		for {

			text, _ := reader.ReadString('\n')
			text = strings.TrimSuffix(text, "\n")
			currTime := time.Now().Unix()
			request := &chat.Message{
				Username:  username,
				Timestamp: currTime,
				Content:   text,
			}
			// fmt.Printf("[%v|%v] : %v\n", currTime, username, text)
			_, err := c.SendMessage(context.Background(), request)
			if err != nil {
				log.Fatalf("Error while creating Send Message: %v", err)
				close(waitc)
			}

		}

	}()
	// we receive a bunch of messages from the server(go routine)
	go func() {
		// receive a bunch of messages

		for {
			res, err := stream.Recv()

			if err == io.EOF {
				close(waitc)
			}

			if err != nil {
				log.Fatalf("Error while receiving from server: %v", err)
				close(waitc)
			}
			user := strings.TrimSuffix(res.GetUsername(), "\n")
			tiStamp := res.GetTimestamp()
			conTent := res.GetContent()
			fmt.Printf("[%v|%v] : %v\n", tiStamp, user, conTent)
		}

	}()
	// block until everything is done
	<-waitc
}

func main() {

	fmt.Println("Hello, I'm a client")
	cc, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("could not connect: %v", err)
	}

	defer cc.Close()

	c := chat.NewChatClient(cc)
	fmt.Printf("Created client %f", c)

	doBiDiStreaming(c)

}
