package protoutil

import (
	"fmt"

	"google.golang.org/protobuf/proto"
)

func SliceEqualError[T proto.Message](a, b []T) error {
	if len(a) != len(b) {
		return fmt.Errorf("len(%d) != len(%d)", len(a), len(b))
	}

	for i := 0; i < len(a); i++ {
		if err := ProtoEqualError(a[i], b[i]); err != nil {
			return fmt.Errorf("mismatch[%d]: %w", i, err)
		}
	}

	return nil
}

func ProtoEqualError(a, b proto.Message) error {
	if !proto.Equal(a, b) {
		return fmt.Errorf("expected: %v\nactual: %v\n", a, b)
	}

	return nil
}

func SliceClone[T proto.Message](src []T) []T {
	cloned := make([]T, len(src))
	for i := range src {
		cloned[i] = proto.Clone(src[i]).(T)
	}
	return cloned
}
