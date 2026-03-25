package cmd

import (
	"context"
	"log"
	"time"

	"github.com/clawhost/clawhost/model"
	"github.com/clawhost/clawhost/service/k8s"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Stop expired bots and clean up K8s resources",
	Run: func(cmd *cobra.Command, args []string) {
		if err := initConfigLight(); err != nil {
			log.Fatalf("init config failed: %v", err)
		}
		if err := k8s.InitClient(); err != nil {
			log.Fatalf("init k8s client failed: %v", err)
		}

		graceHours := viper.GetInt("bot.cleanup_grace_hours")
		if graceHours <= 0 {
			graceHours = 72
		}
		batchSize := viper.GetInt("bot.cleanup_batch_size")
		if batchSize <= 0 {
			batchSize = 50
		}
		grace := time.Duration(graceHours) * time.Hour

		log.Printf("[cleanup] starting, grace=%dh, batch=%d", graceHours, batchSize)
		cleanupExpiredBots(grace, batchSize)
		log.Printf("[cleanup] done")
	},
}

func init() {
	rootCmd.AddCommand(cleanupCmd)
}

func cleanupExpiredBots(grace time.Duration, limit int) {
	bots, err := model.ListExpiredBots(grace, limit)
	if err != nil {
		log.Printf("[cleanup] failed to list expired bots: %v", err)
		return
	}
	if len(bots) == 0 {
		log.Printf("[cleanup] no expired bots found")
		return
	}

	log.Printf("[cleanup] found %d expired bot(s)", len(bots))
	ctx := context.Background()

	for _, bot := range bots {
		log.Printf("[cleanup] removing bot %s (%s), status=%s, expired at %s",
			bot.ID, bot.Name, bot.Status, bot.ExpiresAt.Format(time.RFC3339))

		// Clean up K8s resources (idempotent, safe if not exists)
		k8s.DeleteDeployment(ctx, bot.ID)
		k8s.DeleteService(ctx, bot.ID)

		// Mark as deleted
		if err := model.UpdateBotStatus(bot.ID, model.BotStatusDeleted, ""); err != nil {
			log.Printf("[cleanup] failed to mark bot %s as deleted: %v", bot.ID, err)
			continue
		}

		log.Printf("[cleanup] bot %s marked as deleted", bot.ID)
	}
}
