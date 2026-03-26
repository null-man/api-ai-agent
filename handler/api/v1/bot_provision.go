package v1

import (
	"context"
	"strings"
	"time"

	"github.com/clawhost/clawhost/middleware"
	"github.com/clawhost/clawhost/model"
	"github.com/clawhost/clawhost/service/k8s"
	"github.com/clawhost/clawhost/util"
	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
)

type ProvisionTelegramBotRequest struct {
	UserID           string     `json:"user_id"`
	Name             string     `json:"name"`
	Slug             string     `json:"slug,omitempty"`
	TelegramBotToken string     `json:"telegram_bot_token,omitempty"`
	BotToken         string     `json:"botToken,omitempty"`
	Token            string     `json:"token,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
}

type ProvisionTelegramBotResponse struct {
	*model.Bot
	AccessURL string `json:"access_url"`
}

// ProvisionTelegramBot creates a bot, applies the default model provider config,
// starts the bot, and wires Telegram using only a Telegram bot token.
// POST /bots/provision/telegram
func ProvisionTelegramBot(c echo.Context) error {
	var req ProvisionTelegramBotRequest
	if err := c.Bind(&req); err != nil {
		return util.BadRequest(c, "invalid request body")
	}

	req.UserID = strings.TrimSpace(req.UserID)
	req.Name = strings.TrimSpace(req.Name)
	req.Slug = strings.TrimSpace(req.Slug)

	if req.UserID == "" {
		return util.BadRequest(c, "user_id is required")
	}
	if req.Name == "" {
		return util.BadRequest(c, "name is required")
	}

	telegramBotToken := strings.TrimSpace(req.TelegramBotToken)
	if telegramBotToken == "" {
		telegramBotToken = strings.TrimSpace(req.BotToken)
	}
	if telegramBotToken == "" {
		telegramBotToken = strings.TrimSpace(req.Token)
	}
	if telegramBotToken == "" {
		return util.BadRequest(c, "telegram_bot_token is required")
	}

	if req.Slug != "" {
		if !isValidSlug(req.Slug) {
			return util.BadRequest(c, "slug must be 1-50 characters, lowercase letters, numbers, and hyphens only")
		}
		existing, _ := model.GetBotBySlug(req.Slug)
		if existing != nil {
			return util.BadRequest(c, "slug is already taken")
		}
	}

	app := middleware.GetAppFromContext(c)
	if app == nil {
		return util.Forbidden(c, "not authorized")
	}

	openclawConfig, err := buildProvisionTelegramConfig(telegramBotToken)
	if err != nil {
		return util.InternalError(c, err.Error())
	}

	bot := &model.Bot{
		AppID:     app.ID,
		UserID:    req.UserID,
		Name:      req.Name,
		Slug:      req.Slug,
		Status:    model.BotStatusCreated,
		ExpiresAt: req.ExpiresAt,
	}

	if err := bot.SetOpenClawConfig(openclawConfig); err != nil {
		return util.InternalError(c, "failed to set provision config")
	}
	if err := model.CreateBot(bot); err != nil {
		return util.InternalError(c, "failed to create bot")
	}

	ctx := context.Background()
	k8sConfig := convertToK8sConfig(bot, openclawConfig)

	if err := k8s.CreateDeployment(ctx, bot.ID, bot.UserID, bot.AccessToken, k8sConfig); err != nil {
		return util.InternalError(c, "failed to create deployment: "+err.Error())
	}

	endpoint, err := k8s.CreateService(ctx, bot.ID, bot.UserID)
	if err != nil {
		k8s.DeleteDeployment(ctx, bot.ID)
		return util.InternalError(c, "failed to create service: "+err.Error())
	}

	if err := model.UpdateBotStatus(bot.ID, model.BotStatusStarting, endpoint); err != nil {
		return util.InternalError(c, "failed to update bot status")
	}

	go func() {
		bgCtx := context.Background()
		_, waitErr := k8s.WaitForPodReady(bgCtx, bot.ID, 120)
		if waitErr != nil {
			model.UpdateBotStatus(bot.ID, model.BotStatusError, endpoint)
			return
		}
		model.UpdateBotStatus(bot.ID, model.BotStatusRunning, endpoint)
		if k8sConfig.AccessToken != "" {
			_ = k8s.WriteConfigToBot(bgCtx, bot.ID, k8sConfig, false)
		}
	}()

	bot.Status = model.BotStatusStarting
	bot.Endpoint = endpoint

	return util.Success(c, &ProvisionTelegramBotResponse{
		Bot:       bot,
		AccessURL: buildAccessURL(bot.Slug, bot.AccessToken),
	})
}

func buildProvisionTelegramConfig(telegramBotToken string) (*model.OpenClawConfig, error) {
	providerName := strings.TrimSpace(viper.GetString("provision.model.provider"))
	if providerName == "" {
		providerName = "minimax"
	}

	modelID := strings.TrimSpace(viper.GetString("provision.model.id"))
	if modelID == "" {
		modelID = "MiniMax-M2.5-highspeed"
	}

	baseURL := strings.TrimSpace(viper.GetString("provision.model.base_url"))
	if baseURL == "" {
		baseURL = "https://api.minimax.io/v1"
	}

	apiKey := strings.TrimSpace(viper.GetString("provision.model.api_key"))
	if apiKey == "" {
		return nil, echo.NewHTTPError(500, "server provision.model.api_key is not configured")
	}

	api := strings.TrimSpace(viper.GetString("provision.model.api"))
	if api == "" {
		api = "openai-completions"
	}

	auth := strings.TrimSpace(viper.GetString("provision.model.auth"))
	if auth == "" {
		auth = "api-key"
	}

	dmPolicy := strings.TrimSpace(viper.GetString("provision.telegram.dm_policy"))
	if dmPolicy == "" {
		dmPolicy = "open"
	}

	groupPolicy := strings.TrimSpace(viper.GetString("provision.telegram.group_policy"))
	if groupPolicy == "" {
		groupPolicy = "allowlist"
	}

	channel := map[string]interface{}{
		"enabled":     true,
		"dmPolicy":    dmPolicy,
		"groupPolicy": groupPolicy,
		"accounts": map[string]interface{}{
			"default": map[string]interface{}{
				"botToken": telegramBotToken,
			},
		},
	}
	if dmPolicy == "open" {
		channel["allowFrom"] = []string{"*"}
	}

	return &model.OpenClawConfig{
		Models: &model.ModelsConfig{
			Mode: "merge",
			Providers: map[string]*model.ProviderConfig{
				providerName: {
					BaseURL: baseURL,
					APIKey:  apiKey,
					Auth:    auth,
					API:     api,
					Models: []model.ProviderModelConfig{
						{
							ID:            modelID,
							Name:          modelID,
							Input:         []string{"text"},
							ContextWindow: 200000,
							MaxTokens:     8192,
						},
					},
				},
			},
		},
		Agents: &model.AgentsConfig{
			Defaults: &model.AgentDefaultsConfig{
				Model: &model.AgentModelConfig{
					Primary: providerName + "/" + modelID,
				},
			},
		},
		Channels: model.ChannelsConfig{
			"telegram": channel,
		},
	}, nil
}
