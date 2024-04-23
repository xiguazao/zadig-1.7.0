/*
Copyright 2022 The KodeRover Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/koderover/zadig/pkg/tool/crypto"

	commonservice "github.com/koderover/zadig/pkg/microservice/aslan/core/common/service"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/system/service"
	internalhandler "github.com/koderover/zadig/pkg/shared/handler"
	e "github.com/koderover/zadig/pkg/tool/errors"
	"github.com/koderover/zadig/pkg/tool/log"
)

var SonarIntegrationValidationError = errors.New("name and server must be provided")

func CreateSonarIntegration(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	args := new(service.SonarIntegration)

	data, err := c.GetRawData()
	if err != nil {
		log.Errorf("Create sonar integration GetRawData err : %s", err)
	}
	if err = json.Unmarshal(data, args); err != nil {
		log.Errorf("Create sonar integration Unmarshal err : %s", err)
	}
	internalhandler.InsertOperationLog(c, ctx.UserName, "", "新增", "系统配置-Sonar集成", fmt.Sprintf("server: %s, token: %s", args.ServerAddress, args.Token), string(data), ctx.Logger)

	if err != nil {
		ctx.Err = fmt.Errorf("failed to update sonar integration: %s", err)
		return
	}

	if args.ServerAddress == "" || args.Token == "" {
		ctx.Err = SonarIntegrationValidationError
		return
	}
	ctx.Err = service.CreateSonarIntegration(args, ctx.Logger)
}

func UpdateSonarIntegration(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	args := new(service.SonarIntegration)

	data, err := c.GetRawData()
	if err != nil {
		log.Errorf("Update sonar integration GetRawData err : %s", err)
	}
	if err = json.Unmarshal(data, args); err != nil {
		log.Errorf("Update sonar integration Unmarshal err : %s", err)
	}
	internalhandler.InsertOperationLog(c, ctx.UserName, "", "更新", "系统配置-Sonar集成", fmt.Sprintf("server: %s, token: %s", args.ServerAddress, args.Token), string(data), ctx.Logger)

	if err != nil {
		ctx.Err = fmt.Errorf("failed to update sonar integration: %s", err)
		return
	}

	if args.ServerAddress == "" || args.Token == "" {
		ctx.Err = SonarIntegrationValidationError
		return
	}
	ctx.Err = service.UpdateSonarIntegration(c.Param("id"), args, ctx.Logger)
}

func ListSonarIntegration(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	encryptedKey := c.Query("encryptedKey")
	if len(encryptedKey) == 0 {
		ctx.Err = e.ErrInvalidParam
		return
	}

	aesKey, err := commonservice.GetAesKeyFromEncryptedKey(encryptedKey, ctx.Logger)
	if err != nil {
		ctx.Err = err
		return
	}

	sonarList, _, err := service.ListSonarIntegration(ctx.Logger)
	if err != nil {
		ctx.Err = err
		return
	}

	for _, sonar := range sonarList {
		encryptedSonarToken, err := crypto.AesEncryptByKey(sonar.Token, aesKey.PlainText)
		if err != nil {
			ctx.Err = fmt.Errorf("failed to encrypt sonar token, err: %s", err)
			return
		}
		sonar.Token = encryptedSonarToken
	}
	ctx.Resp = sonarList
}

func GetSonarIntegration(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	encryptedKey := c.Query("encryptedKey")
	if len(encryptedKey) == 0 {
		ctx.Err = e.ErrInvalidParam
		return
	}

	aesKey, err := commonservice.GetAesKeyFromEncryptedKey(encryptedKey, ctx.Logger)
	if err != nil {
		ctx.Err = err
		return
	}

	resp, err := service.GetSonarIntegration(c.Param("id"), ctx.Logger)
	if err != nil {
		ctx.Err = err
		return
	}
	encryptedSonarToken, err := crypto.AesEncryptByKey(resp.Token, aesKey.PlainText)
	if err != nil {
		ctx.Err = fmt.Errorf("failed to encrypt sonar token, err: %s", err)
		return
	}
	resp.Token = encryptedSonarToken
	ctx.Resp = resp
}

func DeleteSonarIntegration(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	internalhandler.InsertOperationLog(c, ctx.UserName, "", "删除", "系统配置-Sonar集成", fmt.Sprintf("id:%s", c.Param("id")), "", ctx.Logger)
	ctx.Err = service.DeleteSonarIntegration(c.Param("id"), ctx.Logger)
}

func ValidateSonarInformation(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	args := new(service.SonarIntegration)

	data, err := c.GetRawData()
	if err != nil {
		log.Errorf("Validate sonar integration GetRawData err : %s", err)
		ctx.Err = fmt.Errorf("validate sonar integration GetRawData err : %s", err)
		return
	}
	if err = json.Unmarshal(data, args); err != nil {
		log.Errorf("Validate sonar integration Unmarshal err : %s", err)
		ctx.Err = fmt.Errorf("validate sonar integration Unmarshal err : %s", err)
		return
	}

	ctx.Err = service.ValidateSonarIntegration(args, ctx.Logger)
}
