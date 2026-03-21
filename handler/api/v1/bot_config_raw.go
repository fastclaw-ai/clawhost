package v1

import (
	"context"
	"encoding/json"

	"github.com/clawhost/clawhost/middleware"
	"github.com/clawhost/clawhost/model"
	"github.com/clawhost/clawhost/service/k8s"
	"github.com/clawhost/clawhost/util"
	"github.com/labstack/echo/v4"
)

// GetBotRawConfig reads the openclaw.json config from the bot's running pod
// GET /bot/api/v1/bots/:id/config/raw
func GetBotRawConfig(c echo.Context) error {
	bot := middleware.GetBotFromContext(c)
	if bot == nil {
		return util.Forbidden(c, "not authorized")
	}

	if bot.Status != model.BotStatusRunning {
		return util.BadRequest(c, "bot is not running")
	}

	ctx := context.Background()
	config, err := k8s.ReadBotConfig(ctx, bot.ID)
	if err != nil {
		return util.InternalError(c, "failed to read config: "+err.Error())
	}

	return util.Success(c, config)
}

// UpdateBotRawConfig writes the openclaw.json config to the bot's running pod
// PUT /bot/api/v1/bots/:id/config/raw
//
// Accepts either a full config (replaces entirely) or a partial config (merged).
// Query param: ?mode=merge (default) or ?mode=replace
func UpdateBotRawConfig(c echo.Context) error {
	bot := middleware.GetBotFromContext(c)
	if bot == nil {
		return util.Forbidden(c, "not authorized")
	}

	if bot.Status != model.BotStatusRunning {
		return util.BadRequest(c, "bot is not running")
	}

	var input map[string]interface{}
	if err := json.NewDecoder(c.Request().Body).Decode(&input); err != nil {
		return util.BadRequest(c, "invalid JSON body")
	}

	ctx := context.Background()
	mode := c.QueryParam("mode")

	var finalConfig map[string]interface{}

	if mode == "replace" {
		// Full replace
		finalConfig = input
	} else {
		// Merge mode (default): read existing, then merge input on top
		existing, err := k8s.ReadBotConfig(ctx, bot.ID)
		if err != nil {
			return util.InternalError(c, "failed to read existing config: "+err.Error())
		}
		finalConfig = mergeMap(existing, input)
	}

	if err := k8s.WriteBotConfig(ctx, bot.ID, finalConfig); err != nil {
		return util.InternalError(c, "failed to write config: "+err.Error())
	}

	return util.Success(c, finalConfig)
}

// mergeMap does a shallow merge of src into dst (src overwrites dst for top-level keys)
func mergeMap(dst, src map[string]interface{}) map[string]interface{} {
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
