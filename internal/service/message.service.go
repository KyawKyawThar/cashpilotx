package service

import (
	db "cashpilotx/db/sqlc"
	"cashpilotx/internal/api"
	"cashpilotx/internal/dto"
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

func (s *MessageService) ListMessages(
	ctx context.Context,
	invoiceID uuid.UUID,
) ([]*dto.ChatMessageResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.store.ListMessagesByInvoice(dbCtx, &invoiceID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.ChatMessageResponse, 0, len(rows))
	for _, m := range rows {
		out = append(out, dto.ToChatMessageResponse(m))
	}
	return out, nil
}

func (s *MessageService) CreateManualMessage(
	ctx context.Context,
	userID uuid.UUID,
	req *dto.CreateManualMessageRequest,
) (*dto.ChatMessageResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	tone := req.Tone
	if tone == "" {
		tone = defaultTone
	}

	msg, err := s.store.CreateMessage(dbCtx, db.CreateMessageParams{
		UserID:     userID,
		InvoiceID:  &req.InvoiceID,
		CustomerID: req.CustomerID,
		Direction:  "outbound",
		Channel:    req.Channel,
		Content:    req.Content,
		Tone:       tone,
		Column8:    "pending", // status — COALESCE(NULLIF($8::text,'')::message_status, 'draft')
		Column9:    "human",   // source — COALESCE(NULLIF($9::text,'')::message_source, 'ai')
		Column10:   json.RawMessage("{}"),
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return dto.ToChatMessageResponse(msg), nil
}

func (s *MessageService) ApproveMessage(
	ctx context.Context,
	messageID uuid.UUID,
) (*dto.ChatMessageResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	msg, err := s.store.ApproveMessage(dbCtx, messageID)
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrNotFound("message not found or not in draft state")
		}
		return nil, api.FromPgError(err)
	}

	return dto.ToChatMessageResponse(msg), nil
}

func (s *MessageService) MarkMessageSent(
	ctx context.Context,
	messageID uuid.UUID,
	req *dto.MarkMessageSentRequest,
) (*dto.ChatMessageResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	msg, err := s.store.MarkMessageSent(dbCtx, db.MarkMessageSentParams{
		ID:                messageID,
		ExternalMessageID: req.ExternalMessageID,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrNotFound("message not found")
		}
		return nil, api.FromPgError(err)
	}

	return dto.ToChatMessageResponse(msg), nil
}

func (s *MessageService) GetCopyText(
	ctx context.Context,
	messageID uuid.UUID,
) (*dto.CopyTextResponse, error) {
	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	msg, err := s.store.GetMessageByID(dbCtx, messageID)
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrNotFound("message not found")
		}
		return nil, api.FromPgError(err)
	}

	return &dto.CopyTextResponse{
		MessageID: msg.ID,
		Content:   msg.Content,
	}, nil
}
