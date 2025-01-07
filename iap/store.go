package iap

import (
	"context"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

var (
	ErrExists   = errors.New("iap already exists")
	ErrNotFound = errors.New("iap not found")
)

type Product uint8

const (
	ProductUnknown Product = iota
	ProductCreateAccount
)

type State uint8

const (
	StateUnknown State = iota
	StateWaitingForPayment
	StateWaitingForFulfillment
	StateFulfilled
)

type Purchase struct {
	ReceiptID []byte
	Platform  commonpb.Platform
	User      *commonpb.UserId
	Product   Product
	State     State
	CreatedAt time.Time
}

type Store interface {
	CreatePurchase(ctx context.Context, purchase *Purchase) error
	GetPurchase(ctx context.Context, receiptId []byte) (*Purchase, error)
}

func (p *Purchase) Clone() *Purchase {
	return &Purchase{
		ReceiptID: p.ReceiptID,
		Platform:  p.Platform,
		User:      proto.Clone(p.User).(*commonpb.UserId),
		Product:   p.Product,
		State:     p.State,
		CreatedAt: p.CreatedAt,
	}
}
