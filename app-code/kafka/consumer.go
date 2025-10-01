package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"
	"witness/models"
	"witness/opensearch"

	"github.com/IBM/sarama"
)

// Consumer представляет собой consumer group для Kafka.
type Consumer struct {
	ready         chan bool
	osClient      *opensearch.Client
	eventBuffer   []*models.AuditEvent
	bufferMutex   sync.Mutex
	maxBufferSize int
	flushInterval time.Duration
}

// NewConsumer создает новый экземпляр consumer'a.
func NewConsumer(osClient *opensearch.Client) *Consumer {
	return &Consumer{
		ready:         make(chan bool),
		osClient:      osClient,
		eventBuffer:   make([]*models.AuditEvent, 0, 100),
		maxBufferSize: 100,
		flushInterval: 5 * time.Second,
	}
}

// StartConsumerGroup запускает consumer group.
func (c *Consumer) StartConsumerGroup(ctx context.Context, wg *sync.WaitGroup, brokers []string, groupID, topic string) {
	defer wg.Done()

	config := sarama.NewConfig()
	config.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()
	config.Consumer.Offsets.Initial = sarama.OffsetOldest

	client, err := sarama.NewConsumerGroup(brokers, groupID, config)
	if err != nil {
		slog.Error("error creating consumer group client", "error", err)
		return
	}

	// Запускаем периодическую отправку буфера
	go c.runFlusher(ctx)

	slog.Info("kafka consumer group started")

	for {
		// `Consume` должен вызываться в бесконечном цикле, т.к. он завершается
		// при ребалансировке сессии.
		if err := client.Consume(ctx, []string{topic}, c); err != nil {
			if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				slog.Info("consumer group closed")
				return
			}
			slog.Error("error from consumer", "error", err)
		}
		// Проверка, не был ли контекст отменен
		if ctx.Err() != nil {
			slog.Info("context cancelled, stopping consumer group")
			return
		}
		c.ready = make(chan bool)
	}
}

func (c *Consumer) runFlusher(ctx context.Context) {
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("flusher context done, performing final flush")
			c.flushBuffer(context.Background()) // Используем новый контекст для последней отправки
			return
		case <-ticker.C:
			c.flushBuffer(ctx)
		}
	}
}

// flushBuffer отправляет накопленные события в OpenSearch.
func (c *Consumer) flushBuffer(ctx context.Context) {
	c.bufferMutex.Lock()
	if len(c.eventBuffer) == 0 {
		c.bufferMutex.Unlock()
		return
	}

	// Копируем срез, чтобы можно было разблокировать мьютекс
	eventsToFlush := make([]*models.AuditEvent, len(c.eventBuffer))
	copy(eventsToFlush, c.eventBuffer)
	c.eventBuffer = c.eventBuffer[:0] // Очищаем буфер
	c.bufferMutex.Unlock()

	err := c.osClient.IndexEventsBulk(ctx, eventsToFlush)
	if err != nil {
		slog.Error("failed to flush events to opensearch", "error", err)
		// TODO: Реализовать механизм retry или отправки в dead-letter-queue
	} else {
		slog.Info("flushed events to opensearch", "count", len(eventsToFlush))
	}
}

// Setup вызывается при начале новой сессии, перед ConsumeClaim.
func (c *Consumer) Setup(sarama.ConsumerGroupSession) error {
	close(c.ready)
	return nil
}

// Cleanup вызывается в конце сессии, после завершения всех циклов ConsumeClaim.
func (c *Consumer) Cleanup(sarama.ConsumerGroupSession) error {
	// Отправляем остатки из буфера перед завершением
	c.flushBuffer(context.Background())
	return nil
}

// ConsumeClaim - основной цикл обработки сообщений.
func (c *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message, ok := <-claim.Messages():
			if !ok {
				slog.Info("message channel was closed")
				return nil
			}

			var event models.AuditEvent
			if err := json.Unmarshal(message.Value, &event); err != nil {
				slog.Error("failed to unmarshal kafka message", "error", err)
				// Пропускаем сбойное сообщение, но коммитим его, чтобы не читать снова.
				session.MarkMessage(message, "")
				continue
			}

			c.bufferMutex.Lock()
			c.eventBuffer = append(c.eventBuffer, &event)
			bufferSize := len(c.eventBuffer)
			c.bufferMutex.Unlock()

			if bufferSize >= c.maxBufferSize {
				c.flushBuffer(session.Context())
			}

			session.MarkMessage(message, "")

		case <-session.Context().Done():
			return nil
		}
	}
}
