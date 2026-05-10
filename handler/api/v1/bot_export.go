package v1

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/clawhost/clawhost/middleware"
	"github.com/clawhost/clawhost/service/k8s"
	"github.com/clawhost/clawhost/util"
	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
)

const defaultExportMaxSize = int64(1) << 30 // 1 GiB

// ExportBotData streams a tar.gz of the bot's data root (~/.openclaw) to the client.
// GET /bot/api/v1/bots/:id/export
//
// The archive is built into a temp file first so we can refuse the download
// up-front if it exceeds export.max_size_bytes (default 1 GiB).
func ExportBotData(c echo.Context) error {
	bot := middleware.GetBotFromContext(c)
	if bot == nil {
		return util.Forbidden(c, "not authorized")
	}

	ctx := c.Request().Context()
	ap, err := k8s.OpenAccess(ctx, bot.ID)
	if err != nil {
		return util.InternalError(c, "failed to open bot data access: "+err.Error())
	}
	defer ap.Close()

	tmp, err := os.CreateTemp("", "bot-export-*.tar.gz")
	if err != nil {
		return util.InternalError(c, "failed to create temp file: "+err.Error())
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	defer tmp.Close()

	if err := k8s.ArchiveFromPod(ctx, ap, tmp, k8s.BotDataExportExcludes); err != nil {
		return util.InternalError(c, "failed to archive bot data: "+err.Error())
	}
	if err := tmp.Sync(); err != nil {
		return util.InternalError(c, "failed to flush archive: "+err.Error())
	}

	stat, err := tmp.Stat()
	if err != nil {
		return util.InternalError(c, "failed to stat archive: "+err.Error())
	}
	size := stat.Size()

	limit := viper.GetInt64("export.max_size_bytes")
	if limit <= 0 {
		limit = defaultExportMaxSize
	}
	if size > limit {
		return util.BadRequest(c, fmt.Sprintf(
			"export archive too large (%s, limit %s); please contact the administrator",
			humanSize(size), humanSize(limit)))
	}

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return util.InternalError(c, "failed to rewind archive: "+err.Error())
	}

	filename := fmt.Sprintf("bot-%s-%s.tar.gz", bot.ID, time.Now().UTC().Format("20060102-150405"))
	res := c.Response()
	res.Header().Set("Content-Type", "application/gzip")
	res.Header().Set(echo.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="%s"`, filename))
	res.Header().Set(echo.HeaderContentLength, strconv.FormatInt(size, 10))
	res.Header().Set("X-Content-Type-Options", "nosniff")
	res.WriteHeader(200)

	if _, err := io.Copy(res.Writer, tmp); err != nil {
		c.Logger().Errorf("export bot %s: copy to client failed: %v", bot.ID, err)
	}
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
