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

type RestartAllResponse struct {
	Total     int             `json:"total"`
	Restarted int64           `json:"restarted"`
	Failed    int64           `json:"failed"`
	Skipped   int64           `json:"skipped"`
	Results   []RestartResult `json:"results"`
}

// RestartAllBots restarts all running bots with full pod spec rebuild.
// This picks up all config changes: images, sidecar, resources, env vars, etc.
// POST /bot/api/v1/admin/bots/restart
func RestartAllBots(c echo.Context) error {
	bots, err := model.ListBotsByStatus(model.BotStatusRunning)
	if err != nil {
		return util.InternalError(c, "failed to list running bots")
	}

	if len(bots) == 0 {
		return util.Success(c, &RestartAllResponse{Total: 0})
	}

	ctx := context.Background()
	var restarted, failed, skipped atomic.Int64
	results := make([]RestartResult, len(bots))

	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // max 5 concurrent restarts

	for i, bot := range bots {
		wg.Add(1)
		go func(idx int, b *model.Bot) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := RestartResult{BotID: b.ID}

			// Check deployment exists
			exists, err := k8s.DeploymentExists(ctx, b.ID)
			if err != nil || !exists {
				skipped.Add(1)
				result.Status = "skipped"
				result.Message = "deployment not found"
				results[idx] = result
				return
			}

			// Build config and replace deployment
			openclawConfig, _ := b.GetOpenClawConfig()
			k8sConfig := convertToK8sConfig(b, openclawConfig)

			if err := k8s.ReplaceDeployment(ctx, b.ID, b.UserID, b.AccessToken, k8sConfig); err != nil {
				failed.Add(1)
				result.Status = "failed"
				result.Message = err.Error()
				results[idx] = result
				return
			}

			// Recreate service for port changes
			k8s.DeleteService(ctx, b.ID)
			endpoint, err := k8s.CreateService(ctx, b.ID, b.UserID)
			if err != nil {
				failed.Add(1)
				result.Status = "failed"
				result.Message = fmt.Sprintf("service recreate failed: %v", err)
				results[idx] = result
				return
			}

			_ = model.UpdateBotStatus(b.ID, model.BotStatusRunning, endpoint)

			restarted.Add(1)
			result.Status = "restarted"
			results[idx] = result
		}(i, bot)
	}

	wg.Wait()

	return util.Success(c, &RestartAllResponse{
		Total:     len(bots),
		Restarted: restarted.Load(),
		Failed:    failed.Load(),
		Skipped:   skipped.Load(),
		Results:   results,
	})
}
