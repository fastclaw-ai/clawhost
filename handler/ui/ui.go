package ui

import (
	"embed"
	"net/http"

	"github.com/labstack/echo/v4"
)

//go:embed assets/index.html
var assets embed.FS

func Index(c echo.Context) error {
	content, err := assets.ReadFile("assets/index.html")
	if err != nil {
		return c.String(http.StatusInternalServerError, "failed to load UI")
	}
	return c.HTMLBlob(http.StatusOK, content)
}
