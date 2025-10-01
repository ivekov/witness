package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type HealthStatus struct {
	Status string `json:"Status"`
}

// HealthCheck возвращает статус "Ok".
func HealthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, HealthStatus{Status: "Ok"})
}
