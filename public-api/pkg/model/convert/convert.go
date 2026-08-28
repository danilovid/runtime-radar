package convert

import (
	"github.com/runtime-radar/runtime-radar/public-api/pkg/model"
)

func AccessTokensToResponse(accessTokens []*model.AccessToken) []*model.AccessTokenResp {
	resps := make([]*model.AccessTokenResp, 0, len(accessTokens))
	for _, at := range accessTokens {
		resps = append(resps, &model.AccessTokenResp{
			ID:            at.ID,
			Kind:          at.Kind,
			Name:          at.Name,
			UserID:        at.UserID,
			Permissions:   at.Permissions,
			ExpiresAt:     at.ExpiresAt,
			CreatedAt:     at.CreatedAt,
			InvalidatedAt: at.InvalidatedAt,
		})
	}

	return resps
}
