package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/clawhost/clawhost/service/k8s"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const defaultExportMaxSize = int64(1) << 30 // 1 GiB

var exportCmd = &cobra.Command{
	Use:   "export <bot-id>",
	Short: "Export a bot's ~/.openclaw data as a tar.gz",
	Long: `Export a bot's data root (~/.openclaw inside the pod) as a tar.gz archive.

Reuses the running bot pod when available, otherwise spawns a short-lived
alpine pod with the same PVC mounted. Excludes node_modules and caches.
Extraction yields a single .openclaw/ folder.

The archive is built into a temp file first; if it exceeds
export.max_size_bytes (default 1 GiB) the export is refused.

Examples:
  clawhost export abc12345                       # writes ./bot-abc12345-<ts>.tar.gz
  clawhost export abc12345 -o /tmp/snap.tar.gz   # explicit path
  clawhost export abc12345 -o - > snap.tar.gz    # stream to stdout`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		botID := args[0]
		out, _ := cmd.Flags().GetString("output")
		return runExport(cmd.Context(), botID, out)
	},
}

func init() {
	exportCmd.Flags().StringP("output", "o", "", "output path (default: ./bot-<id>-<ts>.tar.gz; use - for stdout)")
	rootCmd.AddCommand(exportCmd)
}

func runExport(ctx context.Context, botID, output string) error {
	if output == "" {
		output = fmt.Sprintf("bot-%s-%s.tar.gz", botID, time.Now().UTC().Format("20060102-150405"))
	}

	ap, err := openAccess(botID)
	if err != nil {
		return err
	}
	defer ap.Close()

	tmp, err := os.CreateTemp("", "bot-export-*.tar.gz")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	cleanup := func() { tmp.Close() }
	defer cleanup()

	if err := k8s.ArchiveFromPod(ctx, ap, tmp, k8s.BotDataExportExcludes); err != nil {
		return fmt.Errorf("archive: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}

	stat, err := tmp.Stat()
	if err != nil {
		return fmt.Errorf("stat archive: %w", err)
	}
	size := stat.Size()

	limit := viper.GetInt64("export.max_size_bytes")
	if limit <= 0 {
		limit = defaultExportMaxSize
	}
	if size > limit {
		return fmt.Errorf("export archive too large (%s, limit %s); please contact the administrator",
			humanSize(size), humanSize(limit))
	}

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind: %w", err)
	}

	if output == "-" {
		if _, err := io.Copy(os.Stdout, tmp); err != nil {
			return fmt.Errorf("stream stdout: %w", err)
		}
		return nil
	}

	if dir := filepath.Dir(output); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir output: %w", err)
		}
	}
	dst, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer dst.Close()
	if _, err := io.Copy(dst, tmp); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%s)\n", output, humanSize(size))
	return nil
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
