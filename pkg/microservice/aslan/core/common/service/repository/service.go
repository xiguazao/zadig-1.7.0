/*
Copyright 2023 The KodeRover Authors.

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

package repository

import (
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
)

func QueryTemplateService(option *mongodb.ServiceFindOption, production bool) (*models.Service, error) {
	if !production {
		return mongodb.NewServiceColl().Find(option)
	} else {
		return mongodb.NewProductionServiceColl().Find(option)
	}
}

func ListServicesWithSRevision(option *mongodb.SvcRevisionListOption, production bool) ([]*models.Service, error) {
	if !production {
		return mongodb.NewServiceColl().ListServicesWithSRevision(option)
	} else {
		return mongodb.NewProductionServiceColl().ListServicesWithSRevision(option)
	}
}

func ListMaxRevisionsServices(productName string, production bool) ([]*models.Service, error) {
	if !production {
		return mongodb.NewServiceColl().ListMaxRevisionsByProduct(productName)
	} else {
		return mongodb.NewProductionServiceColl().ListMaxRevisionsByProduct(productName)
	}
}

func GetMaxRevisionsServicesMap(productName string, production bool) (map[string]*models.Service, error) {
	svcMap := make(map[string]*models.Service)
	services, err := ListMaxRevisionsServices(productName, production)
	if err != nil {
		return nil, err
	}

	for _, svc := range services {
		svcMap[svc.ServiceName] = svc
	}

	return svcMap, nil
}
