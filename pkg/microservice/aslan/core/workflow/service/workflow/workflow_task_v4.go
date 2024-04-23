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

package workflow

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	commonmodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	commonrepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/instantmessage"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/lark"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/s3"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/scmnotify"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/workflowcontroller"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/workflow/service/workflow/job"
	jobctl "github.com/koderover/zadig/pkg/microservice/aslan/core/workflow/service/workflow/job"
	"github.com/koderover/zadig/pkg/microservice/user/core"
	"github.com/koderover/zadig/pkg/microservice/user/core/repository/orm"
	"github.com/koderover/zadig/pkg/setting"
	e "github.com/koderover/zadig/pkg/tool/errors"
	krkubeclient "github.com/koderover/zadig/pkg/tool/kube/client"
	"github.com/koderover/zadig/pkg/tool/kube/getter"
	"github.com/koderover/zadig/pkg/tool/kube/podexec"
	larktool "github.com/koderover/zadig/pkg/tool/lark"
	"github.com/koderover/zadig/pkg/tool/log"
	s3tool "github.com/koderover/zadig/pkg/tool/s3"
	"github.com/koderover/zadig/pkg/types"
	jobspec "github.com/koderover/zadig/pkg/types/job"
	"github.com/koderover/zadig/pkg/types/step"
	stepspec "github.com/koderover/zadig/pkg/types/step"
)

const (
	checkShellStepStart  = "ls /zadig/debug/shell_step"
	checkShellStepDone   = "ls /zadig/debug/shell_step_done"
	setOrUnsetBreakpoint = "%s /zadig/debug/breakpoint_%s"
)

type CreateTaskV4Resp struct {
	ProjectName  string `json:"project_name"`
	WorkflowName string `json:"workflow_name"`
	TaskID       int64  `json:"task_id"`
}

type WorkflowTaskPreview struct {
	TaskID              int64                 `bson:"task_id"                   json:"task_id"`
	WorkflowName        string                `bson:"workflow_name"             json:"workflow_name"`
	WorkflowDisplayName string                `bson:"workflow_display_name"     json:"workflow_display_name"`
	Params              []*commonmodels.Param `bson:"params"                    json:"params"`
	Status              config.Status         `bson:"status"                    json:"status,omitempty"`
	TaskCreator         string                `bson:"task_creator"              json:"task_creator,omitempty"`
	TaskRevoker         string                `bson:"task_revoker,omitempty"    json:"task_revoker,omitempty"`
	CreateTime          int64                 `bson:"create_time"               json:"create_time,omitempty"`
	StartTime           int64                 `bson:"start_time"                json:"start_time,omitempty"`
	EndTime             int64                 `bson:"end_time"                  json:"end_time,omitempty"`
	Stages              []*StageTaskPreview   `bson:"stages"                    json:"stages"`
	ProjectName         string                `bson:"project_name"              json:"project_name"`
	Error               string                `bson:"error,omitempty"           json:"error,omitempty"`
	IsRestart           bool                  `bson:"is_restart"                json:"is_restart"`
	Debug               bool                  `bson:"debug"                     json:"debug"`
}

type StageTaskPreview struct {
	Name      string                 `bson:"name"          json:"name"`
	Status    config.Status          `bson:"status"        json:"status"`
	StartTime int64                  `bson:"start_time"    json:"start_time,omitempty"`
	EndTime   int64                  `bson:"end_time"      json:"end_time,omitempty"`
	Parallel  bool                   `bson:"parallel"      json:"parallel"`
	Approval  *commonmodels.Approval `bson:"approval"      json:"approval"`
	Jobs      []*JobTaskPreview      `bson:"jobs"          json:"jobs"`
}

type JobTaskPreview struct {
	Name             string        `bson:"name"           json:"name"`
	JobType          string        `bson:"type"           json:"type"`
	Status           config.Status `bson:"status"         json:"status"`
	StartTime        int64         `bson:"start_time"     json:"start_time,omitempty"`
	EndTime          int64         `bson:"end_time"       json:"end_time,omitempty"`
	CostSeconds      int64         `bson:"cost_seconds"   json:"cost_seconds,omitempty"`
	Error            string        `bson:"error"          json:"error"`
	BreakpointBefore bool          `bson:"breakpoint_before" json:"breakpoint_before"`
	BreakpointAfter  bool          `bson:"breakpoint_after"  json:"breakpoint_after"`
	Spec             interface{}   `bson:"spec"           json:"spec"`
	// JobInfo contains the fields that make up the job task name, for frontend display
	JobInfo interface{} `bson:"job_info" json:"job_info"`
}

type ZadigBuildJobSpec struct {
	Repos         []*types.Repository    `bson:"repos"           json:"repos"`
	Image         string                 `bson:"image"           json:"image"`
	ServiceName   string                 `bson:"service_name"    json:"service_name"`
	ServiceModule string                 `bson:"service_module"  json:"service_module"`
	Envs          []*commonmodels.KeyVal `bson:"envs"            json:"envs"`
}

type ZadigTestingJobSpec struct {
	Repos         []*types.Repository    `bson:"repos"           json:"repos"`
	JunitReport   bool                   `bson:"junit_report"    json:"junit_report"`
	Archive       bool                   `bson:"archive"         json:"archive"`
	HtmlReport    bool                   `bson:"html_report"     json:"html_report"`
	ProjectName   string                 `bson:"project_name"    json:"project_name"`
	TestName      string                 `bson:"test_name"       json:"test_name"`
	TestType      string                 `bson:"test_type"       json:"test_type"`
	ServiceName   string                 `bson:"service_name"    json:"service_name"`
	ServiceModule string                 `bson:"service_module"  json:"service_module"`
	Envs          []*commonmodels.KeyVal `bson:"envs"            json:"envs"`
}

type ZadigScanningJobSpec struct {
	Repos        []*types.Repository `bson:"repos"           json:"repos"`
	LinkURL      string              `bson:"link_url"        json:"link_url"`
	ScanningName string              `bson:"scanning_name"   json:"scanning_name"`
}

type ZadigDeployJobSpec struct {
	Env                string                        `bson:"env"                          json:"env"`
	SkipCheckRunStatus bool                          `bson:"skip_check_run_status"        json:"skip_check_run_status"`
	ServiceAndImages   []*ServiceAndImage            `bson:"service_and_images"           json:"service_and_images"`
	YamlContent        string                        `bson:"yaml_content"                 json:"yaml_content"`
	KeyVals            []*commonmodels.ServiceKeyVal `bson:"key_vals"                     json:"key_vals"`
}

type CustomDeployJobSpec struct {
	Image              string `bson:"image"                        json:"image"`
	Target             string `bson:"target"                       json:"target"`
	ClusterName        string `bson:"cluster_name"                 json:"cluster_name"`
	Namespace          string `bson:"namespace"                    json:"namespace"`
	SkipCheckRunStatus bool   `bson:"skip_check_run_status"        json:"skip_check_run_status"`
}

type ServiceAndImage struct {
	ServiceName   string `bson:"service_name"           json:"service_name"`
	ServiceModule string `bson:"service_module"         json:"service_module"`
	Image         string `bson:"image"                  json:"image"`
}

type K8sCanaryDeployJobSpec struct {
	Image          string               `bson:"image"                        json:"image"`
	K8sServiceName string               `bson:"k8s_service_name"             json:"k8s_service_name"`
	ClusterName    string               `bson:"cluster_name"                 json:"cluster_name"`
	Namespace      string               `bson:"namespace"                    json:"namespace"`
	ContainerName  string               `bson:"container_name"               json:"container_name"`
	CanaryReplica  int                  `bson:"canary_replica"               json:"canary_replica"`
	Events         *commonmodels.Events `bson:"events"                       json:"events"`
}

type K8sCanaryReleaseJobSpec struct {
	Image          string               `bson:"image"                        json:"image"`
	K8sServiceName string               `bson:"k8s_service_name"             json:"k8s_service_name"`
	ClusterName    string               `bson:"cluster_name"                 json:"cluster_name"`
	Namespace      string               `bson:"namespace"                    json:"namespace"`
	ContainerName  string               `bson:"container_name"               json:"container_name"`
	Events         *commonmodels.Events `bson:"events"                       json:"events"`
}

type K8sBlueGreenDeployJobSpec struct {
	Image          string               `bson:"image"                        json:"image"`
	K8sServiceName string               `bson:"k8s_service_name"             json:"k8s_service_name"`
	ClusterName    string               `bson:"cluster_name"                 json:"cluster_name"`
	Namespace      string               `bson:"namespace"                    json:"namespace"`
	ContainerName  string               `bson:"container_name"               json:"container_name"`
	Events         *commonmodels.Events `bson:"events"                       json:"events"`
}

type K8sBlueGreenReleaseJobSpec struct {
	Image          string               `bson:"image"                        json:"image"`
	K8sServiceName string               `bson:"k8s_service_name"             json:"k8s_service_name"`
	ClusterName    string               `bson:"cluster_name"                 json:"cluster_name"`
	Namespace      string               `bson:"namespace"                    json:"namespace"`
	ContainerName  string               `bson:"container_name"               json:"container_name"`
	Events         *commonmodels.Events `bson:"events"                       json:"events"`
}

type DistributeImageJobSpec struct {
	SourceRegistryID string                       `bson:"source_registry_id"           json:"source_registry_id"`
	TargetRegistryID string                       `bson:"target_registry_id"           json:"target_registry_id"`
	DistributeTarget []*step.DistributeTaskTarget `bson:"distribute_target"            json:"distribute_target"`
}

func GetWorkflowv4Preset(encryptedKey, workflowName, uid, username string, log *zap.SugaredLogger) (*commonmodels.WorkflowV4, error) {
	workflow, err := commonrepo.NewWorkflowV4Coll().Find(workflowName)
	if err != nil {
		log.Errorf("cannot find workflow %s, the error is: %v", workflowName, err)
		return nil, e.ErrPresetWorkflow.AddDesc(err.Error())
	}
	for _, stage := range workflow.Stages {
		for _, job := range stage.Jobs {
			if err := jobctl.SetPreset(job, workflow); err != nil {
				log.Errorf("cannot get workflow %s preset, the error is: %v", workflowName, err)
				return nil, e.ErrPresetWorkflow.AddDesc(err.Error())
			}
		}
	}

	if err := ensureWorkflowV4Resp(encryptedKey, workflow, log); err != nil {
		return workflow, err
	}
	return workflow, nil
}

func CheckWorkflowV4LarkApproval(workflowName, uid string, log *zap.SugaredLogger) error {
	workflow, err := commonrepo.NewWorkflowV4Coll().Find(workflowName)
	if err != nil {
		log.Errorf("cannot find workflow %s, the error is: %v", workflowName, err)
		return e.ErrFindWorkflow.AddErr(err)
	}
	userInfo, err := orm.GetUserByUid(uid, core.DB)
	if err != nil || userInfo == nil {
		return errors.New("failed to get user info by id")
	}

	for _, stage := range workflow.Stages {
		if stage.Approval != nil && stage.Approval.Enabled && stage.Approval.Type == config.LarkApproval && stage.Approval.LarkApproval != nil {
			cli, err := lark.GetLarkClientByIMAppID(stage.Approval.LarkApproval.ApprovalID)
			if err != nil {
				return errors.Errorf("failed to get lark app info by id-%s", stage.Approval.LarkApproval.ApprovalID)
			}
			_, err = cli.GetUserOpenIDByEmailOrMobile(larktool.QueryTypeMobile, userInfo.Phone)
			if err != nil {
				return e.ErrCheckLarkApprovalCreator.AddDesc(fmt.Sprintf("failed to get lark user info. lark app id: %s, phone: %s, error: %v", stage.Approval.LarkApproval.ApprovalID, userInfo.Phone, err))
			}
		}
	}
	return nil
}

type CreateWorkflowTaskV4Args struct {
	Name   string
	UserID string
}

func CreateWorkflowTaskV4ByBuildInTrigger(triggerName string, args *commonmodels.WorkflowV4, log *zap.SugaredLogger) (*CreateTaskV4Resp, error) {
	resp := &CreateTaskV4Resp{
		ProjectName:  args.Project,
		WorkflowName: args.Name,
	}
	workflow, err := mongodb.NewWorkflowV4Coll().Find(args.Name)
	if err != nil {
		errMsg := fmt.Sprintf("cannot find workflow %s, the error is: %v", args.Name, err)
		log.Error(errMsg)
		return resp, e.ErrCreateTask.AddDesc(errMsg)
	}
	if err := job.MergeArgs(workflow, args); err != nil {
		errMsg := fmt.Sprintf("merge workflow args error: %v", err)
		log.Error(errMsg)
		return resp, e.ErrCreateTask.AddDesc(errMsg)
	}
	return CreateWorkflowTaskV4(&CreateWorkflowTaskV4Args{Name: triggerName}, workflow, log)
}

func CreateWorkflowTaskV4(args *CreateWorkflowTaskV4Args, workflow *commonmodels.WorkflowV4, log *zap.SugaredLogger) (*CreateTaskV4Resp, error) {
	resp := &CreateTaskV4Resp{
		ProjectName:  workflow.Project,
		WorkflowName: workflow.Name,
	}
	if err := LintWorkflowV4(workflow, log); err != nil {
		return resp, err
	}

	dbWorkflow, err := commonrepo.NewWorkflowV4Coll().Find(workflow.Name)
	if err != nil {
		log.Errorf("cannot find workflow %s, the error is: %v", workflow.Name, err)
		return nil, e.ErrFindWorkflow.AddDesc(err.Error())
	}

	if err := jobctl.InstantiateWorkflow(workflow); err != nil {
		log.Errorf("instantiate workflow error: %s", err)
		return resp, e.ErrCreateTask.AddErr(err)
	}

	workflowTask := &commonmodels.WorkflowTask{}

	// if user info exists, get user email and put it to workflow task info
	if args.UserID != "" {
		userInfo, err := orm.GetUserByUid(args.UserID, core.DB)
		if err != nil || userInfo == nil {
			return resp, errors.New("failed to get user info by uid")
		}
		workflowTask.TaskCreatorEmail = userInfo.Email
		workflowTask.TaskCreatorPhone = userInfo.Phone
	}

	// save workflow original workflow task args.
	originTaskArgs := &commonmodels.WorkflowV4{}
	if err := commonmodels.IToi(workflow, originTaskArgs); err != nil {
		log.Errorf("save original workflow args error: %v", err)
		return resp, e.ErrCreateTask.AddDesc(err.Error())
	}
	workflowTask.OriginWorkflowArgs = originTaskArgs
	nextTaskID, err := commonrepo.NewCounterColl().GetNextSeq(fmt.Sprintf(setting.WorkflowTaskV4Fmt, workflow.Name))
	if err != nil {
		log.Errorf("Counter.GetNextSeq error: %v", err)
		return resp, e.ErrGetCounter.AddDesc(err.Error())
	}
	resp.TaskID = nextTaskID

	if err := jobctl.RemoveFixedValueMarks(workflow); err != nil {
		log.Errorf("RemoveFixedValueMarks error: %v", err)
		return resp, e.ErrCreateTask.AddDesc(err.Error())
	}

	workflowTask.TaskID = nextTaskID
	workflowTask.TaskCreator = args.Name
	workflowTask.TaskRevoker = args.Name
	workflowTask.CreateTime = time.Now().Unix()
	workflowTask.WorkflowName = workflow.Name
	workflowTask.WorkflowDisplayName = workflow.DisplayName
	workflowTask.ProjectName = workflow.Project
	workflowTask.Params = workflow.Params
	workflowTask.KeyVals = workflow.KeyVals
	workflowTask.MultiRun = workflow.MultiRun
	workflowTask.ShareStorages = workflow.ShareStorages
	workflowTask.IsDebug = workflow.Debug
	workflowTask.WorkflowHash = fmt.Sprintf("%x", dbWorkflow.CalculateHash())
	// set workflow params repo info, like commitid, branch etc.
	setZadigParamRepos(workflow, log)
	for _, stage := range workflow.Stages {
		stageTask := &commonmodels.StageTask{
			Name:     stage.Name,
			Parallel: stage.Parallel,
			Approval: stage.Approval,
		}
		for _, job := range stage.Jobs {
			if jobctl.JobSkiped(job) {
				continue
			}
			// TODO: move this logic to job controller
			if job.JobType == config.JobZadigBuild {
				if err := setZadigBuildRepos(job, log); err != nil {
					log.Errorf("zadig build job set build info error: %v", err)
					return resp, e.ErrCreateTask.AddDesc(err.Error())
				}
			}
			if job.JobType == config.JobFreestyle {
				if err := setFreeStyleRepos(job, log); err != nil {
					log.Errorf("freestyle job set build info error: %v", err)
					return resp, e.ErrCreateTask.AddDesc(err.Error())
				}
			}
			if job.JobType == config.JobZadigTesting {
				if err := setZadigTestingRepos(job, log); err != nil {
					log.Errorf("testing job set build info error: %v", err)
					return resp, e.ErrCreateTask.AddDesc(err.Error())
				}
			}

			if job.JobType == config.JobZadigScanning {
				if err := setZadigScanningRepos(job, log); err != nil {
					log.Errorf("scanning job set build info error: %v", err)
					return resp, e.ErrCreateTask.AddDesc(err.Error())
				}
			}

			jobs, err := jobctl.ToJobs(job, workflow, nextTaskID)
			if err != nil {
				log.Errorf("cannot create workflow %s, the error is: %v", workflow.Name, err)
				return resp, e.ErrCreateTask.AddDesc(err.Error())
			}
			// add breakpoint_before when workflowTask is debug mode
			for _, jobTask := range jobs {
				switch config.JobType(jobTask.JobType) {
				case config.JobFreestyle, config.JobZadigTesting, config.JobZadigBuild, config.JobZadigScanning:
					if workflowTask.IsDebug {
						jobTask.BreakpointBefore = true
					}
				}
			}

			stageTask.Jobs = append(stageTask.Jobs, jobs...)
		}
		if len(stageTask.Jobs) > 0 {
			workflowTask.Stages = append(workflowTask.Stages, stageTask)
		}
	}

	if err := jobctl.RenderGlobalVariables(workflow, nextTaskID, args.Name); err != nil {
		log.Errorf("RenderGlobalVariables error: %v", err)
		return resp, e.ErrCreateTask.AddDesc(err.Error())
	}

	if err := workflowTaskLint(workflowTask, log); err != nil {
		return resp, err
	}

	workflowTask.WorkflowArgs = workflow
	workflowTask.Status = config.StatusCreated
	workflowTask.StartTime = time.Now().Unix()
	if err := instantmessage.NewWeChatClient().SendWorkflowTaskNotifications(workflowTask); err != nil {
		log.Errorf("send workflow task notification failed, error: %v", err)
	}

	if err := workflowcontroller.CreateTask(workflowTask); err != nil {
		log.Errorf("create workflow task error: %v", err)
		return resp, e.ErrCreateTask.AddDesc(err.Error())
	}
	// Updating the comment in the git repository, this will not cause the function to return error if this function call fails
	if err := scmnotify.NewService().UpdateWebhookCommentForWorkflowV4(workflowTask, log); err != nil {
		log.Warnf("Failed to update comment for custom workflow %s, taskID: %d the error is: %s", workflowTask.WorkflowName, workflowTask.TaskID, err)
	}
	if err := scmnotify.NewService().UpdateGitCheckForWorkflowV4(workflowTask.WorkflowArgs, workflowTask.TaskID, log); err != nil {
		log.Warnf("Failed to update github check status for custom workflow %s, taskID: %d the error is: %s", workflowTask.WorkflowName, workflowTask.TaskID, err)
	}

	return resp, nil
}

func CloneWorkflowTaskV4(workflowName string, taskID int64, logger *zap.SugaredLogger) (*commonmodels.WorkflowV4, error) {
	task, err := commonrepo.NewworkflowTaskv4Coll().Find(workflowName, taskID)
	if err != nil {
		logger.Errorf("find workflowTaskV4 error: %s", err)
		return nil, e.ErrGetTask.AddErr(err)
	}
	return task.OriginWorkflowArgs, nil
}

func SetWorkflowTaskV4Breakpoint(workflowName, jobName string, taskID int64, set bool, position string, logger *zap.SugaredLogger) error {
	w := workflowcontroller.GetWorkflowTaskInMap(workflowName, taskID)
	if w == nil {
		logger.Error("set workflowTaskV4 breakpoint failed: not found task")
		return e.ErrSetBreakpoint.AddDesc("工作流任务已完成或不存在")
	}
	w.Lock()
	var ack func()
	defer func() {
		if ack != nil {
			ack()
		}
		w.Unlock()
	}()
	var task *commonmodels.JobTask
FOR:
	for _, stage := range w.WorkflowTask.Stages {
		for _, jobTask := range stage.Jobs {
			if jobTask.Name == jobName {
				task = jobTask
				break FOR
			}
		}
	}
	if task == nil {
		logger.Error("set workflowTaskV4 breakpoint failed: not found job")
		return e.ErrSetBreakpoint.AddDesc("当前任务不存在")
	}
	// job task has not run, update data in memory and ack
	if task.Status == "" {
		switch position {
		case "before":
			task.BreakpointBefore = set
			ack = w.Ack
		case "after":
			task.BreakpointAfter = set
			ack = w.Ack
		}
		logger.Infof("set workflowTaskV4 breakpoint success: %s-%s %v", jobName, position, set)
		return nil
	}
	// job task is running, check whether shell step has run, and touch breakpoint file
	// if job task status is debug_after, only breakpoint operation can do is unset breakpoint_after, which should be done by StopDebugWorkflowTaskJobV4
	// if job task status is prepare, setting breakpoints has a low probability of not taking effect, and the current design allows for this flaw
	if task.Status == config.StatusRunning || task.Status == config.StatusDebugBefore || task.Status == config.StatusPrepare {
		jobTaskSpec := &commonmodels.JobTaskFreestyleSpec{}
		if err := commonmodels.IToi(task.Spec, jobTaskSpec); err != nil {
			logger.Errorf("set workflowTaskV4 breakpoint failed: IToi %v", err)
			return e.ErrSetBreakpoint.AddDesc("修改断点意外失败")
		}
		pods, err := getter.ListPods(jobTaskSpec.Properties.Namespace, labels.Set{"job-name": task.K8sJobName}.AsSelector(), krkubeclient.Client())
		if err != nil {
			logger.Errorf("set workflowTaskV4 breakpoint failed: list pods %v", err)
			return e.ErrSetBreakpoint.AddDesc("修改断点意外失败: ListPods")
		}
		if len(pods) == 0 {
			logger.Error("set workflowTaskV4 breakpoint failed: list pods num 0")
			return e.ErrSetBreakpoint.AddDesc("修改断点意外失败: ListPods num 0")
		}
		pod := pods[0]
		switch pod.Status.Phase {
		case corev1.PodRunning:
		default:
			logger.Errorf("set workflowTaskV4 breakpoint failed: pod status is %s", pod.Status.Phase)
			return e.ErrSetBreakpoint.AddDesc(fmt.Sprintf("当前任务状态 %s 无法修改断点", pod.Status.Phase))
		}
		exec := func(cmd string) bool {
			opt := podexec.ExecOptions{
				Namespace:     jobTaskSpec.Properties.Namespace,
				PodName:       pod.Name,
				ContainerName: pod.Spec.Containers[0].Name,
				Command:       []string{"sh", "-c", cmd},
			}
			_, stderr, success, _ := podexec.KubeExec(krkubeclient.Clientset(), krkubeclient.RESTConfig(), opt)
			logger.Errorf("set workflowTaskV4 breakpoint exec %s error: %s", cmd, stderr)
			return success
		}
		touchOrRemove := func(set bool) string {
			if set {
				return "touch"
			}
			return "rm"
		}
		switch position {
		case "before":
			if exec(checkShellStepStart) {
				logger.Error("set workflowTaskV4 before breakpoint failed: shell step has started")
				return e.ErrSetBreakpoint.AddDesc("当前任务已开始运行脚本，无法修改前断点")
			}
			exec(fmt.Sprintf(setOrUnsetBreakpoint, touchOrRemove(set), position))
		case "after":
			if exec(checkShellStepDone) {
				logger.Error("set workflowTaskV4 after breakpoint failed: shell step has been done")
				return e.ErrSetBreakpoint.AddDesc("当前任务已运行完脚本，无法修改后断点")
			}
			exec(fmt.Sprintf(setOrUnsetBreakpoint, touchOrRemove(set), position))
		}
		// update data in memory and ack
		switch position {
		case "before":
			task.BreakpointBefore = set
			ack = w.Ack
		case "after":
			task.BreakpointAfter = set
			ack = w.Ack
		}
		logger.Infof("set workflowTaskV4 breakpoint success: %s-%s %v", jobName, position, set)
		return nil
	}
	logger.Errorf("set workflowTaskV4 breakpoint failed: job status is %s", task.Status)
	return e.ErrSetBreakpoint.AddDesc("当前任务状态无法修改断点 ")
}

func EnableDebugWorkflowTaskV4(workflowName string, taskID int64, logger *zap.SugaredLogger) error {
	w := workflowcontroller.GetWorkflowTaskInMap(workflowName, taskID)
	if w == nil {
		logger.Error("set workflowTaskV4 breakpoint failed: not found task")
		return e.ErrStopDebugShell.AddDesc("工作流任务已完成或不存在")
	}
	w.Lock()
	var ack func()
	defer func() {
		if ack != nil {
			ack()
		}
		w.Unlock()
	}()
	t := w.WorkflowTask
	if t.IsDebug {
		return e.ErrStopDebugShell.AddDesc("任务已开启调试模式")
	}
	t.IsDebug = true
	ack = w.Ack
	logger.Infof("enable workflowTaskV4 debug mode success: %s-%d", workflowName, taskID)
	return nil
}

func StopDebugWorkflowTaskJobV4(workflowName, jobName string, taskID int64, position string, logger *zap.SugaredLogger) error {
	w := workflowcontroller.GetWorkflowTaskInMap(workflowName, taskID)
	if w == nil {
		logger.Error("stop debug workflowTaskV4 job failed: not found task")
		return e.ErrStopDebugShell.AddDesc("工作流任务已完成或不存在")
	}
	w.Lock()
	var ack func()
	defer func() {
		if ack != nil {
			ack()
		}
		w.Unlock()
	}()

	var task *commonmodels.JobTask
FOR:
	for _, stage := range w.WorkflowTask.Stages {
		for _, jobTask := range stage.Jobs {
			if jobTask.Name == jobName {
				task = jobTask
				break FOR
			}
		}
	}
	if task == nil {
		logger.Error("stop workflowTaskV4 debug shell failed: not found job")
		return e.ErrStopDebugShell.AddDesc("Job不存在")
	}
	jobTaskSpec := &commonmodels.JobTaskFreestyleSpec{}
	if err := commonmodels.IToi(task.Spec, jobTaskSpec); err != nil {
		logger.Errorf("stop workflowTaskV4 debug shell failed: IToi %v", err)
		return e.ErrStopDebugShell.AddDesc("结束调试意外失败")
	}
	pods, err := getter.ListPods(jobTaskSpec.Properties.Namespace, labels.Set{"job-name": task.K8sJobName}.AsSelector(), krkubeclient.Client())
	if err != nil {
		logger.Errorf("stop workflowTaskV4 debug shell failed: list pods %v", err)
		return e.ErrStopDebugShell.AddDesc("结束调试意外失败: ListPods")
	}
	if len(pods) == 0 {
		logger.Error("stop workflowTaskV4 debug shell failed: list pods num 0")
		return e.ErrStopDebugShell.AddDesc("结束调试意外失败: ListPods num 0")
	}
	pod := pods[0]
	switch pod.Status.Phase {
	case corev1.PodRunning:
	default:
		logger.Errorf("stop workflowTaskV4 debug shell failed: pod status is %s", pod.Status.Phase)
		return e.ErrStopDebugShell.AddDesc(fmt.Sprintf("Job 状态 %s 无法结束调试", pod.Status.Phase))
	}
	exec := func(cmd string) bool {
		opt := podexec.ExecOptions{
			Namespace:     jobTaskSpec.Properties.Namespace,
			PodName:       pod.Name,
			ContainerName: pod.Spec.Containers[0].Name,
			Command:       []string{"sh", "-c", cmd},
		}
		_, stderr, success, _ := podexec.KubeExec(krkubeclient.Clientset(), krkubeclient.RESTConfig(), opt)
		logger.Errorf("stop workflowTaskV4 debug shell exec %s error: %s", cmd, stderr)
		return success
	}

	if !exec(fmt.Sprintf("ls /zadig/debug/breakpoint_%s", position)) {
		logger.Errorf("set workflowTaskV4 %s breakpoint failed: not found file", position)
		return e.ErrStopDebugShell.AddDesc("未找到断点文件")
	}
	exec(fmt.Sprintf("rm /zadig/debug/breakpoint_%s", position))

	ack = w.Ack
	logger.Infof("stop workflowTaskV4 debug shell success: %s-%d", workflowName, taskID)
	return nil
}

func UpdateWorkflowTaskV4(id string, workflowTask *commonmodels.WorkflowTask, logger *zap.SugaredLogger) error {
	err := commonrepo.NewworkflowTaskv4Coll().Update(
		id,
		workflowTask,
	)
	if err != nil {
		logger.Errorf("update workflowTaskV4 error: %s", err)
		return e.ErrCreateTask.AddErr(err)
	}
	return nil
}

func ListWorkflowTaskV4(workflowName string, pageNum, pageSize int64, logger *zap.SugaredLogger) ([]*commonmodels.WorkflowTask, int64, error) {
	resp, total, err := commonrepo.NewworkflowTaskv4Coll().List(&commonrepo.ListWorkflowTaskV4Option{WorkflowName: workflowName, Limit: int(pageSize), Skip: int((pageNum - 1) * pageSize)})
	if err != nil {
		logger.Errorf("list workflowTaskV4 error: %s", err)
		return resp, total, err
	}
	cleanWorkflowV4Tasks(resp)
	return resp, total, nil
}

func getLatestWorkflowTaskV4(workflowName string) (*commonmodels.WorkflowTask, error) {
	resp, err := commonrepo.NewworkflowTaskv4Coll().GetLatest(workflowName)
	if err != nil {
		return nil, err
	}
	resp.WorkflowArgs = nil
	resp.OriginWorkflowArgs = nil
	resp.Stages = nil
	return resp, nil
}

// clean extra message for list workflow
func cleanWorkflowV4Tasks(workflows []*commonmodels.WorkflowTask) {
	const StatusNotRun = ""
	for _, workflow := range workflows {
		var stageList []*commonmodels.StageTask
		workflow.WorkflowArgs = nil
		workflow.OriginWorkflowArgs = nil
		for _, stage := range workflow.Stages {
			if stage.Approval != nil && stage.Approval.Enabled {
				approvalStage := &commonmodels.StageTask{
					Name:      "人工审批",
					StartTime: stage.Approval.StartTime,
					EndTime:   stage.Approval.EndTime,
				}
				switch {
				//case stage.Status == config.StatusWaitingApprove:
				//	approvalStage.Status = config.StatusWaitingApprove
				//	stage.Status = StatusNotRun
				case stage.Status == config.StatusPassed || stage.Status == config.StatusRunning:
					approvalStage.Status = config.StatusPassed
				default:
					approvalStage.Status = stage.Status
					stage.Status = StatusNotRun
				}
				stageList = append(stageList, approvalStage)
			}
			stageList = append(stageList, stage)
			stage.Jobs = nil
		}
		workflow.Stages = stageList
	}
}

func CancelWorkflowTaskV4(userName, workflowName string, taskID int64, logger *zap.SugaredLogger) error {
	if err := workflowcontroller.CancelWorkflowTask(userName, workflowName, taskID, logger); err != nil {
		logger.Errorf("cancel workflowTaskV4 error: %s", err)
		return e.ErrCancelTask.AddErr(err)
	}
	return nil
}

func GetWorkflowTaskV4(workflowName string, taskID int64, logger *zap.SugaredLogger) (*WorkflowTaskPreview, error) {
	task, err := commonrepo.NewworkflowTaskv4Coll().Find(workflowName, taskID)
	if err != nil {
		logger.Errorf("find workflowTaskV4 error: %s", err)
		return nil, err
	}
	resp := &WorkflowTaskPreview{
		TaskID:              task.TaskID,
		WorkflowName:        task.WorkflowName,
		WorkflowDisplayName: task.WorkflowDisplayName,
		ProjectName:         task.ProjectName,
		Status:              task.Status,
		Params:              task.Params,
		TaskCreator:         task.TaskCreator,
		TaskRevoker:         task.TaskRevoker,
		CreateTime:          task.CreateTime,
		StartTime:           task.StartTime,
		EndTime:             task.EndTime,
		Error:               task.Error,
		IsRestart:           task.IsRestart,
		Debug:               task.IsDebug,
	}
	timeNow := time.Now().Unix()
	for _, stage := range task.Stages {
		resp.Stages = append(resp.Stages, &StageTaskPreview{
			Name:      stage.Name,
			Status:    stage.Status,
			StartTime: stage.StartTime,
			EndTime:   stage.EndTime,
			Parallel:  stage.Parallel,
			Approval:  stage.Approval,
			Jobs:      jobsToJobPreviews(stage.Jobs, task.GlobalContext, timeNow),
		})
	}
	return resp, nil
}

func ApproveStage(workflowName, stageName, userName, userID, comment string, taskID int64, approve bool, logger *zap.SugaredLogger) error {
	if workflowName == "" || stageName == "" || taskID == 0 {
		errMsg := fmt.Sprintf("can not find approved workflow: %s, taskID: %d,stage: %s", workflowName, taskID, stageName)
		logger.Error(errMsg)
		return e.ErrApproveTask.AddDesc(errMsg)
	}
	if err := workflowcontroller.ApproveStage(workflowName, stageName, userName, userID, comment, taskID, approve); err != nil {
		logger.Error(err)
		return e.ErrApproveTask.AddErr(err)
	}
	return nil
}

func jobsToJobPreviews(jobs []*commonmodels.JobTask, context map[string]string, now int64) []*JobTaskPreview {
	resp := []*JobTaskPreview{}
	for _, job := range jobs {
		costSeconds := int64(0)
		if job.StartTime != 0 {
			costSeconds = now - job.StartTime
			if job.EndTime != 0 {
				costSeconds = job.EndTime - job.StartTime
			}
		}
		jobPreview := &JobTaskPreview{
			Name:             job.Name,
			Status:           job.Status,
			StartTime:        job.StartTime,
			EndTime:          job.EndTime,
			Error:            job.Error,
			JobType:          job.JobType,
			BreakpointBefore: job.BreakpointBefore,
			BreakpointAfter:  job.BreakpointAfter,
			CostSeconds:      costSeconds,
			JobInfo:          job.JobInfo,
		}
		switch job.JobType {
		case string(config.JobFreestyle):
			fallthrough
		case string(config.JobZadigBuild):
			spec := ZadigBuildJobSpec{}
			taskJobSpec := &commonmodels.JobTaskFreestyleSpec{}
			if err := commonmodels.IToi(job.Spec, taskJobSpec); err != nil {
				continue
			}
			for _, arg := range taskJobSpec.Properties.Envs {
				if arg.Key == "SERVICE" {
					spec.ServiceName = arg.Value
					continue
				}
				if arg.Key == "SERVICE_MODULE" {
					spec.ServiceModule = arg.Value
					continue
				}
			}
			// get image from global context
			imageContextKey := workflowcontroller.GetContextKey(jobspec.GetJobOutputKey(job.Key, "IMAGE"))
			if context != nil {
				spec.Image = context[imageContextKey]
			}

			spec.Envs = taskJobSpec.Properties.CustomEnvs
			for _, step := range taskJobSpec.Steps {
				if step.StepType == config.StepGit {
					stepSpec := &stepspec.StepGitSpec{}
					commonmodels.IToi(step.Spec, &stepSpec)
					spec.Repos = stepSpec.Repos
					continue
				}
			}
			jobPreview.Spec = spec
		case string(config.JobZadigDistributeImage):
			spec := &DistributeImageJobSpec{}
			taskJobSpec := &commonmodels.JobTaskFreestyleSpec{}
			if err := commonmodels.IToi(job.Spec, taskJobSpec); err != nil {
				continue
			}

			for _, step := range taskJobSpec.Steps {
				if step.StepType == config.StepDistributeImage {
					stepSpec := &stepspec.StepImageDistributeSpec{}
					commonmodels.IToi(step.Spec, &stepSpec)
					spec.DistributeTarget = stepSpec.DistributeTarget
					break
				}
			}
			jobPreview.Spec = spec
		case string(config.JobZadigTesting):
			spec := &ZadigTestingJobSpec{}
			jobPreview.Spec = spec
			taskJobSpec := &commonmodels.JobTaskFreestyleSpec{}
			if err := commonmodels.IToi(job.Spec, taskJobSpec); err != nil {
				continue
			}
			spec.Envs = taskJobSpec.Properties.CustomEnvs
			for _, step := range taskJobSpec.Steps {
				if step.StepType == config.StepGit {
					stepSpec := &stepspec.StepGitSpec{}
					commonmodels.IToi(step.Spec, &stepSpec)
					spec.Repos = stepSpec.Repos
					continue
				}
			}
			for _, arg := range taskJobSpec.Properties.Envs {
				if arg.Key == "TESTING_PROJECT" {
					spec.ProjectName = arg.Value
					continue
				}
				if arg.Key == "TESTING_NAME" {
					spec.TestName = arg.Value
					continue
				}
				if arg.Key == "TESTING_TYPE" {
					spec.TestType = arg.Value
					continue
				}
				if arg.Key == "SERVICE" {
					spec.ServiceName = arg.Value
					continue
				}
				if arg.Key == "SERVICE_MODULE" {
					spec.ServiceModule = arg.Value
					continue
				}

			}
			if job.Status == config.StatusPassed || job.Status == config.StatusFailed {
				for _, step := range taskJobSpec.Steps {
					if step.Name == config.TestJobArchiveResultStepName {
						spec.Archive = true
					}
					if step.Name == config.TestJobHTMLReportStepName {
						spec.HtmlReport = true
					}
					if step.Name == config.TestJobJunitReportStepName {
						spec.JunitReport = true
					}
				}
			}
		case string(config.JobZadigScanning):
			spec := ZadigScanningJobSpec{}
			taskJobSpec := &commonmodels.JobTaskFreestyleSpec{}
			if err := commonmodels.IToi(job.Spec, taskJobSpec); err != nil {
				continue
			}
			for _, step := range taskJobSpec.Steps {
				if step.StepType == config.StepGit {
					stepSpec := &stepspec.StepGitSpec{}
					commonmodels.IToi(step.Spec, &stepSpec)
					spec.Repos = stepSpec.Repos
					continue
				}
			}
			for _, arg := range taskJobSpec.Properties.Envs {
				if arg.Key == "SONAR_LINK" {
					spec.LinkURL = arg.Value
					continue
				}
				if arg.Key == "SCANNING_NAME" {
					spec.ScanningName = arg.Value
					continue
				}
			}
			jobPreview.Spec = spec
		case string(config.JobZadigDeploy):
			spec := ZadigDeployJobSpec{}
			taskJobSpec := &commonmodels.JobTaskDeploySpec{}
			if err := commonmodels.IToi(job.Spec, taskJobSpec); err != nil {
				continue
			}
			spec.Env = taskJobSpec.Env
			spec.KeyVals = taskJobSpec.KeyVals
			spec.YamlContent = taskJobSpec.YamlContent
			spec.SkipCheckRunStatus = taskJobSpec.SkipCheckRunStatus
			// for compatibility
			if taskJobSpec.ServiceModule != "" {
				spec.ServiceAndImages = append(spec.ServiceAndImages, &ServiceAndImage{
					ServiceName:   taskJobSpec.ServiceName,
					ServiceModule: taskJobSpec.ServiceModule,
					Image:         taskJobSpec.Image,
				})
			}

			for _, imageAndmodule := range taskJobSpec.ServiceAndImages {
				spec.ServiceAndImages = append(spec.ServiceAndImages, &ServiceAndImage{
					ServiceName:   taskJobSpec.ServiceName,
					ServiceModule: imageAndmodule.ServiceModule,
					Image:         imageAndmodule.Image,
				})
			}
			jobPreview.Spec = spec
		case string(config.JobZadigHelmDeploy):
			jobPreview.JobType = string(config.JobZadigDeploy)
			spec := ZadigDeployJobSpec{}
			job.JobType = string(config.JobZadigDeploy)
			taskJobSpec := &commonmodels.JobTaskHelmDeploySpec{}
			if err := commonmodels.IToi(job.Spec, taskJobSpec); err != nil {
				continue
			}
			spec.Env = taskJobSpec.Env
			spec.SkipCheckRunStatus = taskJobSpec.SkipCheckRunStatus
			for _, imageAndmodule := range taskJobSpec.ImageAndModules {
				spec.ServiceAndImages = append(spec.ServiceAndImages, &ServiceAndImage{
					ServiceName:   taskJobSpec.ServiceName,
					ServiceModule: imageAndmodule.ServiceModule,
					Image:         imageAndmodule.Image,
				})
			}
			jobPreview.Spec = spec
		case string(config.JobPlugin):
			taskJobSpec := &commonmodels.JobTaskPluginSpec{}
			if err := commonmodels.IToi(job.Spec, taskJobSpec); err != nil {
				continue
			}
			jobPreview.Spec = taskJobSpec.Plugin
		case string(config.JobCustomDeploy):
			spec := CustomDeployJobSpec{}
			taskJobSpec := &commonmodels.JobTaskCustomDeploySpec{}
			if err := commonmodels.IToi(job.Spec, taskJobSpec); err != nil {
				continue
			}
			spec.Image = taskJobSpec.Image
			spec.Namespace = taskJobSpec.Namespace
			spec.SkipCheckRunStatus = taskJobSpec.SkipCheckRunStatus
			spec.Target = strings.Join([]string{taskJobSpec.WorkloadType, taskJobSpec.WorkloadName, taskJobSpec.ContainerName}, "/")
			cluster, err := commonrepo.NewK8SClusterColl().Get(taskJobSpec.ClusterID)
			if err != nil {
				log.Errorf("cluster id: %s not found", taskJobSpec.ClusterID)
			} else {
				spec.ClusterName = cluster.Name
			}
			jobPreview.Spec = spec
		case string(config.JobK8sCanaryDeploy):
			taskJobSpec := &commonmodels.JobTaskCanaryDeploySpec{}
			if err := commonmodels.IToi(job.Spec, taskJobSpec); err != nil {
				continue
			}
			sepc := K8sCanaryDeployJobSpec{
				Image:          taskJobSpec.Image,
				K8sServiceName: taskJobSpec.K8sServiceName,
				Namespace:      taskJobSpec.Namespace,
				ContainerName:  taskJobSpec.ContainerName,
				CanaryReplica:  taskJobSpec.CanaryReplica,
				Events:         taskJobSpec.Events,
			}
			cluster, err := commonrepo.NewK8SClusterColl().Get(taskJobSpec.ClusterID)
			if err != nil {
				log.Errorf("cluster id: %s not found", taskJobSpec.ClusterID)
			} else {
				sepc.ClusterName = cluster.Name
			}
			jobPreview.Spec = sepc
		case string(config.JobK8sCanaryRelease):
			taskJobSpec := &commonmodels.JobTaskCanaryReleaseSpec{}
			if err := commonmodels.IToi(job.Spec, taskJobSpec); err != nil {
				continue
			}
			sepc := K8sCanaryReleaseJobSpec{
				Image:          taskJobSpec.Image,
				K8sServiceName: taskJobSpec.K8sServiceName,
				Namespace:      taskJobSpec.Namespace,
				ContainerName:  taskJobSpec.ContainerName,
				Events:         taskJobSpec.Events,
			}
			cluster, err := commonrepo.NewK8SClusterColl().Get(taskJobSpec.ClusterID)
			if err != nil {
				log.Errorf("cluster id: %s not found", taskJobSpec.ClusterID)
			} else {
				sepc.ClusterName = cluster.Name
			}
			jobPreview.Spec = sepc
		case string(config.JobK8sBlueGreenDeploy):
			taskJobSpec := &commonmodels.JobTaskBlueGreenDeploySpec{}
			if err := commonmodels.IToi(job.Spec, taskJobSpec); err != nil {
				continue
			}
			sepc := K8sBlueGreenDeployJobSpec{
				Image:          taskJobSpec.Image,
				K8sServiceName: taskJobSpec.K8sServiceName,
				Namespace:      taskJobSpec.Namespace,
				ContainerName:  taskJobSpec.ContainerName,
				Events:         taskJobSpec.Events,
			}
			cluster, err := commonrepo.NewK8SClusterColl().Get(taskJobSpec.ClusterID)
			if err != nil {
				log.Errorf("cluster id: %s not found", taskJobSpec.ClusterID)
			} else {
				sepc.ClusterName = cluster.Name
			}
			jobPreview.Spec = sepc
		case string(config.JobK8sBlueGreenRelease):
			taskJobSpec := &commonmodels.JobTaskBlueGreenReleaseSpec{}
			if err := commonmodels.IToi(job.Spec, taskJobSpec); err != nil {
				continue
			}
			sepc := K8sBlueGreenReleaseJobSpec{
				Image:          taskJobSpec.Image,
				K8sServiceName: taskJobSpec.K8sServiceName,
				Namespace:      taskJobSpec.Namespace,
				ContainerName:  taskJobSpec.ContainerName,
				Events:         taskJobSpec.Events,
			}
			cluster, err := commonrepo.NewK8SClusterColl().Get(taskJobSpec.ClusterID)
			if err != nil {
				log.Errorf("cluster id: %s not found", taskJobSpec.ClusterID)
			} else {
				sepc.ClusterName = cluster.Name
			}
			jobPreview.Spec = sepc
		default:
			jobPreview.Spec = job.Spec
		}
		resp = append(resp, jobPreview)
	}
	return resp
}

func setZadigParamRepos(workflow *commonmodels.WorkflowV4, logger *zap.SugaredLogger) {
	for _, param := range workflow.Params {
		if param.ParamsType != "repo" {
			continue
		}
		setBuildInfo(param.Repo, []*types.Repository{param.Repo}, logger)
	}
}

func setZadigBuildRepos(job *commonmodels.Job, logger *zap.SugaredLogger) error {
	spec := &commonmodels.ZadigBuildJobSpec{}
	if err := commonmodels.IToi(job.Spec, spec); err != nil {
		return err
	}
	for _, build := range spec.ServiceAndBuilds {
		if err := setManunalBuilds(build.Repos, build.Repos, logger); err != nil {
			return err
		}
	}
	job.Spec = spec
	return nil
}

func setZadigTestingRepos(job *commonmodels.Job, logger *zap.SugaredLogger) error {
	spec := &commonmodels.ZadigTestingJobSpec{}
	if err := commonmodels.IToi(job.Spec, spec); err != nil {
		return err
	}
	for _, build := range spec.TestModules {
		if err := setManunalBuilds(build.Repos, build.Repos, logger); err != nil {
			return err
		}
	}
	job.Spec = spec
	return nil
}

func setZadigScanningRepos(job *commonmodels.Job, logger *zap.SugaredLogger) error {
	spec := &commonmodels.ZadigScanningJobSpec{}
	if err := commonmodels.IToi(job.Spec, spec); err != nil {
		return err
	}
	for _, build := range spec.Scannings {
		if err := setManunalBuilds(build.Repos, build.Repos, logger); err != nil {
			return err
		}
	}
	job.Spec = spec
	return nil
}

func setFreeStyleRepos(job *commonmodels.Job, logger *zap.SugaredLogger) error {
	spec := &commonmodels.FreestyleJobSpec{}
	if err := commonmodels.IToi(job.Spec, spec); err != nil {
		return err
	}
	for _, step := range spec.Steps {
		if step.StepType != config.StepGit {
			continue
		}
		stepSpec := &stepspec.StepGitSpec{}
		if err := commonmodels.IToi(step.Spec, stepSpec); err != nil {
			return err
		}
		if err := setManunalBuilds(stepSpec.Repos, stepSpec.Repos, logger); err != nil {
			return err
		}
		step.Spec = stepSpec
	}
	job.Spec = spec
	return nil
}

func workflowTaskLint(workflowTask *commonmodels.WorkflowTask, logger *zap.SugaredLogger) error {
	if len(workflowTask.Stages) <= 0 {
		errMsg := fmt.Sprintf("no stage found in workflow task: %s,taskID: %d", workflowTask.WorkflowName, workflowTask.TaskID)
		logger.Error(errMsg)
		return e.ErrCreateTask.AddDesc(errMsg)
	}
	for _, stage := range workflowTask.Stages {
		if len(stage.Jobs) <= 0 {
			errMsg := fmt.Sprintf("no job found in workflow task: %s,taskID: %d,stage: %s", workflowTask.WorkflowName, workflowTask.TaskID, stage.Name)
			logger.Error(errMsg)
			return e.ErrCreateTask.AddDesc(errMsg)
		}
	}
	return nil
}

func GetWorkflowV4ArtifactFileContent(workflowName, jobName string, taskID int64, log *zap.SugaredLogger) ([]byte, error) {
	workflowTask, err := commonrepo.NewworkflowTaskv4Coll().Find(workflowName, taskID)
	if err != nil {
		return []byte{}, fmt.Errorf("cannot find workflow task, workflow name: %s, task id: %d", workflowName, taskID)
	}
	var jobTask *commonmodels.JobTask
	for _, stage := range workflowTask.Stages {
		for _, job := range stage.Jobs {
			if job.Name != jobName {
				continue
			}
			if job.JobType != string(config.JobZadigTesting) {
				return []byte{}, fmt.Errorf("job: %s was not a testing job", jobName)
			}
			jobTask = job
		}
	}
	if jobTask == nil {
		return []byte{}, fmt.Errorf("cannot find job task, workflow name: %s, task id: %d, job name: %s", workflowName, taskID, jobName)
	}
	jobSpec := &commonmodels.JobTaskFreestyleSpec{}
	if err := commonmodels.IToi(jobTask.Spec, jobSpec); err != nil {
		return []byte{}, fmt.Errorf("unmashal job spec error: %v", err)
	}

	var stepTask *commonmodels.StepTask
	for _, step := range jobSpec.Steps {
		if step.Name != config.TestJobArchiveResultStepName {
			continue
		}
		if step.StepType != config.StepTarArchive {
			return []byte{}, fmt.Errorf("step: %s was not a junit report step", step.Name)
		}
		stepTask = step
	}
	if stepTask == nil {
		return []byte{}, fmt.Errorf("cannot find step task, workflow name: %s, task id: %d, job name: %s", workflowName, taskID, jobName)
	}
	stepSpec := &step.StepTarArchiveSpec{}
	if err := commonmodels.IToi(stepTask.Spec, stepSpec); err != nil {
		return []byte{}, fmt.Errorf("unmashal step spec error: %v", err)
	}

	storage, err := s3.FindDefaultS3()
	if err != nil {
		log.Errorf("GetTestArtifactInfo FindDefaultS3 err:%v", err)
		return []byte{}, fmt.Errorf("findDefaultS3 err: %v", err)
	}
	forcedPathStyle := true
	if storage.Provider == setting.ProviderSourceAli {
		forcedPathStyle = false
	}
	client, err := s3tool.NewClient(storage.Endpoint, storage.Ak, storage.Sk, storage.Region, storage.Insecure, forcedPathStyle)
	if err != nil {
		log.Errorf("GetTestArtifactInfo Create S3 client err:%+v", err)
		return []byte{}, fmt.Errorf("create S3 client err: %v", err)
	}
	objectKey := filepath.Join(stepSpec.S3DestDir, stepSpec.FileName)
	object, err := client.GetFile(storage.Bucket, objectKey, &s3tool.DownloadOption{RetryNum: 2})
	if err != nil {
		log.Errorf("GetTestArtifactInfo GetFile err:%s", err)
		return []byte{}, fmt.Errorf("GetFile err: %v", err)
	}
	fileByts, err := ioutil.ReadAll(object.Body)
	if err != nil {
		log.Errorf("GetTestArtifactInfo ioutil.ReadAll err:%s", err)
		return []byte{}, fmt.Errorf("ioutil.ReadAll err: %v", err)
	}
	return fileByts, nil
}
