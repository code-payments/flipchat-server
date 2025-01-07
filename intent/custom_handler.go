package intent

import (
	"context"

	"google.golang.org/protobuf/proto"

	codeintent "github.com/code-payments/code-server/pkg/code/data/intent"
)

// todo: Move this stuff as part of code-server when we're ready.

type StatusCode uint8

const (
	SUCCESS StatusCode = iota
	INVALID
	DENIED
)

type ValidationResult struct {
	StatusCode       StatusCode
	ErrorDescription string
}

// todo: some way to register a handler to a proto URL
type CustomHandler interface {
	Validate(ctx context.Context, intentRecord *codeintent.Record, customMetadata proto.Message) (*ValidationResult, error)
}
