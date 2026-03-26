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
