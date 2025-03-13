package logging

import (
	"context"

	"go.uber.org/zap"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/model"
)

type StaffLogger struct {
	log *zap.Logger
}

func NewStaffLogger(ctx context.Context, log *zap.Logger, userID *commonpb.UserId, accounts account.Store) *StaffLogger {
	isStaff, _ := accounts.IsStaff(ctx, userID)
	if !isStaff {
		return nil
	}

	log = log.With(
		zap.String("user_id", model.UserIDString(userID)),
	)

	return &StaffLogger{
		log: log,
	}
}

func (l *StaffLogger) Info(msg string, fields ...zap.Field) {
	if l == nil {
		return
	}
	l.log.Info(msg, fields...)
}

func (l *StaffLogger) With(fields ...zap.Field) *StaffLogger {
	if l == nil {
		return nil
	}
	l.log = l.log.With(fields...)
	return l
}
