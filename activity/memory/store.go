package memory

import (
	"bytes"
	"context"
	"sort"
	"sync"

	activitypb "github.com/code-payments/flipchat-protobuf-api/generated/go/activity/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	"google.golang.org/protobuf/proto"

	"github.com/code-payments/flipchat-server/activity"
	"github.com/code-payments/flipchat-server/protoutil"
)

type NotificationsByTimestamp []*activitypb.Notification

func (a NotificationsByTimestamp) Len() int      { return len(a) }
func (a NotificationsByTimestamp) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a NotificationsByTimestamp) Less(i, j int) bool {
	return a[i].Ts.AsTime().Before(a[j].Ts.AsTime())
}

type InMemoryStore struct {
	mu            sync.RWMutex
	notifications map[string][]*activitypb.Notification
}

func NewInMemory() activity.Store {
	return &InMemoryStore{
		notifications: map[string][]*activitypb.Notification{},
	}
}

func (m *InMemoryStore) SaveNotification(ctx context.Context, activityFeedType activitypb.ActivityFeedType, userID *commonpb.UserId, notification *activitypb.Notification) (*activitypb.Notification, error) {
	if activityFeedType != activitypb.ActivityFeedType_TRANSACTION_HISTORY {
		return nil, activity.ErrInvalidActivityFeedType
	}

	switch notification.AdditionalMetadata.(type) {
	case
		*activitypb.Notification_WelcomeBonus,
		*activitypb.Notification_WeeklyBonus:
		//*activitypb.Notification_CreateGroup,
		//*activitypb.Notification_SendListenerMessage,
		//*activitypb.Notification_SendTip,
		//*activitypb.Notification_ReceivedTip,
		//*activitypb.Notification_PromotedToSpeaker,
	default:
		return nil, activity.ErrInvalidNotificationType
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, existing := range m.notifications[string(userID.Value)] {
		if bytes.Equal(notification.Id.Value, existing.Id.Value) {
			return proto.Clone(existing).(*activitypb.Notification), nil
		}
	}

	m.notifications[string(userID.Value)] = append(m.notifications[string(userID.Value)], proto.Clone(notification).(*activitypb.Notification))

	return proto.Clone(notification).(*activitypb.Notification), nil
}

func (m *InMemoryStore) GetLatestNotifications(ctx context.Context, activityFeedType activitypb.ActivityFeedType, userID *commonpb.UserId, limit int) ([]*activitypb.Notification, error) {
	if activityFeedType != activitypb.ActivityFeedType_TRANSACTION_HISTORY {
		return nil, activity.ErrInvalidActivityFeedType
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	res := protoutil.SliceClone(m.notifications[string(userID.Value)])

	sorted := NotificationsByTimestamp(res)
	sort.Sort(sorted)

	if len(sorted) > limit {
		sorted = sorted[:limit]
	}

	return sorted, nil
}

func (m *InMemoryStore) reset() {
	m.mu.Lock()
	m.notifications = map[string][]*activitypb.Notification{}
	m.mu.Unlock()
}
