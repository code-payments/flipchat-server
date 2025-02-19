package logging

import (
	"context"
	"sync"

	"go.uber.org/zap"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/model"
)

var (
	cachedStaffFlagMu sync.RWMutex
	cachedStaffFlag   map[string]bool
)

func init() {
	cachedStaffFlag = make(map[string]bool)
}

type StaffLogger struct {
	log *zap.Logger
}

func NewStaffLogger(ctx context.Context, log *zap.Logger, userID *commonpb.UserId, accounts account.Store) *StaffLogger {
	isStaff := isStaff(ctx, userID, accounts)
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

func isStaff(ctx context.Context, userID *commonpb.UserId, accounts account.Store) bool {
	cacheKey := model.UserIDString(userID)

	cachedStaffFlagMu.RLock()
	isStaff, ok := cachedStaffFlag[cacheKey]
	if ok {
		cachedStaffFlagMu.RUnlock()
		return isStaff
	}
	cachedStaffFlagMu.RUnlock()

	isStaff, err := accounts.IsStaff(ctx, userID)
	if err != nil {
		return false
	}

	cachedStaffFlagMu.Lock()
	cachedStaffFlag[cacheKey] = isStaff
	cachedStaffFlagMu.Unlock()

	return isStaff
}
