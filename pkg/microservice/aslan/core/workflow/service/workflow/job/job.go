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

package job

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	commonmodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	commonrepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	"github.com/koderover/zadig/pkg/setting"
	"github.com/koderover/zadig/pkg/types"
	"github.com/koderover/zadig/pkg/types/job"
)

const (
	OutputNameRegexString = "^[a-zA-Z0-9_]{1,64}$"
	JobNameKey            = "job_name"
)

var (
	OutputNameRegex = regexp.MustCompile(OutputNameRegexString)
)

type JobCtl interface {
	Instantiate() error
	SetPreset() error
	ToJobs(taskID int64) ([]*commonmodels.JobTask, error)
	MergeArgs(args *commonmodels.Job) error
	LintJob() error
}

func InitJobCtl(job *commonmodels.Job, workflow *commonmodels.WorkflowV4) (JobCtl, error) {
	var resp JobCtl
	switch job.JobType {
	case config.JobZadigBuild:
		resp = &BuildJob{job: job, workflow: workflow}
	case config.JobZadigDeploy:
		resp = &DeployJob{job: job, workflow: workflow}
	case config.JobPlugin:
		resp = &PluginJob{job: job, workflow: workflow}
	case config.JobFreestyle:
		resp = &FreeStyleJob{job: job, workflow: workflow}
	case config.JobCustomDeploy:
		resp = &CustomDeployJob{job: job, workflow: workflow}
	case config.JobK8sBlueGreenDeploy:
		resp = &BlueGreenDeployJob{job: job, workflow: workflow}
	case config.JobK8sBlueGreenRelease:
		resp = &BlueGreenReleaseJob{job: job, workflow: workflow}
	case config.JobK8sCanaryDeploy:
		resp = &CanaryDeployJob{job: job, workflow: workflow}
	case config.JobK8sCanaryRelease:
		resp = &CanaryReleaseJob{job: job, workflow: workflow}
	case config.JobZadigTesting:
		resp = &TestingJob{job: job, workflow: workflow}
	case config.JobK8sGrayRelease:
		resp = &GrayReleaseJob{job: job, workflow: workflow}
	case config.JobK8sGrayRollback:
		resp = &GrayRollbackJob{job: job, workflow: workflow}
	case config.JobK8sPatch:
		resp = &K8sPacthJob{job: job, workflow: workflow}
	case config.JobZadigScanning:
		resp = &ScanningJob{job: job, workflow: workflow}
	case config.JobZadigDistributeImage:
		resp = &ImageDistributeJob{job: job, workflow: workflow}
	case config.JobIstioRelease:
		resp = &IstioReleaseJob{job: job, workflow: workflow}
	case config.JobIstioRollback:
		resp = &IstioRollBackJob{job: job, workflow: workflow}
	case config.JobJira:
		resp = &JiraJob{job: job, workflow: workflow}
	case config.JobNacos:
		resp = &NacosJob{job: job, workflow: workflow}
	case config.JobApollo:
		resp = &ApolloJob{job: job, workflow: workflow}
	case config.JobMeegoTransition:
		resp = &MeegoTransitionJob{job: job, workflow: workflow}
	case config.JobWorkflowTrigger:
		resp = &WorkflowTriggerJob{job: job, workflow: workflow}
	default:
		return resp, fmt.Errorf("job type not found %s", job.JobType)
	}
	return resp, nil
}

func InstantiateWorkflow(workflow *commonmodels.WorkflowV4) error {
	for _, stage := range workflow.Stages {
		for _, job := range stage.Jobs {
			if err := Instantiate(job, workflow); err != nil {
				return err
			}
		}
	}
	return nil
}

func Instantiate(job *commonmodels.Job, workflow *commonmodels.WorkflowV4) error {
	ctl, err := InitJobCtl(job, workflow)
	if err != nil {
		return warpJobError(job.Name, err)
	}
	return ctl.Instantiate()
}

func SetPreset(job *commonmodels.Job, workflow *commonmodels.WorkflowV4) error {
	jobCtl, err := InitJobCtl(job, workflow)
	if err != nil {
		return warpJobError(job.Name, err)
	}
	JobPresetSkiped(job)
	return jobCtl.SetPreset()
}

func JobPresetSkiped(job *commonmodels.Job) {
	if job.RunPolicy == config.ForceRun {
		job.Skipped = false
		return
	}
	if job.RunPolicy == config.DefaultNotRun {
		job.Skipped = true
		return
	}
	job.Skipped = false
}

func ToJobs(job *commonmodels.Job, workflow *commonmodels.WorkflowV4, taskID int64) ([]*commonmodels.JobTask, error) {
	jobCtl, err := InitJobCtl(job, workflow)
	if err != nil {
		return []*commonmodels.JobTask{}, warpJobError(job.Name, err)
	}
	return jobCtl.ToJobs(taskID)
}

func LintJob(job *commonmodels.Job, workflow *commonmodels.WorkflowV4) error {
	jobCtl, err := InitJobCtl(job, workflow)
	if err != nil {
		return warpJobError(job.Name, err)
	}
	return jobCtl.LintJob()
}

func MergeWebhookRepo(workflow *commonmodels.WorkflowV4, repo *types.Repository) error {
	for _, stage := range workflow.Stages {
		for _, job := range stage.Jobs {
			if job.JobType == config.JobZadigBuild {
				jobCtl := &BuildJob{job: job, workflow: workflow}
				if err := jobCtl.MergeWebhookRepo(repo); err != nil {
					return warpJobError(job.Name, err)
				}
			}
			if job.JobType == config.JobFreestyle {
				jobCtl := &FreeStyleJob{job: job, workflow: workflow}
				if err := jobCtl.MergeWebhookRepo(repo); err != nil {
					return warpJobError(job.Name, err)
				}
			}
			if job.JobType == config.JobZadigTesting {
				jobCtl := &TestingJob{job: job, workflow: workflow}
				if err := jobCtl.MergeWebhookRepo(repo); err != nil {
					return warpJobError(job.Name, err)
				}
			}
			if job.JobType == config.JobZadigScanning {
				jobCtl := &ScanningJob{job: job, workflow: workflow}
				if err := jobCtl.MergeWebhookRepo(repo); err != nil {
					return warpJobError(job.Name, err)
				}
			}
		}
	}
	return nil
}

func GetWorkflowOutputs(workflow *commonmodels.WorkflowV4, currentJobName string, log *zap.SugaredLogger) []string {
	resp := []string{}
	jobRankMap := getJobRankMap(workflow.Stages)
	for _, stage := range workflow.Stages {
		for _, job := range stage.Jobs {
			// we only need to get the outputs from job runs before the current job
			if jobRankMap[job.Name] >= jobRankMap[currentJobName] {
				return resp
			}
			if job.JobType == config.JobZadigBuild {
				jobCtl := &BuildJob{job: job, workflow: workflow}
				resp = append(resp, jobCtl.GetOutPuts(log)...)
			}
			if job.JobType == config.JobFreestyle {
				jobCtl := &FreeStyleJob{job: job, workflow: workflow}
				resp = append(resp, jobCtl.GetOutPuts(log)...)
			}
			if job.JobType == config.JobZadigTesting {
				jobCtl := &TestingJob{job: job, workflow: workflow}
				resp = append(resp, jobCtl.GetOutPuts(log)...)
			}
			if job.JobType == config.JobZadigScanning {
				jobCtl := &ScanningJob{job: job, workflow: workflow}
				resp = append(resp, jobCtl.GetOutPuts(log)...)
			}
			if job.JobType == config.JobZadigDistributeImage {
				jobCtl := &ImageDistributeJob{job: job, workflow: workflow}
				resp = append(resp, jobCtl.GetOutPuts(log)...)
			}
			if job.JobType == config.JobPlugin {
				jobCtl := &PluginJob{job: job, workflow: workflow}
				resp = append(resp, jobCtl.GetOutPuts(log)...)
			}
		}
	}
	return resp
}

type RepoIndex struct {
	JobName       string `json:"job_name"`
	ServiceName   string `json:"service_name"`
	ServiceModule string `json:"service_module"`
	RepoIndex     int    `json:"repo_index"`
}

func GetWorkflowRepoIndex(workflow *commonmodels.WorkflowV4, currentJobName string, log *zap.SugaredLogger) []*RepoIndex {
	resp := []*RepoIndex{}
	jobRankMap := getJobRankMap(workflow.Stages)
	for _, stage := range workflow.Stages {
		for _, job := range stage.Jobs {
			// we only need to get the outputs from job runs before the current job
			if jobRankMap[job.Name] >= jobRankMap[currentJobName] {
				return resp
			}
			if job.JobType == config.JobZadigBuild {
				jobSpec := &commonmodels.ZadigBuildJobSpec{}
				if err := commonmodels.IToiYaml(job.Spec, jobSpec); err != nil {
					log.Errorf("get job spec failed, err: %v", err)
					continue
				}
				for _, build := range jobSpec.ServiceAndBuilds {
					buildInfo, err := commonrepo.NewBuildColl().Find(&commonrepo.BuildFindOption{Name: build.BuildName})
					if err != nil {
						log.Errorf("find build: %s error: %v", build.BuildName, err)
						continue
					}
					if err := fillBuildDetail(buildInfo, build.ServiceName, build.ServiceModule); err != nil {
						log.Errorf("fill build: %s detail error: %v", build.BuildName, err)
						continue
					}
					for _, target := range buildInfo.Targets {
						if target.ServiceName == build.ServiceName && target.ServiceModule == build.ServiceModule {
							repos := mergeRepos(buildInfo.Repos, build.Repos)
							for index := range repos {
								resp = append(resp, &RepoIndex{
									JobName:       job.Name,
									ServiceName:   build.ServiceName,
									ServiceModule: build.ServiceModule,
									RepoIndex:     index,
								})
							}
							break
						}
					}
				}
			}
		}
	}
	return resp
}

func GetRepos(workflow *commonmodels.WorkflowV4) ([]*types.Repository, error) {
	repos := []*types.Repository{}
	for _, stage := range workflow.Stages {
		for _, job := range stage.Jobs {
			if job.JobType == config.JobZadigBuild {
				jobCtl := &BuildJob{job: job, workflow: workflow}
				buildRepos, err := jobCtl.GetRepos()
				if err != nil {
					return repos, warpJobError(job.Name, err)
				}
				repos = append(repos, buildRepos...)
			}
			if job.JobType == config.JobFreestyle {
				jobCtl := &FreeStyleJob{job: job, workflow: workflow}
				freeStyleRepos, err := jobCtl.GetRepos()
				if err != nil {
					return repos, warpJobError(job.Name, err)
				}
				repos = append(repos, freeStyleRepos...)
			}
			if job.JobType == config.JobZadigTesting {
				jobCtl := &TestingJob{job: job, workflow: workflow}
				testingRepos, err := jobCtl.GetRepos()
				if err != nil {
					return repos, warpJobError(job.Name, err)
				}
				repos = append(repos, testingRepos...)
			}
			if job.JobType == config.JobZadigScanning {
				jobCtl := &ScanningJob{job: job, workflow: workflow}
				scanningRepos, err := jobCtl.GetRepos()
				if err != nil {
					return repos, warpJobError(job.Name, err)
				}
				repos = append(repos, scanningRepos...)
			}
		}
	}
	newRepos := []*types.Repository{}
	for _, repo := range repos {
		if repo.SourceFrom != types.RepoSourceRuntime {
			continue
		}
		newRepos = append(newRepos, repo)
	}
	return newRepos, nil
}

func MergeArgs(workflow, workflowArgs *commonmodels.WorkflowV4) error {
	argsMap := make(map[string]*commonmodels.Job)
	if workflowArgs != nil {
		for _, stage := range workflowArgs.Stages {
			for _, job := range stage.Jobs {
				jobKey := strings.Join([]string{job.Name, string(job.JobType)}, "-")
				argsMap[jobKey] = job
			}
		}
		workflow.Params = renderParams(workflowArgs.Params, workflow.Params)
	}
	for _, stage := range workflow.Stages {
		for _, job := range stage.Jobs {
			if err := SetPreset(job, workflow); err != nil {
				return warpJobError(job.Name, err)
			}
			jobKey := strings.Join([]string{job.Name, string(job.JobType)}, "-")
			if jobArgs, ok := argsMap[jobKey]; ok {
				job.Skipped = JobSkiped(jobArgs)
				jobCtl, err := InitJobCtl(job, workflow)
				if err != nil {
					return warpJobError(job.Name, err)
				}
				if err := jobCtl.MergeArgs(jobArgs); err != nil {
					return warpJobError(job.Name, err)
				}
				continue
			}
		}
	}
	return nil
}

func JobSkiped(job *commonmodels.Job) bool {
	if job.RunPolicy == config.ForceRun {
		return false
	}
	return job.Skipped
}

// use service name and service module hash to generate job name
func jobNameFormat(jobName string) string {
	if len(jobName) > 63 {
		jobName = jobName[:63]
	}
	jobName = strings.Trim(jobName, "-")
	jobName = strings.ToLower(jobName)
	return jobName
}

func getReposVariables(repos []*types.Repository) []*commonmodels.KeyVal {
	ret := make([]*commonmodels.KeyVal, 0)
	for index, repo := range repos {

		repoNameIndex := fmt.Sprintf("REPONAME_%d", index)
		ret = append(ret, &commonmodels.KeyVal{Key: fmt.Sprintf(repoNameIndex), Value: repo.RepoName, IsCredential: false})

		repoName := strings.Replace(repo.RepoName, "-", "_", -1)
		repoName = strings.Replace(repoName, ".", "_", -1)

		repoIndex := fmt.Sprintf("REPO_%d", index)
		ret = append(ret, &commonmodels.KeyVal{Key: fmt.Sprintf(repoIndex), Value: repoName, IsCredential: false})

		if len(repo.Branch) > 0 {
			ret = append(ret, &commonmodels.KeyVal{Key: fmt.Sprintf("%s_BRANCH", repoName), Value: repo.Branch, IsCredential: false})
		}

		if len(repo.Tag) > 0 {
			ret = append(ret, &commonmodels.KeyVal{Key: fmt.Sprintf("%s_TAG", repoName), Value: repo.Tag, IsCredential: false})
		}

		if repo.PR > 0 {
			ret = append(ret, &commonmodels.KeyVal{Key: fmt.Sprintf("%s_PR", repoName), Value: strconv.Itoa(repo.PR), IsCredential: false})
		}

		if len(repo.PRs) > 0 {
			prStrs := []string{}
			for _, pr := range repo.PRs {
				prStrs = append(prStrs, strconv.Itoa(pr))
			}
			ret = append(ret, &commonmodels.KeyVal{Key: fmt.Sprintf("%s_PR", repoName), Value: strings.Join(prStrs, ","), IsCredential: false})
		}

		if len(repo.CommitID) > 0 {
			ret = append(ret, &commonmodels.KeyVal{Key: fmt.Sprintf("%s_COMMIT_ID", repoName), Value: repo.CommitID, IsCredential: false})
		}
		ret = append(ret, getEnvFromCommitMsg(repo.CommitMessage)...)
	}
	return ret
}

// before workflowflow task was created, we need to remove the fixed mark from variables.
func RemoveFixedValueMarks(workflow *commonmodels.WorkflowV4) error {
	bf := bytes.NewBuffer([]byte{})
	jsonEncoder := json.NewEncoder(bf)
	jsonEncoder.SetEscapeHTML(false)
	jsonEncoder.Encode(workflow)
	replacedString := strings.ReplaceAll(bf.String(), setting.FixedValueMark, "")
	return json.Unmarshal([]byte(replacedString), &workflow)
}

func RenderGlobalVariables(workflow *commonmodels.WorkflowV4, taskID int64, creator string) error {
	b, err := json.Marshal(workflow)
	if err != nil {
		return fmt.Errorf("marshal workflow error: %v", err)
	}
	params, err := getWorkflowDefaultParams(workflow, taskID, creator)
	if err != nil {
		return fmt.Errorf("get workflow default params error: %v", err)
	}
	replacedString := renderMultiLineString(string(b), setting.RenderValueTemplate, params)
	return json.Unmarshal([]byte(replacedString), &workflow)
}

func renderString(value, template string, inputs []*commonmodels.Param) string {
	for _, input := range inputs {
		value = strings.ReplaceAll(value, fmt.Sprintf(template, input.Name), input.Value)
	}
	return value
}

func renderMultiLineString(value, template string, inputs []*commonmodels.Param) string {
	for _, input := range inputs {
		inputValue := strings.ReplaceAll(input.Value, "\n", "\\n")
		value = strings.ReplaceAll(value, fmt.Sprintf(template, input.Name), inputValue)
	}
	return value
}

func getWorkflowDefaultParams(workflow *commonmodels.WorkflowV4, taskID int64, creator string) ([]*commonmodels.Param, error) {
	resp := []*commonmodels.Param{}
	resp = append(resp, &commonmodels.Param{Name: "project", Value: workflow.Project, ParamsType: "string", IsCredential: false})
	resp = append(resp, &commonmodels.Param{Name: "workflow.name", Value: workflow.Name, ParamsType: "string", IsCredential: false})
	resp = append(resp, &commonmodels.Param{Name: "workflow.task.id", Value: fmt.Sprintf("%d", taskID), ParamsType: "string", IsCredential: false})
	resp = append(resp, &commonmodels.Param{Name: "workflow.task.creator", Value: creator, ParamsType: "string", IsCredential: false})
	resp = append(resp, &commonmodels.Param{Name: "workflow.task.timestamp", Value: fmt.Sprintf("%d", time.Now().Unix()), ParamsType: "string", IsCredential: false})
	for _, stage := range workflow.Stages {
		for _, job := range stage.Jobs {
			switch job.JobType {
			case config.JobZadigBuild:
				build := new(commonmodels.ZadigBuildJobSpec)
				if err := commonmodels.IToi(job.Spec, build); err != nil {
					return nil, errors.Wrap(err, "Itoi")
				}
				var serviceAndModuleName, branchList []string
				for _, serviceAndBuild := range build.ServiceAndBuilds {
					serviceAndModuleName = append(serviceAndModuleName, serviceAndBuild.ServiceModule+"/"+serviceAndBuild.ServiceName)
					branch, commitID := "", ""
					if len(serviceAndBuild.Repos) > 0 {
						branch = serviceAndBuild.Repos[0].Branch
						commitID = serviceAndBuild.Repos[0].CommitID
					}
					branchList = append(branchList, branch)
					resp = append(resp, &commonmodels.Param{Name: fmt.Sprintf("job.%s.%s.%s.BRANCH",
						job.Name, serviceAndBuild.ServiceName, serviceAndBuild.ServiceModule),
						Value: branch, ParamsType: "string", IsCredential: false})
					resp = append(resp, &commonmodels.Param{Name: fmt.Sprintf("job.%s.%s.%s.COMMITID",
						job.Name, serviceAndBuild.ServiceName, serviceAndBuild.ServiceModule),
						Value: commitID, ParamsType: "string", IsCredential: false})
				}
				resp = append(resp, &commonmodels.Param{Name: fmt.Sprintf("job.%s.SERVICES", job.Name), Value: strings.Join(serviceAndModuleName, ","), ParamsType: "string", IsCredential: false})
				resp = append(resp, &commonmodels.Param{Name: fmt.Sprintf("job.%s.BRANCHES", job.Name), Value: strings.Join(branchList, ","), ParamsType: "string", IsCredential: false})
			case config.JobZadigDeploy:
				deploy := new(commonmodels.ZadigDeployJobSpec)
				if err := commonmodels.IToi(job.Spec, deploy); err != nil {
					return nil, errors.Wrap(err, "Itoi")
				}
				resp = append(resp, &commonmodels.Param{Name: fmt.Sprintf("job.%s.envName", job.Name), Value: deploy.Env, ParamsType: "string", IsCredential: false})
			}
		}
	}
	for _, param := range workflow.Params {
		paramsKey := strings.Join([]string{"workflow", "params", param.Name}, ".")
		resp = append(resp, &commonmodels.Param{Name: paramsKey, Value: param.Value, ParamsType: "string", IsCredential: false})
	}
	return resp, nil
}

func renderParams(input, origin []*commonmodels.Param) []*commonmodels.Param {
	for i, originParam := range origin {
		for _, inputParam := range input {
			if originParam.Name == inputParam.Name {
				// always use origin credential config.
				isCredential := originParam.IsCredential
				origin[i] = inputParam
				origin[i].IsCredential = isCredential
			}
		}
	}
	return origin
}

func getJobRankMap(stages []*commonmodels.WorkflowStage) map[string]int {
	resp := make(map[string]int, 0)
	index := 0
	for _, stage := range stages {
		for _, job := range stage.Jobs {
			if !stage.Parallel {
				index++
			}
			resp[job.Name] = index
		}
		index++
	}
	return resp
}

func getOutputKey(jobKey string, outputs []*commonmodels.Output) []string {
	resp := []string{}
	for _, output := range outputs {
		resp = append(resp, job.GetJobOutputKey(jobKey, output.Name))
	}
	return resp
}

// generate script to save outputs variable to file
func outputScript(outputs []*commonmodels.Output) []string {
	resp := []string{"set +ex"}
	for _, output := range outputs {
		resp = append(resp, fmt.Sprintf("echo $%s > %s", output.Name, path.Join(job.JobOutputDir, output.Name)))
	}
	return resp
}

func checkOutputNames(outputs []*commonmodels.Output) error {
	for _, output := range outputs {
		if match := OutputNameRegex.MatchString(output.Name); !match {
			return fmt.Errorf("output name must match %s", OutputNameRegexString)
		}
	}
	return nil
}

func getShareStorageDetail(shareStorages []*commonmodels.ShareStorage, shareStorageInfo *commonmodels.ShareStorageInfo, workflowName string, taskID int64) []*commonmodels.StorageDetail {
	resp := []*commonmodels.StorageDetail{}
	if shareStorageInfo == nil {
		return resp
	}
	if !shareStorageInfo.Enabled {
		return resp
	}
	if len(shareStorages) == 0 || len(shareStorageInfo.ShareStorages) == 0 {
		return resp
	}
	storageMap := make(map[string]*commonmodels.ShareStorage, len(shareStorages))
	for _, shareStorage := range shareStorages {
		storageMap[shareStorage.Name] = shareStorage
	}
	for _, storageInfo := range shareStorageInfo.ShareStorages {
		storage, ok := storageMap[storageInfo.Name]
		if !ok {
			continue
		}
		storageDetail := &commonmodels.StorageDetail{
			Name:      storageInfo.Name,
			Type:      types.NFSMedium,
			SubPath:   types.GetShareStorageSubPath(workflowName, storageInfo.Name, taskID),
			MountPath: storage.Path,
		}
		resp = append(resp, storageDetail)
	}
	return resp
}

func getEnvFromCommitMsg(commitMsg string) []*commonmodels.KeyVal {
	resp := []*commonmodels.KeyVal{}
	if commitMsg == "" {
		return resp
	}
	compileRegex := regexp.MustCompile(`(?U)#(\w+=.+)#`)
	kvArrs := compileRegex.FindAllStringSubmatch(commitMsg, -1)
	for _, kvArr := range kvArrs {
		if len(kvArr) == 0 {
			continue
		}
		keyValStr := kvArr[len(kvArr)-1]
		keyValArr := strings.Split(keyValStr, "=")
		if len(keyValArr) == 2 {
			resp = append(resp, &commonmodels.KeyVal{Key: keyValArr[0], Value: keyValArr[1], Type: commonmodels.StringType})
		}
	}
	return resp
}

func warpJobError(jobName string, err error) error {
	return fmt.Errorf("[job: %s] %v", jobName, err)
}

func getOriginJobName(workflow *commonmodels.WorkflowV4, jobName string) string {
	return getOriginJobNameByRecursion(workflow, jobName, 0)
}

func getOriginJobNameByRecursion(workflow *commonmodels.WorkflowV4, jobName string, depth int) string {
	// Recursion depth limit to 10
	if depth > 10 {
		return jobName
	}
	depth++
	for _, stage := range workflow.Stages {
		for _, job := range stage.Jobs {
			switch v := job.Spec.(type) {
			case commonmodels.ZadigDistributeImageJobSpec:
				if v.Source == config.SourceFromJob {
					return getOriginJobNameByRecursion(workflow, v.JobName, depth)
				}
			case *commonmodels.ZadigDistributeImageJobSpec:
				if v.Source == config.SourceFromJob {
					return getOriginJobNameByRecursion(workflow, v.JobName, depth)
				}
			case commonmodels.ZadigDeployJobSpec:
				if v.Source == config.SourceFromJob {
					return getOriginJobNameByRecursion(workflow, v.JobName, depth)
				}
			case *commonmodels.ZadigDeployJobSpec:
				if v.Source == config.SourceFromJob {
					return getOriginJobNameByRecursion(workflow, v.JobName, depth)
				}
			}

		}
	}
	return jobName
}

func findMatchedRepoFromParams(params []*commonmodels.Param, paramName string) (*types.Repository, error) {
	for _, param := range params {
		if param.Name == paramName {
			if param.ParamsType != "repo" {
				continue
			}
			return param.Repo, nil
		}
	}
	return nil, fmt.Errorf("not found repo from params")
}
