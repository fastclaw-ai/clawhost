package middleware

import (
	"strings"

	"github.com/clawhost/clawhost/model"
	"github.com/clawhost/clawhost/util"
	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

const (
	// ContextKeyApp is the key used to store the authenticated app in context
	ContextKeyApp = "authenticated_app"
	// ContextKeyBot is the key used to store the authorized bot in context
	ContextKeyBot = "authorized_bot"
	// ContextKeyIsAdmin flags that the request was authenticated via admin token
	ContextKeyIsAdmin = "is_admin"
)

// BearerAuth returns a middleware that validates Bearer token against the apps table
func BearerAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Check if legacy config token is set (for backward compatibility)
			configuredToken := viper.GetString("api.token")

			// Get Authorization header
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				// If no auth header and no config token, skip auth (for development)
				if configuredToken == "" {
					return next(c)
				}
				return util.Unauthorized(c, "missing authorization header")
			}

			// Check Bearer prefix
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				return util.Unauthorized(c, "invalid authorization format, expected: Bearer <token>")
			}

			token := parts[1]

			// First, try to validate against apps table
			app, err := model.GetAppByAPIToken(token)
			if err == nil && app != nil {
				// Token found in apps table, store app in context
				c.Set(ContextKeyApp, app)
				return next(c)
			}

			// Fallback: validate against configured token (legacy support)
			if configuredToken != "" && token == configuredToken {
				return next(c)
			}

			return util.Unauthorized(c, "invalid token")
		}
	}
}

// GetAppFromContext retrieves the authenticated app from the request context
func GetAppFromContext(c echo.Context) *model.App {
	app, ok := c.Get(ContextKeyApp).(*model.App)
	if !ok {
		return nil
	}
	return app
}

// BotOwnerAuth returns a middleware that validates the authenticated app owns the bot
// being accessed. The bot is identified by the "id" path parameter.
// If the app owns the bot, the bot is stored in context for handlers to use.
func BotOwnerAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			botID := c.Param("id")
			if botID == "" {
				return util.BadRequest(c, "bot id is required")
			}

			bot, err := model.GetBotByID(botID)
			if err != nil {
				if err == gorm.ErrRecordNotFound {
					return util.NotFound(c, "bot not found")
				}
				return util.InternalError(c, "failed to get bot")
			}

			// Skip ownership check for admin-authenticated requests
			isAdmin, _ := c.Get(ContextKeyIsAdmin).(bool)
			if !isAdmin {
				// If an app is in context, verify ownership
				app := GetAppFromContext(c)
				if app != nil {
					if bot.AppID != app.ID {
						return util.Forbidden(c, "not authorized to access this bot")
					}
				}
			}

			// Store bot in context for handlers
			c.Set(ContextKeyBot, bot)
			return next(c)
		}
	}
}

// GetBotFromContext retrieves the authorized bot from the request context
func GetBotFromContext(c echo.Context) *model.Bot {
	bot, ok := c.Get(ContextKeyBot).(*model.Bot)
	if !ok {
		return nil
	}
	return bot
}

// AdminAuth returns a middleware that validates admin token from config
// This is used for app management endpoints
func AdminAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			adminToken := viper.GetString("api.admin_token")
			if adminToken == "" {
				return util.Unauthorized(c, "admin access is disabled")
			}

			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return util.Unauthorized(c, "missing authorization header")
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				return util.Unauthorized(c, "invalid authorization format, expected: Bearer <token>")
			}

			token := parts[1]
			if token != adminToken {
				return util.Unauthorized(c, "invalid admin token")
			}

			c.Set(ContextKeyIsAdmin, true)
			return next(c)
		}
	}
}
