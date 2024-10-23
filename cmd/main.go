package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"

	accountpb "github.com/code-payments/flipchat-protobuf-api/generated/go/account/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

func main() {
	cc, err := grpc.NewClient("api.flipchat.codeinfra.net:443", grpc.WithTransportCredentials(credentials.NewTLS(nil)))
	if err != nil {
		log.Fatal("Failed to create connection:", err)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatal("Failed to generate key:", err)
	}

	req := &accountpb.RegisterRequest{
		DisplayName: "test",
		PublicKey:   &commonpb.PublicKey{Value: pub[:]},
	}
	reqBytes, err := proto.Marshal(req)
	if err != nil {
		log.Fatal("Failed to marshal request:", err)
	}

	sig := ed25519.Sign(priv, reqBytes)
	req.Signature = &commonpb.Signature{Value: sig[:]}

	client := accountpb.NewAccountClient(cc)
	resp, err := client.Register(context.Background(), req)

	fmt.Println("Result:", resp, err)
}
