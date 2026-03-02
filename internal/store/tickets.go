package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oriys/nova/internal/domain"
)

// TicketUpdate contains mutable ticket fields for PATCH operations.
type TicketUpdate struct {
	Status     *domain.TicketStatus   `json:"status,omitempty"`
	Priority   *domain.TicketPriority `json:"priority,omitempty"`
	AssignedTo *string                `json:"assigned_to,omitempty"`
}

// CreateTicket persists a new ticket. The creator tenant/namespace are taken
// from the context scope.
func (s *PostgresStore) CreateTicket(ctx context.Context, t *domain.Ticket) error {
	if t == nil {
		return fmt.Errorf("ticket is required")
	}
	if strings.TrimSpace(t.ID) == "" {
		return fmt.Errorf("ticket id is required")
	}
	if strings.TrimSpace(t.Title) == "" {
		return fmt.Errorf("ticket title is required")
	}

	scope := tenantScopeFromContext(ctx)
	t.CreatorTenantID = scope.TenantID
	t.CreatorNamespace = scope.Namespace

	now := time.Now()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = domain.TicketStatusOpen
	}
	if t.Priority == "" {
		t.Priority = domain.TicketPriorityMedium
	}
	if t.Category == "" {
		t.Category = domain.TicketCategoryGeneral
	}

	data, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal ticket: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO tickets (id, creator_tenant_id, creator_namespace, data, created_at, updated_at)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6)
	`, t.ID, t.CreatorTenantID, t.CreatorNamespace, data, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create ticket: %w", err)
	}
	return nil
}

// GetTicket retrieves a ticket by ID. The default tenant can read any ticket;
// other tenants can only read their own.
func (s *PostgresStore) GetTicket(ctx context.Context, id string) (*domain.Ticket, error) {
	scope := tenantScopeFromContext(ctx)

	var query string
	var args []interface{}
	if scope.TenantID == DefaultTenantID {
		query = `SELECT data FROM tickets WHERE id = $1`
		args = []interface{}{id}
	} else {
		query = `SELECT data FROM tickets WHERE id = $1 AND creator_tenant_id = $2`
		args = []interface{}{id, scope.TenantID}
	}

	var data []byte
	err := s.pool.QueryRow(ctx, query, args...).Scan(&data)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("ticket not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get ticket: %w", err)
	}

	var t domain.Ticket
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("unmarshal ticket: %w", err)
	}
	return &t, nil
}

// ListTickets returns paginated tickets. The default tenant sees all tickets;
// other tenants see only their own.
func (s *PostgresStore) ListTickets(ctx context.Context, limit, offset int) ([]*domain.Ticket, int, error) {
	scope := tenantScopeFromContext(ctx)

	var countQuery, dataQuery string
	var args []interface{}
	if scope.TenantID == DefaultTenantID {
		countQuery = `SELECT count(*) FROM tickets`
		dataQuery = `SELECT data FROM tickets ORDER BY created_at DESC LIMIT $1 OFFSET $2`
		args = []interface{}{limit, offset}
	} else {
		countQuery = `SELECT count(*) FROM tickets WHERE creator_tenant_id = $1`
		dataQuery = `SELECT data FROM tickets WHERE creator_tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
		args = []interface{}{scope.TenantID, limit, offset}
	}

	var total int
	if scope.TenantID == DefaultTenantID {
		if err := s.pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("count tickets: %w", err)
		}
	} else {
		if err := s.pool.QueryRow(ctx, countQuery, scope.TenantID).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("count tickets: %w", err)
		}
	}

	rows, err := s.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list tickets: %w", err)
	}
	defer rows.Close()

	var tickets []*domain.Ticket
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, 0, fmt.Errorf("scan ticket: %w", err)
		}
		var t domain.Ticket
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, 0, fmt.Errorf("unmarshal ticket: %w", err)
		}
		tickets = append(tickets, &t)
	}
	return tickets, total, nil
}

// UpdateTicket applies a partial update. The default tenant can update any
// ticket; other tenants can only update their own.
func (s *PostgresStore) UpdateTicket(ctx context.Context, id string, update *TicketUpdate) (*domain.Ticket, error) {
	t, err := s.GetTicket(ctx, id)
	if err != nil {
		return nil, err
	}

	if update.Status != nil {
		t.Status = *update.Status
		if t.Status == domain.TicketStatusClosed {
			now := time.Now()
			t.ClosedAt = &now
		}
	}
	if update.Priority != nil {
		t.Priority = *update.Priority
	}
	if update.AssignedTo != nil {
		t.AssignedTo = *update.AssignedTo
	}
	t.UpdatedAt = time.Now()

	data, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("marshal ticket: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE tickets SET data = $1::jsonb, updated_at = $2 WHERE id = $3
	`, data, t.UpdatedAt, id)
	if err != nil {
		return nil, fmt.Errorf("update ticket: %w", err)
	}
	return t, nil
}

// DeleteTicket removes a ticket. Only the default tenant can delete tickets.
func (s *PostgresStore) DeleteTicket(ctx context.Context, id string) error {
	scope := tenantScopeFromContext(ctx)
	if scope.TenantID != DefaultTenantID {
		return fmt.Errorf("only the default tenant can delete tickets")
	}

	tag, err := s.pool.Exec(ctx, `DELETE FROM tickets WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete ticket: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("ticket not found: %s", id)
	}
	return nil
}

// CreateTicketComment adds a comment to a ticket.
func (s *PostgresStore) CreateTicketComment(ctx context.Context, c *domain.TicketComment) error {
	if c == nil {
		return fmt.Errorf("comment is required")
	}
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("comment id is required")
	}
	if strings.TrimSpace(c.TicketID) == "" {
		return fmt.Errorf("ticket_id is required")
	}
	if strings.TrimSpace(c.Content) == "" {
		return fmt.Errorf("comment content is required")
	}

	// Verify the ticket exists and is accessible
	if _, err := s.GetTicket(ctx, c.TicketID); err != nil {
		return err
	}

	scope := tenantScopeFromContext(ctx)
	c.AuthorTenantID = scope.TenantID
	c.AuthorNamespace = scope.Namespace
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}

	// Only default tenant can create internal comments
	if c.Internal && scope.TenantID != DefaultTenantID {
		c.Internal = false
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO ticket_comments (id, ticket_id, author_tenant_id, author_namespace, content, internal, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, c.ID, c.TicketID, c.AuthorTenantID, c.AuthorNamespace, c.Content, c.Internal, c.CreatedAt)
	if err != nil {
		return fmt.Errorf("create ticket comment: %w", err)
	}
	return nil
}

// ListTicketComments returns comments for a ticket. Non-default tenants
// cannot see internal comments.
func (s *PostgresStore) ListTicketComments(ctx context.Context, ticketID string, limit, offset int) ([]*domain.TicketComment, int, error) {
	// Verify the ticket exists and is accessible
	if _, err := s.GetTicket(ctx, ticketID); err != nil {
		return nil, 0, err
	}

	scope := tenantScopeFromContext(ctx)
	isDefault := scope.TenantID == DefaultTenantID

	var countQuery, dataQuery string
	var countArgs, dataArgs []interface{}
	if isDefault {
		countQuery = `SELECT count(*) FROM ticket_comments WHERE ticket_id = $1`
		countArgs = []interface{}{ticketID}
		dataQuery = `SELECT id, ticket_id, author_tenant_id, author_namespace, content, internal, created_at
			FROM ticket_comments WHERE ticket_id = $1 ORDER BY created_at ASC LIMIT $2 OFFSET $3`
		dataArgs = []interface{}{ticketID, limit, offset}
	} else {
		countQuery = `SELECT count(*) FROM ticket_comments WHERE ticket_id = $1 AND internal = FALSE`
		countArgs = []interface{}{ticketID}
		dataQuery = `SELECT id, ticket_id, author_tenant_id, author_namespace, content, internal, created_at
			FROM ticket_comments WHERE ticket_id = $1 AND internal = FALSE ORDER BY created_at ASC LIMIT $2 OFFSET $3`
		dataArgs = []interface{}{ticketID, limit, offset}
	}

	var total int
	if err := s.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count ticket comments: %w", err)
	}

	rows, err := s.pool.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list ticket comments: %w", err)
	}
	defer rows.Close()

	var comments []*domain.TicketComment
	for rows.Next() {
		var c domain.TicketComment
		if err := rows.Scan(&c.ID, &c.TicketID, &c.AuthorTenantID, &c.AuthorNamespace, &c.Content, &c.Internal, &c.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan ticket comment: %w", err)
		}
		comments = append(comments, &c)
	}
	return comments, total, nil
}
