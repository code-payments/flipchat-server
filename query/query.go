package query

import (
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	"github.com/code-payments/flipchat-server/database/prisma/db"
)

type Option func(*Options)

func WithLimit(limit int) Option {
	return func(o *Options) {
		if limit > 0 {
			o.Limit = limit
		}
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

func ApplyOptions(options ...Option) Options {
	applied := DefaultOptions()
	for _, option := range options {
		option(&applied)
	}
	return applied
}

func FromProtoOptions(protoOptions *commonpb.QueryOptions) []Option {
	if protoOptions == nil {
		return nil
	}

	options := []Option{WithOrder(protoOptions.Order)}

	if protoOptions.PageSize > 0 {
		options = append(options, WithLimit(int(protoOptions.PageSize)))
	}

	if protoOptions.PagingToken != nil {
		options = append(options, WithToken(protoOptions.PagingToken))
	}

	return options
}

func ToPrismaSortOrder(protoSortOrder commonpb.QueryOptions_Order) db.SortOrder {
	switch protoSortOrder {
	case commonpb.QueryOptions_DESC:
		return db.SortOrderDesc
	default:
		return db.SortOrderAsc
	}
}
