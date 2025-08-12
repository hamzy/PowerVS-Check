// Copyright 2025 IBM Corp
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
)

const (
	xObjectName = "object"
)

type Object struct {
}

func NewObject(services *Services) ([]RunnableObject, error) {
	var (
//		err       error
	)

	return []RunnableObject{&Object{
	}}, nil
}

func (o *Object) CRN() (string, error) {
	return "", fmt.Errorf("@TODO not implemented yet")
}

func (o *Object) Name() (string, error) {
	return "", fmt.Errorf("@TODO not implemented yet")
}

func (o *Object) ObjectName() (string, error) {
	return "@TODO", nil
}

func (o *Object) Run() error {
	return fmt.Errorf("@TODO not implemented yet")
}

func (o *Object) CiStatus(shouldClean bool) {
}

func (o *Object) ClusterStatus() {
}

func (o *Object) Priority() (int, error) {
	return -1, nil
}
