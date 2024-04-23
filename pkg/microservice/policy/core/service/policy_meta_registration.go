/*
Copyright 2021 The KodeRover Authors.

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

package service

import (
	"sort"
	"strings"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/koderover/zadig/pkg/microservice/aslan/core/label/config"
	"github.com/koderover/zadig/pkg/microservice/policy/core/repository/models"
	"github.com/koderover/zadig/pkg/microservice/policy/core/repository/mongodb"
	"github.com/koderover/zadig/pkg/setting"
	"github.com/koderover/zadig/pkg/types"
)

type PolicyDefinition struct {
	Resource    string                  `json:"resource"`
	ResourceKey string                  `json:"resource_key"`
	Alias       string                  `json:"alias"`
	Description string                  `json:"description"`
	Rules       []*PolicyRuleDefinition `json:"rules"`
}

type PolicyRuleDefinition struct {
	Action      string `json:"action"`
	Alias       string `json:"alias"`
	Description string `json:"description"`
}

func CreateOrUpdatePolicyRegistration(p *types.PolicyMeta, _ *zap.SugaredLogger) error {
	obj := &models.PolicyMeta{
		Resource:    p.Resource,
		Alias:       p.Alias,
		Description: p.Description,
	}

	for _, r := range p.Rules {
		rule := &models.PolicyMetaRule{
			Action:      r.Action,
			Alias:       r.Alias,
			Description: r.Description,
		}
		for _, ar := range r.Rules {
			var as []models.Attribute
			for _, a := range ar.MatchAttributes {
				as = append(as, models.Attribute{Key: a.Key, Value: a.Value})
			}

			rule.Rules = append(rule.Rules, &models.ActionRule{
				Method:          ar.Method,
				Endpoint:        ar.Endpoint,
				ResourceType:    ar.ResourceType,
				IDRegex:         ar.IDRegex,
				MatchAttributes: as,
			})
		}
		obj.Rules = append(obj.Rules, rule)
	}

	return mongodb.NewPolicyMetaColl().UpdateOrCreate(obj)
}

var definitionMap = map[string]int{
	"Project":        1,
	"Template":       2,
	"TestCenter":     3,
	"ReleaseCenter":  4,
	"DeliveryCenter": 5,
	"DataCenter":     6,
}

var projectDefinitionMap = map[string]int{
	"Workflow":              1,
	"Environment":           2,
	"ProductionEnvironment": 3,
	"Service":               4,
	"ProductionService":     5,
	"Build":                 6,
	"Test":                  7,
	"Scan":                  8,
	"Delivery":              9,
}

func GetPolicyRegistrationDefinitions(scope, envType string, _ *zap.SugaredLogger) ([]*PolicyDefinition, error) {
	policieMetas, err := mongodb.NewPolicyMetaColl().List()
	if err != nil {
		return nil, err
	}
	systemScopeSet := sets.NewString("Project", "Template", "TestCenter", "ReleaseCenter", "DeliveryCenter", "DataCenter")
	projectScopeSet := sets.NewString("Workflow", "Environment", "ProductionEnvironment", "Test", "Delivery", "Build", "Service", "ProductionService", "Scan")
	systemPolicyMetas, projectPolicyMetas, filteredPolicyMetas := []*models.PolicyMeta{}, []*models.PolicyMeta{}, []*models.PolicyMeta{}
	for _, v := range policieMetas {
		if systemScopeSet.Has(v.Resource) {
			systemPolicyMetas = append(systemPolicyMetas, v)
		} else if projectScopeSet.Has(v.Resource) {
			projectPolicyMetas = append(projectPolicyMetas, v)
		}
	}

	switch scope {
	case string(types.SystemScope):
		for _, meta := range systemPolicyMetas {
			if meta.Resource == "TestCenter" {
				tmpRules := []*models.PolicyMetaRule{}
				for _, rule := range meta.Rules {
					if rule.Action == "create_test" {
						continue
					}
					tmpRules = append(tmpRules, rule)
				}

				meta.Rules = tmpRules
			}
		}
		filteredPolicyMetas = systemPolicyMetas
	case string(types.ProjectScope):
		tmp := []*models.PolicyMeta{}
		for _, meta := range projectPolicyMetas {
			if envType == setting.PMDeployType && (meta.Resource == "ProductionEnvironment" || meta.Resource == "Delivery") {
				continue
			}
			if envType == setting.TrusteeshipDeployType && meta.Resource == "Delivery" {
				continue
			}
			tmp = append(tmp, meta)
		}
		projectPolicyMetas = tmp
		for i, meta := range projectPolicyMetas {
			if meta.Resource == "Environment" || meta.Resource == "ProductionEnvironment" {
				tmpRules := []*models.PolicyMetaRule{}
				for _, rule := range meta.Rules {
					if rule.Action == "debug_pod" && envType == setting.PMDeployType {
						continue
					}
					if rule.Action == "ssh_pm" && envType != setting.PMDeployType {
						continue
					}
					tmpRules = append(tmpRules, rule)
				}
				projectPolicyMetas[i].Rules = tmpRules
			}
		}
		filteredPolicyMetas = projectPolicyMetas
	default:
		filteredPolicyMetas = policieMetas
	}
	var res []*PolicyDefinition
	for _, meta := range filteredPolicyMetas {
		pd := &PolicyDefinition{
			Resource:    meta.Resource,
			ResourceKey: meta.Resource,
			Alias:       meta.Alias,
			Description: meta.Description,
		}
		for _, r := range meta.Rules {
			pd.Rules = append(pd.Rules, &PolicyRuleDefinition{
				Action:      r.Action,
				Alias:       r.Alias,
				Description: r.Description,
			})
		}
		res = append(res, pd)
	}

	for _, v := range res {
		if v.Resource == string(config.ResourceTypeEnvironment) {
			productionEnvDef := &PolicyDefinition{
				Resource:    string(config.ResourceTypeEnvironment),
				ResourceKey: "ProductionEnvironment",
				Alias:       "生产环境",
			}
			productionRules := make([]*PolicyRuleDefinition, 0)
			normalRules := make([]*PolicyRuleDefinition, 0)
			// TODO ugly code
			for _, rule := range v.Rules {
				if strings.Contains(rule.Action, "production") {
					productionRules = append(productionRules, rule)
				} else {
					normalRules = append(normalRules, rule)
				}
			}
			v.Rules = normalRules
			productionEnvDef.Rules = productionRules
			res = append(res, productionEnvDef)
			break
		}
	}

	switch scope {
	case string(types.SystemScope):
		sort.Slice(res, func(i, j int) bool {
			return definitionMap[res[i].Resource] < definitionMap[res[j].Resource]
		})
	case string(types.ProjectScope):
		sort.Slice(res, func(i, j int) bool {
			return projectDefinitionMap[res[i].Resource] < projectDefinitionMap[res[j].Resource]
		})
	}

	return res, nil
}
