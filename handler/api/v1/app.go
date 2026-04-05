package v1

import (
	"github.com/clawhost/clawhost/model"
	"github.com/clawhost/clawhost/util"
	"github.com/labstack/echo/v4"
)

type CreateAppRequest struct {
	Name              string `json:"name" validate:"required"`
	URL               string `json:"url,omitempty"`
	Description       string `json:"description,omitempty"`
	OwnerEmail        string `json:"owner_email,omitempty"`
	BotDomainTemplate string `json:"bot_domain_template,omitempty"`
}

type UpdateAppRequest struct {
	Name              string `json:"name,omitempty"`
	URL               string `json:"url,omitempty"`
	Description       string `json:"description,omitempty"`
	OwnerEmail        string `json:"owner_email,omitempty"`
	BotDomainTemplate string `json:"bot_domain_template,omitempty"`
	Status            string `json:"status,omitempty"` // active, disabled
}

func CreateApp(c echo.Context) error {
	var req CreateAppRequest
	if err := c.Bind(&req); err != nil {
		return util.BadRequest(c, "invalid request body")
	}

	if req.Name == "" {
		return util.BadRequest(c, "name is required")
	}

	app := &model.App{
		Name:              req.Name,
		URL:               req.URL,
		Description:       req.Description,
		OwnerEmail:        req.OwnerEmail,
		BotDomainTemplate: req.BotDomainTemplate,
	}

	if err := model.CreateApp(app); err != nil {
		return util.InternalError(c, "failed to create app")
	}

	return util.Success(c, app)
}

// ListApps returns all apps. API tokens are redacted in list responses for security.
func ListApps(c echo.Context) error {
	apps, err := model.ListApps()
	if err != nil {
		return util.InternalError(c, "failed to list apps")
	}
	// Redact API tokens from list response
	for _, app := range apps {
		app.APIToken = ""
	}
	return util.Success(c, apps)
}

func GetApp(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return util.BadRequest(c, "app id is required")
	}

	app, err := model.GetAppByID(id)
	if err != nil {
		return util.NotFound(c, "app not found")
	}

	return util.Success(c, app)
}

func UpdateApp(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return util.BadRequest(c, "app id is required")
	}

	app, err := model.GetAppByID(id)
	if err != nil {
		return util.NotFound(c, "app not found")
	}

	var req UpdateAppRequest
	if err := c.Bind(&req); err != nil {
		return util.BadRequest(c, "invalid request body")
	}

	if req.Name != "" {
		app.Name = req.Name
	}
	if req.URL != "" {
		app.URL = req.URL
	}
	if req.Description != "" {
		app.Description = req.Description
	}
	if req.OwnerEmail != "" {
		app.OwnerEmail = req.OwnerEmail
	}
	if req.BotDomainTemplate != "" {
		app.BotDomainTemplate = req.BotDomainTemplate
	}
	if req.Status != "" {
		if req.Status != "active" && req.Status != "disabled" {
			return util.BadRequest(c, "status must be 'active' or 'disabled'")
		}
		app.Status = req.Status
	}

	if err := model.UpdateApp(app); err != nil {
		return util.InternalError(c, "failed to update app")
	}

	return util.Success(c, app)
}

func DeleteApp(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return util.BadRequest(c, "app id is required")
	}

	_, err := model.GetAppByID(id)
	if err != nil {
		return util.NotFound(c, "app not found")
	}

	if err := model.DeleteApp(id); err != nil {
		return util.InternalError(c, "failed to delete app")
	}

	return util.Success(c, map[string]string{"message": "app deleted"})
}

func ResetAppToken(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return util.BadRequest(c, "app id is required")
	}

	_, err := model.GetAppByID(id)
	if err != nil {
		return util.NotFound(c, "app not found")
	}

	newToken, err := model.ResetAppAPIToken(id)
	if err != nil {
		return util.InternalError(c, "failed to reset token")
	}

	return util.Success(c, map[string]string{"api_token": newToken})
}
