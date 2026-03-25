package cmd

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/clawhost/clawhost/model"
	"github.com/clawhost/clawhost/service/k8s"
	"github.com/spf13/cobra"
)

var reconcileCmd = &cobra.Command{
	Use:   "reconcile",
	Short: "Sync bot status in DB with actual K8s state",
	Run: func(cmd *cobra.Command, args []string) {
		if err := initConfigLight(); err != nil {
			log.Fatalf("init config failed: %v", err)
		}
		if err := k8s.InitClient(); err != nil {
			log.Fatalf("init k8s client failed: %v", err)
		}

		log.Printf("[reconcile] starting")
		reconcileBotStatus()
		log.Printf("[reconcile] done")
	},
}

func init() {
	rootCmd.AddCommand(reconcileCmd)
}

const startingTimeout = 10 * time.Minute

func reconcileBotStatus() {
	// Phase 1: Check bots that DB thinks are active
	running, _ := model.ListBotsByStatus(model.BotStatusRunning)
	starting, _ := model.ListBotsByStatus(model.BotStatusStarting)
	activeBots := append(running, starting...)

	// Phase 2: Check bots that DB thinks are inactive but K8s might still have resources
	stopped, _ := model.ListBotsByStatus(model.BotStatusStopped)
	errored, _ := model.ListBotsByStatus(model.BotStatusError)
	inactiveBots := append(stopped, errored...)

	total := len(activeBots) + len(inactiveBots)
	if total == 0 {
		log.Printf("[reconcile] no bots to check")
		return
	}

	ctx := context.Background()
	concurrency := 10
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var fixed atomic.Int64

	// Phase 1: active bots — check if K8s matches DB
	if len(activeBots) > 0 {
		log.Printf("[reconcile] checking %d active bot(s)", len(activeBots))
		for _, bot := range activeBots {
			wg.Add(1)
			sem <- struct{}{}
			go func(bot *model.Bot) {
				defer wg.Done()
				defer func() { <-sem }()

				info, err := k8s.GetDeploymentStatusInfo(ctx, bot.ID)
				if err != nil {
					log.Printf("[reconcile] failed to check bot %s: %v", bot.ID, err)
					return
				}

				switch {
				case info.Status == "not_found":
					log.Printf("[reconcile] bot %s (%s): deployment not found, %s -> stopped",
						bot.ID, bot.Name, bot.Status)
					model.UpdateBotStatus(bot.ID, model.BotStatusStopped, "")
					k8s.DeleteService(ctx, bot.ID)
					fixed.Add(1)

				case info.ReadyReplicas > 0 && bot.Status == model.BotStatusStarting:
					log.Printf("[reconcile] bot %s (%s): pod ready, starting -> running",
						bot.ID, bot.Name)
					model.UpdateBotStatus(bot.ID, model.BotStatusRunning, bot.Endpoint)
					fixed.Add(1)

				case info.ReadyReplicas == 0 && bot.Status == model.BotStatusStarting &&
					time.Since(bot.UpdatedAt) > startingTimeout:
					log.Printf("[reconcile] bot %s (%s): stuck starting for %s, cleaning up",
						bot.ID, bot.Name, time.Since(bot.UpdatedAt).Round(time.Second))
					k8s.DeleteDeployment(ctx, bot.ID)
					k8s.DeleteService(ctx, bot.ID)
					model.UpdateBotStatus(bot.ID, model.BotStatusError, "")
					fixed.Add(1)

				case info.ReadyReplicas == 0 && bot.Status == model.BotStatusRunning:
					log.Printf("[reconcile] bot %s (%s): no ready pods, running -> stopped",
						bot.ID, bot.Name)
					k8s.DeleteDeployment(ctx, bot.ID)
					k8s.DeleteService(ctx, bot.ID)
					model.UpdateBotStatus(bot.ID, model.BotStatusStopped, "")
					fixed.Add(1)
				}
			}(bot)
		}
		wg.Wait()
	}

	// Phase 2: inactive bots — clean up orphaned K8s resources
	if len(inactiveBots) > 0 {
		log.Printf("[reconcile] checking %d inactive bot(s) for orphaned resources", len(inactiveBots))
		for _, bot := range inactiveBots {
			wg.Add(1)
			sem <- struct{}{}
			go func(bot *model.Bot) {
				defer wg.Done()
				defer func() { <-sem }()

				exists, err := k8s.GetDeploymentStatus(ctx, bot.ID)
				if err != nil {
					return // can't check, skip
				}
				// exists returns true if readyReplicas > 0, but we also need
				// to catch deployments with 0 ready replicas (CrashLoopBackOff etc.)
				// So check if deployment exists at all via GetDeploymentStatusInfo
				info, err := k8s.GetDeploymentStatusInfo(ctx, bot.ID)
				if err != nil || info.Status == "not_found" {
					return // no K8s resources, nothing to clean
				}
				_ = exists

				log.Printf("[reconcile] bot %s (%s): DB=%s but K8s deployment exists (ready=%d), cleaning up",
					bot.ID, bot.Name, bot.Status, info.ReadyReplicas)
				k8s.DeleteDeployment(ctx, bot.ID)
				k8s.DeleteService(ctx, bot.ID)
				fixed.Add(1)
			}(bot)
		}
		wg.Wait()
	}

	log.Printf("[reconcile] fixed %d bot(s)", fixed.Load())
}
