package queue

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gabinante/flywheel/internal/ticket"
	"github.com/redis/go-redis/v9"
)

func TestClaimTicket_IdempotencyKey_ReuseLease(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	redisStore := NewRedisStore(rdb, 5*time.Minute)

	t1 := &ticket.Ticket{
		ID:        "proj-1",
		ProjectID: "project-id",
		State:     ticket.StateDraft,
		Priority:  ticket.P2,
		DependsOn: nil,
	}
	mockList := &mockTicketLister{tickets: map[string]*ticket.Ticket{"proj-1": t1}, listByState: []*ticket.Ticket{t1}}
	mockTrans := &mockTransitioner{}

	svc := NewService(mockTrans, mockList, redisStore)
	ctx := context.Background()
	agentID := "agent1"
	projectID := "project-id"
	key := "idem-key"

	// First claim: should create lease and store idempotency mapping
	ticket1, lease1, err := svc.ClaimTicket(ctx, agentID, projectID, nil, key)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if ticket1.ID != "proj-1" || lease1.TicketID != "proj-1" || lease1.AgentID != agentID {
		t.Errorf("first claim: got ticket %s lease %+v", ticket1.ID, lease1)
	}

	// Second claim with same key: should renew and return same ticket/lease
	ticket2, lease2, err := svc.ClaimTicket(ctx, agentID, projectID, nil, key)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if ticket2.ID != "proj-1" || lease2.TicketID != "proj-1" {
		t.Errorf("second claim: got ticket %s lease ticket %s", ticket2.ID, lease2.TicketID)
	}
	if lease2.Token != lease1.Token {
		t.Errorf("expected same lease token, got %s vs %s", lease2.Token, lease1.Token)
	}
	if !lease2.ExpiresAt.After(lease1.ExpiresAt) {
		t.Errorf("expected renewed expiry after first; got %v vs %v", lease2.ExpiresAt, lease1.ExpiresAt)
	}
}

type mockTicketLister struct {
	tickets     map[string]*ticket.Ticket
	listByState []*ticket.Ticket
}

func (m *mockTicketLister) ListByState(ctx context.Context, projectID string, state ticket.State) ([]*ticket.Ticket, error) {
	if state != ticket.StateDraft {
		return nil, nil
	}
	return m.listByState, nil
}

func (m *mockTicketLister) GetTicket(ctx context.Context, id string) (*ticket.Ticket, error) {
	return m.tickets[id], nil
}

func (m *mockTicketLister) GetTicketsByIDs(ctx context.Context, ids []string) ([]*ticket.Ticket, error) {
	return nil, nil
}

type mockTransitioner struct{}

func (m *mockTransitioner) TransitionTicket(ctx context.Context, id string, trigger string, actor ticket.Actor, payload map[string]any) error {
	return nil
}
