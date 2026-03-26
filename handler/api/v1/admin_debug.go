package v1

import (
	"context"

	"github.com/clawhost/clawhost/service/k8s"
	"github.com/clawhost/clawhost/util"
	"github.com/labstack/echo/v4"
)

type AdminCapacityResponse struct {
	Capacity *k8s.CapacityInfo `json:"capacity"`
}

// AdminGetCapacity returns a cluster capacity estimate for additional bot pods.
func AdminGetCapacity(c echo.Context) error {
	capacity, err := k8s.GetCapacityInfo(context.Background())
	if err != nil {
		return util.InternalError(c, "failed to calculate capacity: "+err.Error())
	}

	return util.Success(c, &AdminCapacityResponse{
		Capacity: capacity,
	})
}
