package query

import commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

type Option func(*Options)

func WithLimit(limit int) Option {
	return func(o *Options) {
		o.Limit = limit
	}
}

func WithToken(token *commonpb.PagingToken) Option {
	return func(o *Options) {
		o.Token = token
	}
}

func WithOrder(order commonpb.QueryOptions_Order) Option {
	return func(o *Options) {
		o.Order = order
	}
}

func WithAscending() Option {
	return func(o *Options) {
		o.Order = commonpb.QueryOptions_ASC
	}
}

func WithDescending() Option {
	return func(o *Options) {
		o.Order = commonpb.QueryOptions_DESC
	}
}

type Options struct {
	Limit int
	Token *commonpb.PagingToken
	Order commonpb.QueryOptions_Order
}

func DefaultOptions() Options {
	return Options{
		Limit: 100,
		Order: commonpb.QueryOptions_ASC,
	}
}
