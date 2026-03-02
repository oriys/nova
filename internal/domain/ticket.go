package domain

import "time"

// TicketStatus represents the lifecycle state of a ticket.
type TicketStatus string

const (
	TicketStatusOpen       TicketStatus = "open"
	TicketStatusInProgress TicketStatus = "in_progress"
	TicketStatusResolved   TicketStatus = "resolved"
	TicketStatusClosed     TicketStatus = "closed"
)

// TicketPriority represents the urgency of a ticket.
type TicketPriority string

const (
	TicketPriorityLow      TicketPriority = "low"
	TicketPriorityMedium   TicketPriority = "medium"
	TicketPriorityHigh     TicketPriority = "high"
	TicketPriorityCritical TicketPriority = "critical"
)

// TicketCategory classifies the type of request.
type TicketCategory string

const (
	TicketCategoryGeneral        TicketCategory = "general"
	TicketCategoryQuotaRequest   TicketCategory = "quota_request"
	TicketCategoryIncident       TicketCategory = "incident"
	TicketCategoryFeatureRequest TicketCategory = "feature_request"
	TicketCategoryBugReport      TicketCategory = "bug_report"
)

// Ticket represents a work order submitted by a tenant.
//
// The default tenant can view and manage tickets from all tenants.
// Non-default tenants can only view and manage their own tickets.
type Ticket struct {
	ID              string         `json:"id"`
	Title           string         `json:"title"`
	Description     string         `json:"description"`
	Status          TicketStatus   `json:"status"`
	Priority        TicketPriority `json:"priority"`
	Category        TicketCategory `json:"category"`
	CreatorTenantID string         `json:"creator_tenant_id"`
	CreatorNamespace string        `json:"creator_namespace"`
	AssignedTo      string         `json:"assigned_to,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	ClosedAt        *time.Time     `json:"closed_at,omitempty"`
}

// TicketComment represents a reply on a ticket.
type TicketComment struct {
	ID              string    `json:"id"`
	TicketID        string    `json:"ticket_id"`
	AuthorTenantID  string    `json:"author_tenant_id"`
	AuthorNamespace string    `json:"author_namespace"`
	Content         string    `json:"content"`
	Internal        bool      `json:"internal"`
	CreatedAt       time.Time `json:"created_at"`
}
