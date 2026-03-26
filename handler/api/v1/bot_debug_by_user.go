package v1

import (
	"context"
	"strconv"

	"github.com/clawhost/clawhost/middleware"
	"github.com/clawhost/clawhost/model"
	"github.com/clawhost/clawhost/service/k8s"
	"github.com/clawhost/clawhost/util"
	"github.com/labstack/echo/v4"
)

type GetBotDebugByUserResponse struct {
	HasBot bool            `json:"has_bot"`
	UserID string          `json:"user_id"`
	Bot    *GetBotResponse `json:"bot,omitempty"`
	Pod    *k8s.PodInfo    `json:"pod,omitempty"`
	Logs   string          `json:"logs,omitempty"`
	Tail   int64           `json:"tail"`
}

// GetLatestBotDebugByUser returns the latest bot plus pod status and recent logs
// for a given user within the authenticated app.
// GET /bots/by-user/:user_id/debug?tail=200
func GetLatestBotDebugByUser(c echo.Context) error {
	userID := c.Param("user_id")
	if userID == "" {
		userID = c.QueryParam("user_id")
	}
	if userID == "" {
		return util.BadRequest(c, "user_id is required")
	}

	tail := int64(200)
	if t := c.QueryParam("tail"); t != "" {
		if parsed, err := strconv.ParseInt(t, 10, 64); err == nil && parsed > 0 && parsed <= 2000 {
			tail = parsed
		}
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
		return util.Success(c, &GetBotDebugByUserResponse{
			HasBot: false,
			UserID: userID,
			Tail:   tail,
		})
	}

	ctx := context.Background()
	response, err := buildBotResponse(ctx, bots[0])
	if err != nil {
		return util.InternalError(c, "failed to build bot response")
	}

	debugResp := &GetBotDebugByUserResponse{
		HasBot: true,
		UserID: userID,
		Bot:    response,
		Tail:   tail,
	}

	podInfo, err := k8s.GetLatestPodInfo(ctx, bots[0].ID)
	if err == nil && podInfo != nil {
		debugResp.Pod = podInfo
		logs, logErr := k8s.GetPodLogs(ctx, podInfo.Name, podInfo.Container, tail)
		if logErr == nil {
			debugResp.Logs = logs
		}
	}

	return util.Success(c, debugResp)
}
