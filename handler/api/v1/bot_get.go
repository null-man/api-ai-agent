package v1

import (
	"context"

	"github.com/clawhost/clawhost/middleware"
	"github.com/clawhost/clawhost/model"
	"github.com/clawhost/clawhost/service/k8s"
	"github.com/clawhost/clawhost/util"
	"github.com/labstack/echo/v4"
)

// GetBotResponse includes bot info and deployment status
type GetBotResponse struct {
	*model.Bot
	DeploymentStatus *k8s.DeploymentStatusInfo `json:"deployment_status,omitempty"`
	Image            string                    `json:"image,omitempty"`
	LatestImage      string                    `json:"latest_image,omitempty"`
	ImageUpToDate    *bool                     `json:"image_up_to_date,omitempty"`
	BotURL           string                    `json:"bot_url,omitempty"`
	AccessURL        string                    `json:"access_url,omitempty"`
	Provider         string                    `json:"provider,omitempty"`
	ModelName        string                    `json:"model_name,omitempty"`
	Channels         []BotChannelSummary       `json:"channels,omitempty"`
}

type BotChannelSummary struct {
	Channel string `json:"channel"`
	Account string `json:"account,omitempty"`
	Status  string `json:"status,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

func GetBot(c echo.Context) error {
	bot := middleware.GetBotFromContext(c)
	if bot == nil {
		return util.Forbidden(c, "not authorized")
	}

	response, err := buildBotResponse(context.Background(), bot)
	if err != nil {
		return util.InternalError(c, "failed to get bot")
	}

	return util.Success(c, response)
}
