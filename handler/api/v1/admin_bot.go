package v1

import (
	"context"

	"github.com/clawhost/clawhost/model"
	"github.com/clawhost/clawhost/service/k8s"
	"github.com/clawhost/clawhost/util"
	"github.com/labstack/echo/v4"
)

// AdminCreateBot creates a new bot (admin only)
func AdminCreateBot(c echo.Context) error {
	var req struct {
		AppID  string `json:"app_id"`
		UserID string `json:"user_id"`
		Name   string `json:"name"`
		Slug   string `json:"slug"`
	}
	if err := c.Bind(&req); err != nil {
		return util.BadRequest(c, "invalid request body")
	}
	if req.Name == "" {
		return util.BadRequest(c, "name is required")
	}
	if req.AppID == "" {
		return util.BadRequest(c, "app_id is required")
	}
	if req.UserID == "" {
		req.UserID = "admin"
	}

	bot := &model.Bot{
		AppID:  req.AppID,
		UserID: req.UserID,
		Name:   req.Name,
		Slug:   req.Slug,
		Status: model.BotStatusCreated,
	}

	if err := model.CreateBot(bot); err != nil {
		return util.InternalError(c, "failed to create bot")
	}

	return util.Success(c, bot)
}

// AdminListBots lists all bots across all apps (admin only)
// Access tokens are redacted in list responses for security
func AdminListBots(c echo.Context) error {
	bots, err := model.ListAllBots()
	if err != nil {
		return util.InternalError(c, "failed to list bots")
	}
	// Redact access tokens from list response
	for _, bot := range bots {
		bot.AccessToken = ""
	}
	return util.Success(c, bots)
}

// AdminStartBot starts a bot by ID (admin only)
func AdminStartBot(c echo.Context) error {
	botID := c.Param("id")
	bot, err := model.GetBotByID(botID)
	if err != nil {
		return util.NotFound(c, "bot not found")
	}

	if bot.Status == model.BotStatusRunning {
		return util.BadRequest(c, "bot is already running")
	}

	ctx := context.Background()
	openclawConfig, _ := bot.GetOpenClawConfig()
	k8sConfig := convertToK8sConfig(bot, openclawConfig)

	if err := k8s.CreateDeployment(ctx, bot.ID, bot.UserID, bot.AccessToken, k8sConfig); err != nil {
		return util.InternalError(c, "failed to create deployment: "+err.Error())
	}

	endpoint, err := k8s.CreateService(ctx, bot.ID, bot.UserID)
	if err != nil {
		k8s.DeleteDeployment(ctx, bot.ID)
		return util.InternalError(c, "failed to create service: "+err.Error())
	}

	if err := model.UpdateBotStatus(bot.ID, model.BotStatusStarting, endpoint); err != nil {
		return util.InternalError(c, "failed to update bot status")
	}

	// Wait for pod ready in background, then update status
	go func() {
		bgCtx := context.Background()
		_, err := k8s.WaitForPodReady(bgCtx, bot.ID, 300)
		if err != nil {
			model.UpdateBotStatus(bot.ID, model.BotStatusError, endpoint)
			return
		}
		model.UpdateBotStatus(bot.ID, model.BotStatusRunning, endpoint)
		if k8sConfig.AccessToken != "" {
			k8s.WriteConfigToBot(bgCtx, bot.ID, k8sConfig, false)
		}
	}()

	bot.Status = model.BotStatusStarting
	bot.Endpoint = endpoint
	return util.Success(c, bot)
}

// AdminStopBot stops a bot by ID (admin only)
func AdminStopBot(c echo.Context) error {
	botID := c.Param("id")
	bot, err := model.GetBotByID(botID)
	if err != nil {
		return util.NotFound(c, "bot not found")
	}

	if bot.Status != model.BotStatusRunning {
		return util.BadRequest(c, "bot is not running")
	}

	ctx := context.Background()
	if err := k8s.DeleteDeployment(ctx, bot.ID); err != nil {
		return util.InternalError(c, "failed to delete deployment: "+err.Error())
	}
	if err := k8s.DeleteService(ctx, bot.ID); err != nil {
		return util.InternalError(c, "failed to delete service: "+err.Error())
	}

	if err := model.UpdateBotStatus(bot.ID, model.BotStatusStopped, ""); err != nil {
		return util.InternalError(c, "failed to update bot status")
	}

	bot.Status = model.BotStatusStopped
	bot.Endpoint = ""
	return util.Success(c, bot)
}

// AdminDeleteBot deletes a bot by ID (admin only)
func AdminDeleteBot(c echo.Context) error {
	botID := c.Param("id")
	bot, err := model.GetBotByID(botID)
	if err != nil {
		return util.NotFound(c, "bot not found")
	}

	ctx := context.Background()
	if bot.Status == model.BotStatusRunning {
		k8s.DeleteDeployment(ctx, bot.ID)
		k8s.DeleteService(ctx, bot.ID)
	}

	if err := model.DeleteBot(bot.ID); err != nil {
		return util.InternalError(c, "failed to delete bot")
	}

	return util.Success(c, map[string]string{"message": "bot deleted"})
}

// AdminGetBot returns a single bot by ID (admin only)
func AdminGetBot(c echo.Context) error {
	botID := c.Param("id")
	bot, err := model.GetBotByID(botID)
	if err != nil {
		return util.NotFound(c, "bot not found")
	}
	return util.Success(c, bot)
}

// AdminRestartBot restarts a bot by ID (admin only)
func AdminRestartBot(c echo.Context) error {
	botID := c.Param("id")
	bot, err := model.GetBotByID(botID)
	if err != nil {
		return util.NotFound(c, "bot not found")
	}

	if bot.Status != model.BotStatusRunning {
		return util.BadRequest(c, "bot is not running")
	}

	ctx := context.Background()

	// Build current config
	openclawConfig, _ := bot.GetOpenClawConfig()
	k8sConfig := convertToK8sConfig(bot, openclawConfig)

	// Replace deployment spec with rolling update
	if err := k8s.ReplaceDeployment(ctx, bot.ID, bot.UserID, bot.AccessToken, k8sConfig); err != nil {
		return util.InternalError(c, "failed to update deployment: "+err.Error())
	}

	// Recreate service to pick up port changes
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
