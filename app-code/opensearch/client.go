package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
	"witness/models"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
)

const IndexName = "audit-events"

// Client - обертка над клиентом OpenSearch
type Client struct {
	os *opensearch.Client
}

// NewClient создает нового клиента OpenSearch
func NewClient(address string) (*Client, error) {
	cfg := opensearch.Config{
		Addresses:     []string{address},
		RetryOnStatus: []int{502, 503, 504, 429},
		RetryBackoff: func(i int) time.Duration {
			if i == 1 {
				return time.Second
			}
			return time.Duration(i) * time.Second
		},
		MaxRetries: 5,
	}
	client, err := opensearch.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create opensearch client: %w", err)
	}
	return &Client{os: client}, nil
}

// Ping проверяет соединение с OpenSearch
func (c *Client) Ping(ctx context.Context) error {
	req := opensearchapi.PingRequest{}
	res, err := req.Do(ctx, c.os)
	if err != nil {
		return fmt.Errorf("failed to ping OpenSearch: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("OpenSearch ping failed: %s", res.Status())
	}

	slog.Info("successfully connected to OpenSearch")
	return nil
}

// EnsureIndexExists проверяет наличие индекса и создает его, если он не существует.
func (c *Client) EnsureIndexExists(ctx context.Context) error {
	// Сначала проверяем соединение
	if err := c.Ping(ctx); err != nil {
		return fmt.Errorf("OpenSearch not available: %w", err)
	}

	req := opensearchapi.IndicesExistsRequest{
		Index: []string{IndexName},
	}
	res, err := req.Do(ctx, c.os)
	if err != nil {
		return fmt.Errorf("failed to check if index exists: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == 404 {
		slog.Info("index not found, creating it", "index", IndexName)
		return c.createIndex(ctx)
	}

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("error checking index existence: %s, body: %s", res.Status(), string(body))
	}

	slog.Info("index already exists", "index", IndexName)
	return nil
}

func (c *Client) createIndex(ctx context.Context) error {
	mapping := `{
	    "settings": {
	        "number_of_shards": 1,
	        "number_of_replicas": 0
	    },
	    "mappings": {
	        "properties": {
	            "event_id": {"type": "keyword"},
	            "timestamp": {"type": "date_nanos"},
	            "status": {"type": "keyword"},
	            "event_type": {"type": "keyword"},
	            "actor": {
	                "properties": {
	                    "id": {"type": "keyword"},
	                    "type": {"type": "keyword"},
	                    "name": {"type": "text"},
	                    "ip_address": {"type": "ip"}
	                }
	            },
	            "entity": {
	                "properties": {
	                    "id": {"type": "keyword"},
	                    "type": {"type": "keyword"},
	                    "name": {"type": "text"}
	                }
	            },
	            "context": {
	                "properties": {
	                    "source_service": {"type": "keyword"},
	                    "trace_id": {"type": "keyword"},
	                    "request_id": {"type": "keyword"}
	                }
	            },
	            "details": {"type": "flattened"}
	        }
	    }
	}`

	req := opensearchapi.IndicesCreateRequest{
		Index: IndexName,
		Body:  strings.NewReader(mapping),
	}

	res, err := req.Do(ctx, c.os)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		// Читаем тело ответа для детальной информации об ошибке
		body, _ := io.ReadAll(res.Body)
		slog.Error("OpenSearch error response",
			"status", res.Status(),
			"body", string(body))
		return fmt.Errorf("error fuck creating index: %s, body: %s", res.Status(), string(body))
	}

	slog.Info("index created successfully", "index", IndexName)
	return nil
}

// IndexEventsBulk выполняет массовую вставку событий.
func (c *Client) IndexEventsBulk(ctx context.Context, events []*models.AuditEvent) error {
	if len(events) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, event := range events {
		// Meta-данные для bulk-запроса
		meta := []byte(fmt.Sprintf(`{ "index" : { "_index" : "%s", "_id" : "%s" } }%s`, IndexName, event.EventID, "\n"))

		data, err := json.Marshal(event)
		if err != nil {
			slog.Error("failed to marshal event for bulk indexing", "event_id", event.EventID, "error", err)
			continue // Пропускаем сбойное событие
		}

		buf.Grow(len(meta) + len(data) + 1)
		buf.Write(meta)
		buf.Write(data)
		buf.WriteByte('\n')
	}

	req := opensearchapi.BulkRequest{
		Body: &buf,
	}

	res, err := req.Do(ctx, c.os)
	if err != nil {
		return fmt.Errorf("failed to perform bulk request: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("bulk indexing error: %s, body: %s", res.Status(), string(body))
	}

	// Опционально: можно проанализировать ответ на наличие ошибок для отдельных документов
	slog.Info("successfully indexed events in bulk", "count", len(events))
	return nil
}

// SearchEvents выполняет поиск событий по заданным фильтрам.
// В production этот метод нужно будет расширить для поддержки всех полей.
func (c *Client) SearchEvents(ctx context.Context, filter map[string]interface{}, limit, offset int) ([]*models.AuditEvent, int64, error) {
	// Эта функция будет сложной, так как нужно динамически строить запрос.
	// Вот упрощенный пример.

	var query map[string]interface{}
	// TODO: Динамическое построение запроса на основе фильтра.
	// Пока что возвращаем все.
	query = map[string]interface{}{
		"query": map[string]interface{}{
			"match_all": map[string]interface{}{},
		},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return nil, 0, fmt.Errorf("failed to encode search query: %w", err)
	}

	req := opensearchapi.SearchRequest{
		Index: []string{IndexName},
		Body:  &buf,
		Size:  &limit,
		From:  &offset,
	}

	res, err := req.Do(ctx, c.os)
	if err != nil {
		return nil, 0, fmt.Errorf("search request failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, 0, fmt.Errorf("search request error: %s", res.Status())
	}

	var result struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				Source *models.AuditEvent `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("failed to decode search response: %w", err)
	}

	events := make([]*models.AuditEvent, len(result.Hits.Hits))
	for i, hit := range result.Hits.Hits {
		events[i] = hit.Source
	}

	return events, result.Hits.Total.Value, nil
}
