# --- Builder Stage ---
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Копируем и скачиваем зависимости
COPY app-code/go.mod app-code/go.sum ./
RUN go mod download

# Копируем исходный код
COPY app-code/ .

# Собираем приложение
# CGO_ENABLED=0 - статическая сборка без C-зависимостей
# -ldflags="-s -w" - удаляет отладочную информацию для уменьшения размера
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/witness ./main.go

# --- Final Stage ---
FROM alpine:latest

# Устанавливаем корневые сертификаты для HTTPS/TLS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Копируем скомпилированный бинарник из builder stage
COPY --from=builder /app/witness .

# Порт, который будет слушать приложение
EXPOSE 8080

# Команда для запуска приложения
CMD ["./witness"]