package models

import "time"

// AuditEvent представляет одно событие аудита.
// Теги `json` используются для сериализации/десериализации в Kafka и OpenSearch.
type AuditEvent struct {
	EventID   string                 `json:"event_id"`
	Timestamp time.Time              `json:"timestamp"`
	Status    string                 `json:"status"`
	EventType string                 `json:"event_type"`
	Actor     Actor                  `json:"actor"`
	Entity    Entity                 `json:"entity"`
	Context   Context                `json:"context"`
	Security  *Security              `json:"security,omitempty"` // <--- НОВОЕ ПОЛЕ
	Details   map[string]interface{} `json:"details,omitempty"`
}

type Actor struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
}

type Entity struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
}

type Context struct {
	SourceService string `json:"source_service"`
	TraceID       string `json:"trace_id"`
	RequestID     string `json:"request_id"`
}

// Security содержит данные, связанные с безопасностью действия
type Security struct { // <--- НОВАЯ СТРУКТУРА
	AccessLevel string `json:"access_level"` // Например, "LOW", "MEDIUM", "HIGH", "CRITICAL"
	// Можно добавить другие поля, например:
	// UserRoles   []string `json:"user_roles,omitempty"`
	// TenantID    string   `json:"tenant_id,omitempty"`
}

type AuditEventConnection struct {
	Events []*AuditEvent `json:"events"`
	Total  int           `json:"total"`
}

type AuditEventFilter struct {
	Status              *string `json:"status,omitempty"`
	EventType           *string `json:"eventType,omitempty"`
	ActorID             *string `json:"actorId,omitempty"`
	EntityID            *string `json:"entityId,omitempty"`
	SecurityAccessLevel *string `json:"securityAccessLevel,omitempty"` // <--- НОВОЕ ПОЛЕ ДЛЯ ФИЛЬТРА
}

type Query struct {
}
