package graphql

//go:generate go run github.com/99designs/gqlgen generate

import (
	"context"
	"witness/graphql/generated"
	"witness/models"
	"witness/opensearch"
)

// Resolver - корневой резолвер.
type Resolver struct {
	OSClient *opensearch.Client
}

// Query возвращает QueryResolver.
func (r *Resolver) Query() generated.QueryResolver {
	return &queryResolver{r}
}

func (r *queryResolver) SearchEvents(ctx context.Context, filter *models.AuditEventFilter, limit *int, offset *int) (*models.AuditEventConnection, error) {
	// Конвертируем GraphQL-фильтр в карту для OpenSearch
	filterMap := make(map[string]interface{})
	// ... логика конвертации

	l := 20
	if limit != nil {
		l = *limit
	}
	o := 0
	if offset != nil {
		o = *offset
	}

	events, total, err := r.OSClient.SearchEvents(ctx, filterMap, l, o)
	if err != nil {
		return nil, err
	}

	// Конвертируем модели в GraphQL-типы
	gqlEvents := make([]*models.AuditEvent, len(events))
	for i, e := range events {
		gqlEvents[i] = mapModelToGql(e)
	}

	return &models.AuditEventConnection{
		Events: gqlEvents,
		Total:  int(total),
	}, nil
}

func mapModelToGql(e *models.AuditEvent) *models.AuditEvent {
	// Details конвертируем в JSON-строку для простоты

	return &models.AuditEvent{
		EventID:   e.EventID,
		Timestamp: e.Timestamp,
		Status:    e.Status,
		EventType: e.EventType,
		Actor: models.Actor{
			ID:        e.Actor.ID,
			Type:      e.Actor.Type,
			Name:      e.Actor.Name,
			IPAddress: e.Actor.IPAddress,
		},
		Entity: models.Entity{
			ID:   e.Entity.ID,
			Type: e.Entity.Type,
			Name: e.Entity.Name,
		},
		Context: models.Context{
			SourceService: e.Context.SourceService,
			TraceID:       e.Context.TraceID,
			RequestID:     e.Context.RequestID,
		},
		Details: e.Details,
	}
}
