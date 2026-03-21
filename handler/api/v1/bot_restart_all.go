package v1

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/clawhost/clawhost/model"
	"github.com/clawhost/clawhost/service/k8s"
	"github.com/clawhost/clawhost/util"
	"github.com/labstack/echo/v4"
)

type RestartResult struct {
	BotID   string `json:"bot_id"`
	Status  string `json:"status"` // "restarted", "failed", "skipped"
	Message string `json:"message,omitempty"`
}

// RestartAllBots restarts all running bots with full pod spec rebuild.
// Executes asynchronously — returns immediately with the total count,
// bots are restarted in the background.
// POST /bot/api/v1/admin/bots/restart
func RestartAllBots(c echo.Context) error {
	bots, err := model.ListBotsByStatus(model.BotStatusRunning)
	if err != nil {
		return util.InternalError(c, "failed to list running bots")
	}

	if len(bots) == 0 {
		return util.Success(c, map[string]interface{}{
			"total":   0,
			"message": "no running bots to restart",
		})
	}

	// Launch restart in background
	go restartBotsAsync(bots)

	return util.Success(c, map[string]interface{}{
		"total":   len(bots),
		"message": "restart initiated in background",
	})
}

func restartBotsAsync(bots []*model.Bot) {
	ctx := context.Background()
	var restarted, failed, skipped atomic.Int64

	var wg sync.WaitGroup
	sem := make(chan struct{}, 1) // sequential restarts to avoid node memory pressure

	for _, bot := range bots {
		wg.Add(1)
		go func(b *model.Bot) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Check deployment exists
			exists, err := k8s.DeploymentExists(ctx, b.ID)
			if err != nil || !exists {
				skipped.Add(1)
				fmt.Printf("[RestartAll] Skipped bot %s: deployment not found\n", b.ID)
				return
			}

			// Build config and replace deployment
			openclawConfig, _ := b.GetOpenClawConfig()
			k8sConfig := convertToK8sConfig(b, openclawConfig)

			if err := k8s.ReplaceDeployment(ctx, b.ID, b.UserID, b.AccessToken, k8sConfig); err != nil {
				failed.Add(1)
				fmt.Printf("[RestartAll] Failed bot %s: %v\n", b.ID, err)
				return
			}

			// Recreate service for port changes
			k8s.DeleteService(ctx, b.ID)
			endpoint, err := k8s.CreateService(ctx, b.ID, b.UserID)
			if err != nil {
				failed.Add(1)
				fmt.Printf("[RestartAll] Service failed for bot %s: %v\n", b.ID, err)
				return
			}

			_ = model.UpdateBotStatus(b.ID, model.BotStatusRunning, endpoint)

			// Sync config to pod after restart (ensures controlUi, http, etc.)
			if k8sConfig.AccessToken != "" {
				if err := k8s.WriteConfigToBot(ctx, b.ID, k8sConfig, false); err != nil {
					fmt.Printf("[RestartAll] Config sync failed for bot %s: %v\n", b.ID, err)
				}
			}

			restarted.Add(1)
			fmt.Printf("[RestartAll] Restarted bot %s\n", b.ID)
		}(bot)
	}

	wg.Wait()
	fmt.Printf("[RestartAll] Done: %d restarted, %d failed, %d skipped (total %d)\n",
		restarted.Load(), failed.Load(), skipped.Load(), len(bots))
}
