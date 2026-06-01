package cmd

import (
	"context"
	"log"
	"sync"
	"sync/atomic"

	"github.com/clawhost/clawhost/model"
	"github.com/clawhost/clawhost/service/k8s"
	"github.com/spf13/cobra"
)

var (
	stopAllLimit       int
	stopAllConcurrency int
	stopAllUserID      string
	stopAllDryRun      bool
)

var stopAllCmd = &cobra.Command{
	Use:   "stop-all",
	Short: "Stop all bot pods (delete their K8s deployments/services, mark DB stopped)",
	Long: `Stop all openclaw bot deployments in bulk.

The target list is read from ACTUAL K8s state — every deployment labelled
app=openclaw — not from the DB. This makes it immune to DB drift: it won't miss
a live pod whose DB row wrongly says "stopped", and it won't choke on a DB row
that says "running" but whose deployment is already gone.

For each bot it deletes the K8s Deployment and Service and marks the bot
'stopped' in the DB (in-band, so no separate reconcile run is needed). Data on
the shared PVC is untouched, so bots can be restarted later and the data-export
API keeps working without any pod running.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := initConfigLight(); err != nil {
			log.Fatalf("init config failed: %v", err)
		}
		if err := k8s.InitClient(); err != nil {
			log.Fatalf("init k8s client failed: %v", err)
		}

		if stopAllConcurrency <= 0 {
			stopAllConcurrency = 10
		}

		log.Printf("[stop-all] starting, limit=%d, concurrency=%d, user=%q, dry-run=%v",
			stopAllLimit, stopAllConcurrency, stopAllUserID, stopAllDryRun)
		stopAllBots()
		log.Printf("[stop-all] done")
	},
}

func init() {
	stopAllCmd.Flags().IntVar(&stopAllLimit, "limit", 0, "max number of bots to stop (0 = no limit)")
	stopAllCmd.Flags().IntVar(&stopAllConcurrency, "concurrency", 10, "number of bots to stop in parallel")
	stopAllCmd.Flags().StringVar(&stopAllUserID, "user-id", "", "only stop bots owned by this user (default: all users)")
	stopAllCmd.Flags().BoolVar(&stopAllDryRun, "dry-run", false, "list the bots that would be stopped without deleting anything")
	rootCmd.AddCommand(stopAllCmd)
}

func stopAllBots() {
	ctx := context.Background()

	// Drive deletion from actual K8s state (deployments labelled app=openclaw),
	// NOT from the DB. This is immune to DB drift: it won't miss a live pod just
	// because the DB row says "stopped", and it self-heals DB rows that wrongly
	// say "running" but whose deployment is already gone (handled by the
	// reconcile loop, which we also keep in sync here per bot we touch).
	botIDs, err := k8s.ListBotDeploymentIDs(ctx)
	if err != nil {
		log.Printf("[stop-all] failed to list bot deployments: %v", err)
		return
	}

	// Optional per-user filter (needs the DB to resolve ownership).
	if stopAllUserID != "" {
		filtered := botIDs[:0]
		for _, id := range botIDs {
			if bot, err := model.GetBotByID(id); err == nil && bot.UserID == stopAllUserID {
				filtered = append(filtered, id)
			}
		}
		botIDs = filtered
	}

	// Optional limit.
	if stopAllLimit > 0 && len(botIDs) > stopAllLimit {
		botIDs = botIDs[:stopAllLimit]
	}

	if len(botIDs) == 0 {
		log.Printf("[stop-all] no bot deployments found to stop")
		return
	}

	log.Printf("[stop-all] %d bot deployment(s) to stop", len(botIDs))

	if stopAllDryRun {
		for _, id := range botIDs {
			name := "?"
			if bot, err := model.GetBotByID(id); err == nil {
				name = bot.Name
			}
			log.Printf("[stop-all] (dry-run) would stop bot %s (%s)", id, name)
		}
		log.Printf("[stop-all] dry-run: nothing was deleted")
		return
	}

	sem := make(chan struct{}, stopAllConcurrency)
	var wg sync.WaitGroup
	var stopped, failed atomic.Int64

	for _, botID := range botIDs {
		wg.Add(1)
		sem <- struct{}{} // acquire
		go func(botID string) {
			defer wg.Done()
			defer func() { <-sem }() // release

			ctx := context.Background()
			// Delete deployment first so the ReplicaSet can't recreate the pod.
			// DeleteDeployment treats NotFound as success, so this is idempotent.
			if err := k8s.DeleteDeployment(ctx, botID); err != nil {
				log.Printf("[stop-all] bot %s: delete deployment failed: %v", botID, err)
				failed.Add(1)
				return
			}
			k8s.DeleteService(ctx, botID)

			// Keep the DB in sync. The bot row may not exist (orphaned pod with no
			// DB record) — that's fine, the pod is gone either way.
			if err := model.UpdateBotStatus(botID, model.BotStatusStopped, ""); err != nil {
				log.Printf("[stop-all] bot %s: deployment deleted but DB update failed: %v", botID, err)
			}

			stopped.Add(1)
			log.Printf("[stop-all] stopped bot %s", botID)
		}(botID)
	}

	wg.Wait()

	log.Printf("[stop-all] %d stopped, %d failed (total %d)",
		stopped.Load(), failed.Load(), len(botIDs))
}
