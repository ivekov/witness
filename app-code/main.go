package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"witness/graphql"
	"witness/graphql/generated"
	"witness/handlers"
	"witness/kafka"
	"witness/opensearch"
)

func main() {
	// Настройка структурированного логгера
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// --- Конфигурация ---
	port := getEnv("APP_PORT", "8080")
	kafkaBrokers := getEnv("KAFKA_BROKERS", "localhost:9092")
	kafkaTopic := getEnv("KAFKA_TOPIC", "audit-events")
	kafkaGroup := getEnv("KAFKA_CONSUMER_GROUP", "witness-group")
	opensearchURL := getEnv("OPENSEARCH_URL", "http://localhost:9200")

	// --- Инициализация зависимостей ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// OpenSearch Client
	osClient, err := opensearch.NewClient(opensearchURL)
	if err != nil {
		slog.Error("failed to create opensearch client", "error", err)
		os.Exit(1)
	}

	// Ждем доступности OpenSearch и создаем индекс
	err = retry(5, 2*time.Second, func() error {
		return osClient.EnsureIndexExists(ctx)
	})
	if err != nil {
		slog.Error("failed to ensure opensearch index exists", "error", err)
		os.Exit(1)
	}
	slog.Info("opensearch client initialized and index is ready")

	// Kafka Consumer
	consumer := kafka.NewConsumer(osClient)
	var wg sync.WaitGroup
	wg.Add(1)
	go consumer.StartConsumerGroup(ctx, &wg, strings.Split(kafkaBrokers, ","), kafkaGroup, kafkaTopic)
	slog.Info("kafka consumer group started")

	// --- Настройка HTTP сервера (Echo) ---
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// --- GraphQL эндпоинты ---
	gqlResolver := &graphql.Resolver{OSClient: osClient}
	gqlSrv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: gqlResolver}))

	e.GET("/healthz", handlers.HealthCheck)
	e.POST("/graphql", func(c echo.Context) error {
		gqlSrv.ServeHTTP(c.Response(), c.Request())
		return nil
	})
	e.GET("/playground", func(c echo.Context) error {
		playground.Handler("GraphQL playground", "/graphql").ServeHTTP(c.Response(), c.Request())
		return nil
	})

	// --- Запуск и Graceful Shutdown ---
	go func() {
		if err := e.Start(":" + port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("shutting down the server", "error", err)
			e.Logger.Fatal(err)
		}
	}()

	slog.Info("server started", "port", port)

	// Ожидаем сигнал для завершения
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down server...")

	// Даем 10 секунд на завершение
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Останавливаем HTTP сервер
	if err := e.Shutdown(shutdownCtx); err != nil {
		e.Logger.Fatal(err)
	}

	// Сигнализируем Kafka consumer'у о завершении
	cancel()
	wg.Wait() // Ждем, пока consumer завершит работу

	slog.Info("server gracefully stopped")
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func retry(attempts int, sleep time.Duration, fn func() error) error {
	if err := fn(); err != nil {
		if attempts--; attempts > 0 {
			slog.Warn("retrying after error", "error", err, "attempts_left", attempts)
			time.Sleep(sleep)
			return retry(attempts, sleep, fn)
		}
		return err
	}
	return nil
}
