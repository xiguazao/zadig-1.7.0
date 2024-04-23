/*
 * Copyright 2023 The KodeRover Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/service"
	internalhandler "github.com/koderover/zadig/pkg/shared/handler"
	e "github.com/koderover/zadig/pkg/tool/errors"
)

func CreateFavorite(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	f := &models.Favorite{
		UserID:      ctx.UserID,
		ProductName: c.Query("projectName"),
		Name:        c.Param("name"),
		Type:        c.Param("type"),
	}
	switch f.Type {
	case service.FavoriteTypeEnv:
		if f.ProductName == "" {
			ctx.Err = e.ErrInvalidParam.AddDesc("empty projectName")
			return
		}
	default:
		ctx.Err = e.ErrInvalidParam.AddDesc("invalid type")
		return
	}

	ctx.Err = service.CreateFavorite(f)
	return
}

func DeleteFavorite(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	f := &mongodb.FavoriteArgs{
		UserID:      ctx.UserID,
		ProductName: c.Query("projectName"),
		Name:        c.Param("name"),
		Type:        c.Param("type"),
	}
	switch f.Type {
	case service.FavoriteTypeEnv:
		if f.ProductName == "" {
			ctx.Err = e.ErrInvalidParam.AddDesc("empty projectName")
			return
		}
	default:
		ctx.Err = e.ErrInvalidParam.AddDesc("invalid type")
		return
	}

	ctx.Err = service.DeleteFavorite(f)
	return
}
