package graphql

//go:generate go run github.com/99designs/gqlgen generate

import (
	"context"
	"fmt"
	"log/slog"
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
	filterMap := make(map[string]interface{})
	// TODO: Реализовать логику конвертации filter *generated.AuditEventFilter в filterMap
	// Например:
	if filter != nil {
		if filter.Status != nil {
			filterMap["status"] = *filter.Status
		}
		if filter.EventType != nil {
			filterMap["event_type"] = *filter.EventType
		}
		if filter.ActorID != nil {
			filterMap["actor.id"] = *filter.ActorID
		}
		if filter.EntityID != nil {
			filterMap["entity.id"] = *filter.EntityID
		}
		if filter.SecurityAccessLevel != nil {
			filterMap["security.access_level"] = *filter.SecurityAccessLevel
		}
	}

	l := 20
	if limit != nil {
		l = *limit
	}
	o := 0
	if offset != nil {
		o = *offset
	}

	// Вызываем OpenSearch для получения событий.
	// Он возвращает []*models.AuditEvent, что идеально соответствует
	// тому, что ожидает generated.AuditEventConnection.Events.
	events, total, err := r.OSClient.SearchEvents(ctx, filterMap, l, o)
	if err != nil {
		slog.Error("failed to search events in OpenSearch", "error", err)
		return nil, fmt.Errorf("failed to search events: %w", err)
	}

	// Больше не нужно преобразовывать события вручную,
	// так как generated.AuditEventConnection уже ожидает []*models.AuditEvent
	// (см. ваш generated.go и маршалеры там).
	return &models.AuditEventConnection{
		Events: events, // Теперь передаем напрямую
		Total:  int(total),
	}, nil
}

// 🔥 НОВОЕ: Структура для реализации SecurityResolver
type securityResolver struct{ *Resolver }

// 🔥 НОВОЕ: Реализация резолвера для поля access_level в Security
// (По умолчанию gqlgen справится сам, но если бы нужна была кастомная логика,
// она была бы здесь. Интерфейс требует его наличия.)
func (r *securityResolver) AccessLevel(ctx context.Context, obj *models.Security) (string, error) {
	return obj.AccessLevel, nil
}
