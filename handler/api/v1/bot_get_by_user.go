package v1

import (
	"context"

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
	response := &GetBotResponse{Bot: bot}

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
