package v1

import (
	"context"

	"github.com/clawhost/clawhost/middleware"
	"github.com/clawhost/clawhost/model"
	"github.com/clawhost/clawhost/service/k8s"
	"github.com/clawhost/clawhost/util"
	"github.com/labstack/echo/v4"
)

func RestartBot(c echo.Context) error {
	bot := middleware.GetBotFromContext(c)
	if bot == nil {
		return util.Forbidden(c, "not authorized")
	}

	if bot.Status != model.BotStatusRunning {
		return util.BadRequest(c, "bot is not running")
	}

	ctx := context.Background()

	// Build current config
	openclawConfig, _ := bot.GetOpenClawConfig()
	k8sConfig := convertToK8sConfig(bot, openclawConfig)

	// Replace deployment spec with rolling update — picks up all changes
	// (ChatClaw sidecar, image updates, resource changes) without downtime
	if err := k8s.ReplaceDeployment(ctx, bot.ID, bot.UserID, bot.AccessToken, k8sConfig); err != nil {
		return util.InternalError(c, "failed to update deployment: "+err.Error())
	}

	// Recreate service to pick up port changes (e.g., ChatClaw port added/removed)
	// DeleteService + CreateService is safe — existing connections drain naturally
	// during the rolling update window
	k8s.DeleteService(ctx, bot.ID)
	endpoint, err := k8s.CreateService(ctx, bot.ID, bot.UserID)
	if err != nil {
		return util.InternalError(c, "failed to create service: "+err.Error())
	}

	// Update endpoint
	if err := model.UpdateBotStatus(bot.ID, model.BotStatusRunning, endpoint); err != nil {
		c.Logger().Errorf("failed to update bot endpoint: %v", err)
	}

	// Sync config to pod after restart
	go func() {
		if k8sConfig.AccessToken != "" {
			if err := k8s.WriteConfigToBot(context.Background(), bot.ID, k8sConfig, false); err != nil {
				c.Logger().Errorf("failed to write config to bot: %v", err)
			}
		}
	}()

	return util.Success(c, bot)
}
