package v1

import (
	"context"
	"strings"

	"github.com/clawhost/clawhost/middleware"
	"github.com/clawhost/clawhost/model"
	"github.com/clawhost/clawhost/service/k8s"
	"github.com/clawhost/clawhost/util"
	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
)

type GetBotByUserResponse struct {
	HasBot bool            `json:"has_bot"`
	UserID string          `json:"user_id"`
	Bot    *GetBotResponse `json:"bot,omitempty"`
}

// GetLatestBotByUser returns the latest bot for a given user_id within the
// authenticated app. This is intended for business backends that map one user
// to one primary bot and want fresh status on each refresh.
// GET /bots/by-user/:user_id
func GetLatestBotByUser(c echo.Context) error {
	userID := c.Param("user_id")
	if userID == "" {
		userID = c.QueryParam("user_id")
	}
	if userID == "" {
		return util.BadRequest(c, "user_id is required")
	}

	var appID string
	if app := middleware.GetAppFromContext(c); app != nil {
		appID = app.ID
	}

	bots, err := model.ListBotsByAppAndUser(appID, userID)
	if err != nil {
		return util.InternalError(c, "failed to query bots")
	}
	if len(bots) == 0 {
		return util.Success(c, &GetBotByUserResponse{
			HasBot: false,
			UserID: userID,
		})
	}

	response, err := buildBotResponse(context.Background(), bots[0])
	if err != nil {
		return util.InternalError(c, "failed to build bot response")
	}

	return util.Success(c, &GetBotByUserResponse{
		HasBot: true,
		UserID: userID,
		Bot:    response,
	})
}

func buildBotResponse(ctx context.Context, bot *model.Bot) (*GetBotResponse, error) {
	response := &GetBotResponse{
		Bot:       bot,
		BotURL:    buildAccessURL(bot.Slug, ""),
		AccessURL: buildAccessURL(bot.Slug, bot.AccessToken),
	}

	enrichBotResponseFromConfig(response, bot)

	if bot.Status != model.BotStatusRunning {
		return response, nil
	}

	if statusInfo, err := k8s.GetDeploymentStatusInfo(ctx, bot.ID); err == nil {
		response.DeploymentStatus = statusInfo
	}

	if currentImage, err := k8s.GetDeploymentImage(ctx, bot.ID); err == nil {
		response.Image = currentImage
		latestImage := viper.GetString("openclaw.image")
		if latestImage != "" {
			response.LatestImage = latestImage
			upToDate := currentImage == latestImage
			response.ImageUpToDate = &upToDate
		}
	}

	if response.DeploymentStatus != nil && response.DeploymentStatus.Status == "ready" {
		if err := k8s.SyncConfigToDatabase(ctx, bot.ID); err == nil {
			if updatedBot, getErr := model.GetBotByID(bot.ID); getErr == nil {
				response.Bot = updatedBot
			}
		}
	}

	return response, nil
}

func enrichBotResponseFromConfig(response *GetBotResponse, bot *model.Bot) {
	config, err := bot.GetOpenClawConfig()
	if err != nil || config == nil {
		return
	}

	response.Provider, response.ModelName = extractModelSummary(config)
	response.Channels = extractChannelSummaries(config)
}

func extractModelSummary(config *model.OpenClawConfig) (string, string) {
	if config == nil {
		return "", ""
	}

	if config.Agents != nil && config.Agents.Defaults != nil && config.Agents.Defaults.Model != nil {
		primary := strings.TrimSpace(config.Agents.Defaults.Model.Primary)
		if primary != "" {
			parts := strings.SplitN(primary, "/", 2)
			if len(parts) == 2 {
				return parts[0], parts[1]
			}
			return "", primary
		}
	}

	if config.Models != nil {
		for provider, providerConfig := range config.Models.Providers {
			if providerConfig == nil || len(providerConfig.Models) == 0 {
				continue
			}
			modelName := providerConfig.Models[0].Name
			if modelName == "" {
				modelName = providerConfig.Models[0].ID
			}
			return provider, modelName
		}
	}

	return "", ""
}

func extractChannelSummaries(config *model.OpenClawConfig) []BotChannelSummary {
	if config == nil || len(config.Channels) == 0 {
		return nil
	}

	summaries := make([]BotChannelSummary, 0, len(config.Channels))
	for channelName, raw := range config.Channels {
		channelMap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		enabled := readBoolPointer(channelMap["enabled"])
		status := "configured"
		if enabled != nil {
			if *enabled {
				status = "configured"
			} else {
				status = "disabled"
			}
		}

		if accounts, ok := channelMap["accounts"].(map[string]interface{}); ok && len(accounts) > 0 {
			for accountName := range accounts {
				summary := BotChannelSummary{
					Channel: channelName,
					Account: accountName,
					Status:  status,
					Enabled: enabled,
				}
				summaries = append(summaries, summary)
			}
			continue
		}

		summaries = append(summaries, BotChannelSummary{
			Channel: channelName,
			Status:  status,
			Enabled: enabled,
		})
	}

	return summaries
}

func readBoolPointer(v interface{}) *bool {
	value, ok := v.(bool)
	if !ok {
		return nil
	}
	b := value
	return &b
}
