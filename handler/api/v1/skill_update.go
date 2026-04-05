package v1

import (
	"context"
	"fmt"

	"github.com/clawhost/clawhost/middleware"
	"github.com/clawhost/clawhost/model"
	"github.com/clawhost/clawhost/service/k8s"
	"github.com/clawhost/clawhost/util"
	"github.com/labstack/echo/v4"
)

type UpdateSkillRequest struct {
	Content string `json:"content" validate:"required"`
}

func UpdateSkill(c echo.Context) error {
	bot := middleware.GetBotFromContext(c)
	if bot == nil {
		return util.Forbidden(c, "not authorized")
	}

	name := c.Param("name")
	if name == "" {
		return util.BadRequest(c, "skill name is required")
	}

	var req UpdateSkillRequest
	if err := c.Bind(&req); err != nil {
		return util.BadRequest(c, "invalid request body")
	}

	if req.Content == "" {
		return util.BadRequest(c, "content is required")
	}

	if bot.Status != model.BotStatusRunning {
		return util.BadRequest(c, "bot is not running, cannot update skills")
	}

	ctx := context.Background()

	// Write skill file to pod
	if err := writeSkillToPod(ctx, bot.ID, name, req.Content); err != nil {
		return util.InternalError(c, "failed to write skill: "+err.Error())
	}

	return util.Success(c, map[string]string{
		"message": "skill updated",
		"name":    name,
	})
}

func writeSkillToPod(ctx context.Context, botID, skillName, content string) error {
	namespace := k8s.GetNamespace()

	// Get pod name
	podName, err := k8s.GetPodName(ctx, botID)
	if err != nil {
		return fmt.Errorf("failed to get pod: %w", err)
	}

	skillPath := fmt.Sprintf("/app/.openclaw/workspace/skills/%s", skillName)

	// Create directory
	_, err = k8s.ExecInPod(ctx, namespace, podName, "openclaw", []string{"mkdir", "-p", skillPath})
	if err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	// Write SKILL.md file safely via stdin (no shell injection possible)
	filePath := fmt.Sprintf("%s/SKILL.md", skillPath)
	_, err = k8s.ExecInPodWithStdin(ctx, namespace, podName, "openclaw",
		[]string{"tee", filePath}, content)
	if err != nil {
		return fmt.Errorf("failed to write skill file: %w", err)
	}

	return nil
}
