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

package scheduler

import (
	"time"

	"github.com/koderover/zadig/pkg/microservice/cron/core/service"
	"github.com/koderover/zadig/pkg/setting"
	"github.com/koderover/zadig/pkg/types"
	"go.uber.org/zap"
)

func (c *CronClient) UpdatePmHostStatusScheduler(log *zap.SugaredLogger) {
	hosts, err := c.AslanCli.ListPmHosts(log)
	if err != nil {
		log.Error(err)
		return
	}

	log.Info("start init pm host status scheduler..")
	for _, hostElem := range hosts {
		go func(hostPm *service.PrivateKeyHosts, log *zap.SugaredLogger) {
			if hostPm.Port == 0 {
				hostPm.Port = setting.PMHostDefaultPort
			}
			newStatus := setting.PMHostStatusAbnormal
			if hostPm.Probe == nil {
				hostPm.Probe = &types.Probe{ProbeScheme: setting.ProtocolTCP}
			}
			var err error
			msg := ""

			switch hostPm.Probe.ProbeScheme {
			case setting.ProtocolHTTP, setting.ProtocolHTTPS:
				if hostPm.Probe.HttpProbe == nil {
					break
				}
				msg, err = doHTTPProbe(string(hostPm.Probe.ProbeScheme), hostPm.IP, hostPm.Probe.HttpProbe.Path, hostPm.Probe.HttpProbe.Port, hostPm.Probe.HttpProbe.HTTPHeaders, time.Duration(hostPm.Probe.HttpProbe.TimeOutSecond)*time.Second, hostPm.Probe.HttpProbe.ResponseSuccessFlag, log)
				if err != nil {
					log.Warnf("doHttpProbe err:%s", err)
				}
			case setting.ProtocolTCP:
				msg, err = doTCPProbe(hostPm.IP, int(hostPm.Port), 3*time.Second, log)
				if err != nil {
					log.Warnf("doTCPProbe TCP %s:%d err: %s)", hostPm.IP, hostPm.Port, err)
				}
			}

			if msg == Success {
				newStatus = setting.PMHostStatusNormal
			}

			if hostPm.Status == newStatus {
				return
			}
			hostPm.Status = newStatus

			err = c.AslanCli.UpdatePmHost(hostPm, log)
			if err != nil {
				log.Error(err)
			}
		}(hostElem, log)
	}
}
