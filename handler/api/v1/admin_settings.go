package v1

import (
	"context"
	"strings"

	"github.com/clawhost/clawhost/service/k8s"
	"github.com/clawhost/clawhost/util"
	"github.com/labstack/echo/v4"
)

type AdminProvisionSettingsResponse struct {
	Provision k8s.ProvisionSettings `json:"provision"`
}

type UpdateAdminProvisionSettingsRequest struct {
	Provision k8s.ProvisionSettings `json:"provision"`
}

type UpsertAdminProviderRequest struct {
	Name       string `json:"name"`
	ID         string `json:"id"`
	BaseURL    string `json:"base_url"`
	API        string `json:"api"`
	Auth       string `json:"auth"`
	APIKey     string `json:"api_key"`
	SetDefault bool   `json:"set_default"`
}

func AdminGetProvisionSettings(c echo.Context) error {
	return util.Success(c, &AdminProvisionSettingsResponse{
		Provision: k8s.GetProvisionSettings(),
	})
}

func AdminUpdateProvisionSettings(c echo.Context) error {
	var req UpdateAdminProvisionSettingsRequest
	if err := c.Bind(&req); err != nil {
		return util.BadRequest(c, "invalid request body")
	}

	req.Provision.Model.Provider = strings.TrimSpace(req.Provision.Model.Provider)
	req.Provision.Model.ID = strings.TrimSpace(req.Provision.Model.ID)
	req.Provision.Model.BaseURL = strings.TrimSpace(req.Provision.Model.BaseURL)
	req.Provision.Model.API = strings.TrimSpace(req.Provision.Model.API)
	req.Provision.Model.Auth = strings.TrimSpace(req.Provision.Model.Auth)
	req.Provision.Model.APIKey = strings.TrimSpace(req.Provision.Model.APIKey)
	req.Provision.Telegram.DMPolicy = strings.TrimSpace(req.Provision.Telegram.DMPolicy)
	req.Provision.Telegram.GroupPolicy = strings.TrimSpace(req.Provision.Telegram.GroupPolicy)

	if req.Provision.Model.Provider == "" {
		return util.BadRequest(c, "provision.model.provider is required")
	}
	if req.Provision.Model.ID == "" {
		return util.BadRequest(c, "provision.model.id is required")
	}
	if req.Provision.Model.BaseURL == "" {
		return util.BadRequest(c, "provision.model.base_url is required")
	}
	if req.Provision.Model.API == "" {
		return util.BadRequest(c, "provision.model.api is required")
	}
	if req.Provision.Model.Auth == "" {
		return util.BadRequest(c, "provision.model.auth is required")
	}
	if req.Provision.Model.APIKey == "" {
		return util.BadRequest(c, "provision.model.api_key is required")
	}
	if req.Provision.Telegram.DMPolicy == "" {
		req.Provision.Telegram.DMPolicy = "open"
	}
	if req.Provision.Telegram.GroupPolicy == "" {
		req.Provision.Telegram.GroupPolicy = "allowlist"
	}

	if err := k8s.UpdateProvisionSettings(context.Background(), req.Provision); err != nil {
		return util.InternalError(c, "failed to update provision settings: "+err.Error())
	}

	return util.Success(c, map[string]interface{}{
		"message":   "provision settings updated, clawhost is restarting",
		"provision": req.Provision,
	})
}

func AdminListProvisionProviders(c echo.Context) error {
	settings := k8s.GetProvisionSettings()
	return util.Success(c, map[string]interface{}{
		"providers": settings.Providers,
	})
}

func AdminUpsertProvisionProvider(c echo.Context) error {
	var req UpsertAdminProviderRequest
	if err := c.Bind(&req); err != nil {
		return util.BadRequest(c, "invalid request body")
	}

	req.Name = strings.TrimSpace(req.Name)
	req.ID = strings.TrimSpace(req.ID)
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.API = strings.TrimSpace(req.API)
	req.Auth = strings.TrimSpace(req.Auth)
	req.APIKey = strings.TrimSpace(req.APIKey)

	if req.Name == "" {
		return util.BadRequest(c, "name is required")
	}
	if req.ID == "" {
		return util.BadRequest(c, "id is required")
	}
	if req.BaseURL == "" {
		return util.BadRequest(c, "base_url is required")
	}
	if req.API == "" {
		return util.BadRequest(c, "api is required")
	}
	if req.Auth == "" {
		return util.BadRequest(c, "auth is required")
	}
	if req.APIKey == "" {
		return util.BadRequest(c, "api_key is required")
	}

	settings := k8s.GetProvisionSettings()
	entry := k8s.ProvisionProviderEntry{
		Name:    req.Name,
		ID:      req.ID,
		BaseURL: req.BaseURL,
		API:     req.API,
		Auth:    req.Auth,
		APIKey:  req.APIKey,
	}

	if err := k8s.UpsertConfiguredProvider(context.Background(), entry, req.SetDefault, settings.Telegram); err != nil {
		return util.InternalError(c, "failed to save provider: "+err.Error())
	}

	return util.Success(c, map[string]interface{}{
		"message":     "provider saved, clawhost is restarting",
		"provider":    entry,
		"set_default": req.SetDefault,
	})
}

func AdminTestProvisionProvider(c echo.Context) error {
	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		return util.BadRequest(c, "provider name is required")
	}

	settings := k8s.GetProvisionSettings()
	for _, provider := range settings.Providers {
		if provider.Name != name {
			continue
		}
		result, err := k8s.TestConfiguredProvider(context.Background(), provider)
		if err != nil {
			return util.InternalError(c, "failed to test provider: "+err.Error())
		}
		return util.Success(c, result)
	}

	return util.NotFound(c, "provider not found")
}
