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

package dto

import (
	"sync"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	"github.com/koderover/zadig/pkg/setting"
)

type Task struct {
	ID           primitive.ObjectID       `bson:"_id,omitempty"             json:"id,omitempty"`
	TaskID       int64                    `bson:"task_id"                   json:"task_id"`
	ProductName  string                   `bson:"product_name"              json:"product_name"`
	PipelineName string                   `bson:"pipeline_name"             json:"pipeline_name"`
	Type         config.PipelineType      `bson:"type"                      json:"type"`
	Status       config.Status            `bson:"status"                    json:"status,omitempty"`
	Description  string                   `bson:"description,omitempty"     json:"description,omitempty"`
	TaskCreator  string                   `bson:"task_creator"              json:"task_creator,omitempty"`
	TaskRevoker  string                   `bson:"task_revoker,omitempty"    json:"task_revoker,omitempty"`
	CreateTime   int64                    `bson:"create_time"               json:"create_time,omitempty"`
	StartTime    int64                    `bson:"start_time"                json:"start_time,omitempty"`
	EndTime      int64                    `bson:"end_time"                  json:"end_time,omitempty"`
	SubTasks     []map[string]interface{} `bson:"sub_tasks"                 json:"sub_tasks"`
	Stages       []*models.Stage          `bson:"stages"                    json:"stages"`
	ReqID        string                   `bson:"req_id,omitempty"          json:"req_id,omitempty"`
	AgentHost    string                   `bson:"agent_host,omitempty"      json:"agent_host,omitempty"`
	DockerHost   string                   `bson:"-"                         json:"docker_host,omitempty"`
	TeamName     string                   `bson:"team,omitempty"            json:"team,omitempty"`
	IsDeleted    bool                     `bson:"is_deleted"                json:"is_deleted"`
	IsArchived   bool                     `bson:"is_archived"               json:"is_archived"`
	AgentID      string                   `bson:"agent_id"                  json:"agent_id"`
	// is allowed to run multiple times
	MultiRun bool `bson:"multi_run"                 json:"multi_run"`
	// target is container name when k8s, service name when pm
	Target string `bson:"target,omitempty"                    json:"target"`
	// generate SubTasks with predefine build module,
	// query filter param:  ServiceTmpl,  BuildModuleVer
	// if nil，use pipeline self define SubTasks
	BuildModuleVer string `bson:"build_module_ver,omitempty"                 json:"build_module_ver"`
	ServiceName    string `bson:"service_name,omitempty"              json:"service_name,omitempty"`
	// TaskArgs single workflow args
	TaskArgs *models.TaskArgs `bson:"task_args,omitempty"         json:"task_args,omitempty"`
	// WorkflowArgs  multi workflow args
	WorkflowArgs *models.WorkflowTaskArgs `bson:"workflow_args"         json:"workflow_args,omitempty"`
	// TestArgs test workflow args
	TestArgs *models.TestTaskArgs `bson:"test_args,omitempty"         json:"test_args,omitempty"`
	// ServiceTaskArgs sh deploy args
	ServiceTaskArgs *models.ServiceTaskArgs `bson:"service_args,omitempty"         json:"service_args,omitempty"`
	// ArtifactPackageTaskArgs arguments for artifact-package type tasks
	ArtifactPackageTaskArgs *models.ArtifactPackageTaskArgs `bson:"artifact_package_args,omitempty"         json:"artifact_package_args,omitempty"`
	// ConfigPayload system config info
	ConfigPayload *models.ConfigPayload      `bson:"configpayload"                  json:"config_payload,omitempty"`
	Error         string                     `bson:"error,omitempty"                json:"error,omitempty"`
	Services      [][]*models.ProductService `bson:"services"                       json:"services"`
	Render        *models.RenderInfo         `bson:"render"                         json:"render"`
	StorageURI    string                     `bson:"storage_uri,omitempty"          json:"storage_uri,omitempty"`
	// interface{} 为types.TestReport
	TestReports      map[string]interface{}       `bson:"test_reports,omitempty" json:"test_reports,omitempty"`
	RwLock           sync.Mutex                   `bson:"-"                      json:"-"`
	ResetImage       bool                         `bson:"resetImage"             json:"resetImage"`
	ResetImagePolicy setting.ResetImagePolicyType `bson:"reset_image_policy"     json:"reset_image_policy"`
	TriggerBy        *models.TriggerBy            `bson:"trigger_by,omitempty"   json:"trigger_by,omitempty"`
	Features         []string                     `bson:"features"               json:"features"`
	IsRestart        bool                         `bson:"is_restart"             json:"is_restart"`
	StorageEndpoint  string                       `bson:"storage_endpoint"       json:"storage_endpoint"`
	Releases         []Release                    `json:"releases"`
}

type Release struct {
	ID      primitive.ObjectID `json:"id"`
	Version string             `json:"version"`
}
