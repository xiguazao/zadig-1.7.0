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

package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type JenkinsIntegration struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"         json:"id,omitempty"`
	URL       string             `bson:"url"                   json:"url"`
	Username  string             `bson:"username"              json:"username"`
	Password  string             `bson:"password"              json:"password"`
	UpdateBy  string             `bson:"update_by"             json:"update_by"`
	UpdatedAt int64              `bson:"updated_at"            json:"updated_at"`
}

func (j JenkinsIntegration) TableName() string {
	return "jenkins_integration"
}
