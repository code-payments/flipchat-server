package flags

import (
	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codekin "github.com/code-payments/code-server/pkg/kin"
)

// todo: make these configurable
var (
	FeeDestination, _ = codecommon.NewAccountFromPublicKeyString("38u1jq3wpb8YGY5hPVZL7hRx7FUES4dGE9KR5XUeGC4b")

	StartGroupFee = codekin.ToQuarks(100)
)
