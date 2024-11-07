package main

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/timestamppb"

	accountpb "github.com/code-payments/flipchat-protobuf-api/generated/go/account/v1"
	"github.com/code-payments/flipchat-server/model"
)

func main() {
	cc, err := grpc.NewClient("api.flipchat.codeinfra.net:443", grpc.WithTransportCredentials(credentials.NewTLS(nil)))
	if err != nil {
		log.Fatal("Failed to create connection:", err)
	}

	keyPair := model.MustGenerateKeyPair()

	register := &accountpb.RegisterRequest{
		DisplayName: "test",
		PublicKey:   keyPair.Proto(),
	}
	if err := keyPair.Sign(register, &register.Signature); err != nil {
		log.Fatal("failed to sign:", err)
	}

	login := &accountpb.LoginRequest{
		Timestamp: timestamppb.Now(),
	}
	if err := keyPair.Auth(login, &login.Auth); err != nil {
		log.Fatal("failed to sign:", err)
	}

	client := accountpb.NewAccountClient(cc)
	resp, err := client.Register(context.Background(), register)
	fmt.Println("Register Result:", resp, err)

	le, err := client.Login(context.Background(), login)
	fmt.Println("Login Result:", le, err)

}
