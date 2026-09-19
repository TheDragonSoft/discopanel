package services

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	storage "github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/pkg/logger"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
	"github.com/nickheyer/discopanel/pkg/proto/discopanel/v1/discopanelv1connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time check that AuditService implements the interface
var _ discopanelv1connect.AuditServiceHandler = (*AuditService)(nil)

// AuditService implements the Audit service
type AuditService struct {
	store *storage.Store
	log   *logger.Logger
}

// NewAuditService creates a new audit service
func NewAuditService(store *storage.Store, log *logger.Logger) *AuditService {
	return &AuditService{
		store: store,
		log:   log,
	}
}

// dbAuditEntryToProto converts a database audit entry to proto
func dbAuditEntryToProto(entry *storage.AuditEntry) *v1.AuditEntry {
	if entry == nil {
		return nil
	}
	return &v1.AuditEntry{
		Id:        entry.ID,
		UserId:    entry.UserID,
		Username:  entry.Username,
		Procedure: entry.Procedure,
		Resource:  entry.Resource,
		Action:    entry.Action,
		ObjectId:  entry.ObjectID,
		Status:    entry.Status,
		Detail:    entry.Detail,
		CreatedAt: timestamppb.New(entry.CreatedAt),
	}
}

// ListAuditEntries lists audit entries with filters and pagination
func (s *AuditService) ListAuditEntries(ctx context.Context, req *connect.Request[v1.ListAuditEntriesRequest]) (*connect.Response[v1.ListAuditEntriesResponse], error) {
	msg := req.Msg

	limit := int(msg.Limit)
	if limit <= 0 {
		limit = 100
	}

	entries, total, err := s.store.ListAuditEntries(ctx, msg.Username, msg.Resource, msg.Action, limit, int(msg.Offset))
	if err != nil {
		s.log.Error("Failed to list audit entries: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list audit entries"))
	}

	protoEntries := make([]*v1.AuditEntry, 0, len(entries))
	for _, entry := range entries {
		protoEntries = append(protoEntries, dbAuditEntryToProto(entry))
	}

	return connect.NewResponse(&v1.ListAuditEntriesResponse{
		Entries: protoEntries,
		Total:   int32(total),
	}), nil
}

// ClearAuditEntries deletes audit entries recorded before the given time (or
// all of them) and returns the deleted count
func (s *AuditService) ClearAuditEntries(ctx context.Context, req *connect.Request[v1.ClearAuditEntriesRequest]) (*connect.Response[v1.ClearAuditEntriesResponse], error) {
	var before *time.Time
	if req.Msg.Before != nil {
		t := req.Msg.Before.AsTime()
		before = &t
	}

	deleted, err := s.store.ClearAuditEntries(ctx, before)
	if err != nil {
		s.log.Error("Failed to clear audit entries: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to clear audit entries"))
	}

	s.log.Info("Cleared %d audit entries", deleted)

	return connect.NewResponse(&v1.ClearAuditEntriesResponse{
		Deleted: deleted,
	}), nil
}
