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

	m.mu.Lock()
	defer m.mu.Unlock()

	var existing *activitypb.Notification
	for _, n := range m.notifications[string(userID.Value)] {
		if bytes.Equal(notification.Id.Value, n.Id.Value) {
			existing = n
			break
		}
	}

	switch typed := notification.AdditionalMetadata.(type) {
	case
		*activitypb.Notification_WelcomeBonus,
		*activitypb.Notification_WeeklyBonus,
		*activitypb.Notification_CreateGroup,
		*activitypb.Notification_SendListenerMessage:
	case *activitypb.Notification_SendTip:
		if existing != nil {
			existing.GetSendTip().TotalQuarksSent += typed.SendTip.TotalQuarksSent
		}
	case *activitypb.Notification_ReceivedTip:
		if existing != nil {
			existing.GetReceivedTip().TotalQuarksReceived += typed.ReceivedTip.TotalQuarksReceived
		}
	default:
		return nil, activity.ErrInvalidNotificationType
	}

	if existing == nil {
		existing = proto.Clone(notification).(*activitypb.Notification)
		m.notifications[string(userID.Value)] = append(m.notifications[string(userID.Value)], proto.Clone(notification).(*activitypb.Notification))
	}
	return proto.Clone(existing).(*activitypb.Notification), nil
}

func (m *InMemoryStore) GetLatestNotifications(ctx context.Context, activityFeedType activitypb.ActivityFeedType, userID *commonpb.UserId, limit int) ([]*activitypb.Notification, error) {
	if activityFeedType != activitypb.ActivityFeedType_TRANSACTION_HISTORY {
		return nil, activity.ErrInvalidActivityFeedType
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	res := protoutil.SliceClone(m.notifications[string(userID.Value)])

	sorted := NotificationsByTimestamp(res)
	sort.Sort(sort.Reverse(sorted))

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
