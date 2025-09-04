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

// (/bin/rm go.*; go mod init example/user/PowerVS-Check; go mod tidy)
// (echo "vet:"; go vet || exit 1; echo "build:"; go build -ldflags="-X main.version=$(git describe --always --long --dirty) -X main.release=$(git describe --tags --abbrev=0)" -o PowerVS-Check-Create *.go || exit 1; echo "run:"; ./PowerVS-Check check-create -apiKey "..." -metadata metadata.json -shouldDebug true)

package main

import (
	"fmt"

	"encoding/json"
	"os"
)

type clusterConditions struct {
	Available   string
	Degraded    string
	Progressing string
	Upgradeable string
}

type statusCondition struct {
	Status bool
	Type   string
}

func parseJsonFile(filename string) (map[string]interface{}, error) {
	var (
		data     []byte
		jsonData map[string]interface{}
		err      error
	)

	data, err = os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, &jsonData)
	if err != nil {
		return nil, err
	}

//	if false {
//		jsonData, err = convertMap(jsonData)
//	}

	return jsonData, err
}

func getPVSCluster(jsonPVSCluster map[string]interface{}) ([]statusCondition, error) {
	var (
		rootItemArray   []interface{}
		ok              bool
		rootItemMap     map[string]interface{}
		statusMap       map[string]interface{}
		conditionsArray []interface{}
		aconditions     []statusCondition
	)

	rootItemArray, ok = jsonPVSCluster["items"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("getPVSCluster: Could not find JSON items")
	}

	if len(rootItemArray) != 1 {
		return nil, fmt.Errorf("getPVSCluster: len of JSON items != 1 (%d)", len(rootItemArray))
	}

	rootItemMap, ok = rootItemArray[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("getPVSCluster: Could not convert to rootItemMap")
	}
	log.Debugf("rootItemMap = %+v", rootItemMap)

	statusMap, ok = rootItemMap["status"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("getPVSCluster: Could not convert to statusMap")
	}
	log.Debugf("statusMap = %+v", statusMap)

	conditionsArray, ok = statusMap["conditions"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("getPVSCluster: Could not convert to conditionsMap")
	}
	log.Debugf("conditionsArray = %+v", conditionsArray)

	aconditions = make([]statusCondition, 0)
	for _, item := range conditionsArray {
		var (
			itemMap    map[string]interface{}
			status     bool
			stringType string
			sc         statusCondition
		)

		itemMap, ok = item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("getPVSCluster: Could not convert to itemMap: %v", item)
		}

		switch itemMap["status"] {
		case "True":
			status = true
		case "False":
			status = false
		default:
			return nil, fmt.Errorf("getPVSCluster: Could not convert itemMap status: %v", itemMap["status"])
		}

		stringType, ok = itemMap["type"].(string)
		if !ok {
			return nil, fmt.Errorf("getPVSCluster: Could not convert itemMap type: %v", itemMap["type"])
		}

		sc = statusCondition{
			Status: status,
			Type:   stringType,
		}
		log.Debugf("sc = %+v", sc)

		aconditions = append(aconditions, sc)
	}

	return aconditions, nil
}

func getClusterOperator(jsonCo map[string]interface{}, name string) (clusterConditions, error) {
	var (
		ok          bool
		statusMap   map[string]interface{}
		aconditions []interface{}
		cc          clusterConditions
		found       bool
		err         error
	)

	items, ok := jsonCo["items"].([]interface {})
	if !ok {
		return cc, fmt.Errorf("getClusterOperator: Could not find JSON items")
	}
	log.Debugf("len(items) = %d", len(items))

	found = false
	for _, item := range items {
		itemMap, ok := item.(map[string]interface {})
		if !ok {
			return cc, fmt.Errorf("Could not convert item to itemMap")
		}

		metadataMap, ok := itemMap["metadata"].(map[string]interface {})
		if !ok {
			return cc, fmt.Errorf("Could not convert itemMap to metadataMap")
		}

		metadataName, ok := metadataMap["name"].(string)
		if !ok {
			return cc, fmt.Errorf("Could not convert metadataMap to metadataName")
		}
//		log.Debugf("name = %s", metadataName)

		if name != metadataName {
			continue
		}

		found = true

		statusMap, ok = itemMap["status"].(map[string]interface{})
		if !ok {
			return cc, fmt.Errorf("Could not find status in cluster operator named %s", name)
		}

//		value, exists := statusMap["conditions"]
//		log.Debugf("value = %v", value)
//		log.Debugf("type = %T", value)
//		log.Debugf("exists = %v", exists)

		aconditions, ok = statusMap["conditions"].([]interface{})
		if !ok {
			return cc, fmt.Errorf("Could not find conditions in cluster operator named %s", name)
		}

		for _, condition := range aconditions {
			conditionMap, ok := condition.(map[string]interface{})
			if !ok {
				return cc, fmt.Errorf("Could create conditionMap for cluster operator named %s", name)
			}

			typeResult, ok := conditionMap["type"].(string)
			if !ok {
				return cc, fmt.Errorf("Could not find condition type in cluster operator named %s", name)
			}

			statusResult, ok := conditionMap["status"].(string)
			if !ok {
				return cc, fmt.Errorf("Could not find condition status in cluster operator named %s", name)
			}

			switch typeResult {
			case "Available":
				cc.Available = statusResult
			case "Degraded":
				cc.Degraded = statusResult
			case "Progressing":
				cc.Progressing = statusResult
			case "Upgradeable":
				cc.Upgradeable = statusResult
			}
		}
	}

	if !found {
		return cc, fmt.Errorf("Could not find cluster operator named %s", name)
	}

	return cc, err
}
