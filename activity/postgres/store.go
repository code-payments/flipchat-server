package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	activitypb "github.com/code-payments/flipchat-protobuf-api/generated/go/activity/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/activity"
)

type store struct {
	pool *pgxpool.Pool
}

func NewInPostgres(pool *pgxpool.Pool) activity.Store {
	return &store{
		pool: pool,
	}
}

func (s *store) SaveNotification(ctx context.Context, activityFeedType activitypb.ActivityFeedType, userID *commonpb.UserId, notification *activitypb.Notification) (*activitypb.Notification, error) {
	model, err := toModel(activityFeedType, userID, notification)
	if err != nil {
		return nil, err
	}

	err = model.dbSave(ctx, s.pool)
	if err != nil {
		return nil, err
	}

	return fromModel(model)
}

func (s *store) GetLatestNotifications(ctx context.Context, activityFeedType activitypb.ActivityFeedType, userID *commonpb.UserId, limit int) ([]*activitypb.Notification, error) {
	if activityFeedType != activitypb.ActivityFeedType_TRANSACTION_HISTORY {
		return nil, activity.ErrInvalidActivityFeedType
	}

	models, err := dbGetLatestNotifications(ctx, s.pool, activityFeedType, userID, limit)
	if err != nil {
		return nil, err
	}

	res := make([]*activitypb.Notification, len(models))
	for i, model := range models {
		res[i], err = fromModel(model)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (s *store) reset() {
}
