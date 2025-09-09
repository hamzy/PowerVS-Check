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
	Name               string
	Available          string
	AvailableMessage   string
	Degraded           string
	DegradedMessage    string
	Progressing        string
	ProgressingMessage string
	Upgradeable        string
	UpgradeableMessage string
}

type statusCondition struct {
	Name    string
	Address string
	Status  bool
	Type    string
}

type secretInfo struct {
	Name      string
	Namespace string
}

type podInfo struct {
	Name       string
	Namespace  string
	Phase      string
	Containers []containerInfo
}

type containerInfo struct {
	Name            string
	State           string
	HasRestartCount bool
	RestartCount    float64
	ExitCode        float64
	Reason          string
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

func jsonMapHasKey(jsonMap map[string]any, key string, bufferedChannel chan error) (ok bool) {
	_, ok = jsonMap[key]
	return
}

func getJsonArrayValue(jsonMap map[string]any, key string, bufferedChannel chan error) (jsonArrayValue []any) {
	jsonArrayValue, ok := jsonMap[key].([]any)
	if !ok {
		bufferedChannel<-fmt.Errorf("getJsonArrayValue: jsonMap[%s] returned error", key)
	}
	return
}

func getJsonMapValue(jsonMap map[string]any, key string, bufferedChannel chan error) (jsonMapValue map[string]any) {
	jsonMapValue, ok := jsonMap[key].(map[string]any)
	if !ok {
		bufferedChannel<-fmt.Errorf("getJsonMapValue: jsonMap[%s] returned error", key)
	}
	return
}

func getJsonMapString(jsonMap map[string]any, key string, bufferedChannel chan error) (jsonMapString string) {
	jsonMapString, ok := jsonMap[key].(string)
	if !ok {
		bufferedChannel<-fmt.Errorf("getJsonMapString: jsonMap[%s] returned error", key)
	}
	return
}

func getJsonMapInt64(jsonMap map[string]any, key string, bufferedChannel chan error) (jsonMapInt64 int64) {
	jsonMapInt64, ok := jsonMap[key].(int64)
	if !ok {
		bufferedChannel<-fmt.Errorf("getJsonMapInt64: jsonMap[%s] returned error", key)
	}
	return
}

func getJsonMapFloat64(jsonMap map[string]any, key string, bufferedChannel chan error) (jsonMapFloat64 float64) {
	jsonMapFloat64, ok := jsonMap[key].(float64)
	if !ok {
		bufferedChannel<-fmt.Errorf("getJsonMapFloat64: jsonMap[%s] returned error", key)
	}
	return
}

func getJsonMapBool(jsonMap map[string]any, key string, bufferedChannel chan error) (jsonMapBool bool) {
	jsonMapBool, ok := jsonMap[key].(bool)
	if ok {
		return
	}
	// Fallback to string attempt
	jsonMapString, ok := jsonMap[key].(string)
	if !ok {
		bufferedChannel<-fmt.Errorf("getJsonMapBool: jsonMap[%s] returned error", key)
	}
	switch jsonMapString {
	case "True":
		jsonMapBool = true
	case "False":
		jsonMapBool = false
	default:
		bufferedChannel<-fmt.Errorf("getJsonMapBool: Could not convert jsonMapString (%s)", jsonMapString)
	}
	return
}

func getJsonMap(unknown any, bufferedChannel chan error) (jsonMap map[string]any) {
	jsonMap, ok := unknown.(map[string]any)
	if !ok {
		bufferedChannel<-fmt.Errorf("getJsonMap: converting to map returned error")
	}
	return
}

func gatherBufferedErrors(bufferedChannel chan error) (err error) {
        stillHaveErrors := true
        err = nil
        for stillHaveErrors {
                select {
                case oneError := <-bufferedChannel:
                        log.Debugf("gatherBufferedErrors found buffered error: %+v", oneError)
                        if err == nil {
                                err = oneError
                        }
                default:
                        stillHaveErrors = false
                }
        }
	return
}

func getPVSCluster(jsonPVSCluster map[string]any, bufferedChannel chan error) (aconditions []statusCondition, clusterReady bool) {
	var (
		rootItemArray   []any
		rootItemMap     map[string]any
		statusMap       map[string]any
		conditionsArray []any
	)

	rootItemArray = getJsonArrayValue(jsonPVSCluster, "items", bufferedChannel)
	if len(rootItemArray) != 1 {
		bufferedChannel<-fmt.Errorf("getPVSCluster: len of JSON items != 1 (%d)", len(rootItemArray))
		return
	}

	rootItemMap = getJsonMap(rootItemArray[0], bufferedChannel)
	statusMap = getJsonMapValue(rootItemMap, "status", bufferedChannel)
	conditionsArray = getJsonArrayValue(statusMap, "conditions", bufferedChannel)

	aconditions = make([]statusCondition, 0)

	for _, conditionItem := range conditionsArray {
		var (
			conditionItemMap map[string]any
			status           bool
			stringType       string
			sc               statusCondition
		)

		conditionItemMap = getJsonMap(conditionItem, bufferedChannel)

		status = getJsonMapBool(conditionItemMap, "status", bufferedChannel)
		stringType = getJsonMapString(conditionItemMap, "type", bufferedChannel)

		sc = statusCondition{
			Status: status,
			Type:   stringType,
		}
		log.Debugf("getPVSCluster: sc = %+v", sc)

		aconditions = append(aconditions, sc)
	}

	clusterReady = getJsonMapBool(statusMap, "ready", bufferedChannel)
	log.Debugf("getPVSCluster: clusterReady = %v", clusterReady)

	return
}

func getPVSMachines(jsonPVSMachines map[string]any, bufferedChannel chan error) (aconditions []statusCondition) {
	var (
		rootItemArray []any
	)

	rootItemArray = getJsonArrayValue(jsonPVSMachines, "items", bufferedChannel)

	aconditions = make([]statusCondition, 0)

	for _, rootItem := range rootItemArray {
		var (
			itemMap         map[string]any
			metadataMap     map[string]any
			name            string
			statusMap       map[string]any
			addressesArray  []any
			address         string
			conditionsArray []any
		)

		itemMap = getJsonMap(rootItem, bufferedChannel)

		metadataMap = getJsonMapValue(itemMap, "metadata", bufferedChannel)
		name = getJsonMapString(metadataMap, "name", bufferedChannel)

		statusMap = getJsonMapValue(itemMap, "status", bufferedChannel)
		addressesArray = getJsonArrayValue(statusMap, "addresses", bufferedChannel)

		address = ""
		for _, addresseItem := range addressesArray {
			var (
				addressItemMap map[string]any
				stringAddress  string
				stringType     string
			)

			addressItemMap = getJsonMap(addresseItem, bufferedChannel)

			stringAddress = getJsonMapString(addressItemMap, "address", bufferedChannel)
			stringType = getJsonMapString(addressItemMap, "type", bufferedChannel)

			if stringType == "InternalIP" {
				address = stringAddress
			}
		}

		conditionsArray = getJsonArrayValue(statusMap, "conditions", bufferedChannel)

		for _, conditionItem := range conditionsArray {
			var (
				conditionItemMap map[string]any
				status           bool
				stringType       string
				sc               statusCondition
			)

			conditionItemMap = getJsonMap(conditionItem, bufferedChannel)

			status = getJsonMapBool(conditionItemMap, "status", bufferedChannel)
			stringType = getJsonMapString(conditionItemMap, "type", bufferedChannel)

			sc = statusCondition{
				Name:    name,
				Address: address,
				Status:  status,
				Type:    stringType,
			}
			log.Debugf("getPVSMachines: sc = %+v", sc)

			aconditions = append(aconditions, sc)
		}
	}

	return
}

func getPVSImage(jsonPVSImage map[string]any, bufferedChannel chan error) (aconditions []statusCondition) {
	var (
		rootItemArray   []any
		rootItemMap     map[string]any
		statusMap       map[string]any
		conditionsArray []any
	)

	rootItemArray = getJsonArrayValue(jsonPVSImage, "items", bufferedChannel)
	if len(rootItemArray) != 1 {
		bufferedChannel<-fmt.Errorf("getPVSImage: len of JSON items != 1 (%d)", len(rootItemArray))
		return aconditions
	}

	rootItemMap = getJsonMap(rootItemArray[0], bufferedChannel)
	statusMap = getJsonMapValue(rootItemMap, "status", bufferedChannel)
	conditionsArray = getJsonArrayValue(statusMap, "conditions", bufferedChannel)

	aconditions = make([]statusCondition, 0)

	for _, conditionItem := range conditionsArray {
		var (
			conditionItemMap map[string]any
			status           bool
			stringType       string
			sc               statusCondition
		)

		conditionItemMap = getJsonMap(conditionItem, bufferedChannel)

		status = getJsonMapBool(conditionItemMap, "status", bufferedChannel)
		stringType = getJsonMapString(conditionItemMap, "type", bufferedChannel)

		sc = statusCondition{
			Status: status,
			Type:   stringType,
		}
		log.Debugf("getPVSImage: sc = %+v", sc)

		aconditions = append(aconditions, sc)
	}

	return
}

func getClusterOperator(jsonCo map[string]any, name string, bufferedChannel chan error) (cc clusterConditions) {
	var (
		rootItemArray []any
		found         bool
	)

	rootItemArray = getJsonArrayValue(jsonCo, "items", bufferedChannel)
	log.Debugf("getClusterOperator: len(rootItemArray) = %d", len(rootItemArray))

	found = false

	for _, clusterItem := range rootItemArray {
		var (
			clusterItemMap  map[string]any
			metadataMap     map[string]any
			metadataName    string
			statusMap       map[string]any
			conditionsArray []any
		)

		clusterItemMap = getJsonMap(clusterItem, bufferedChannel)

		metadataMap = getJsonMapValue(clusterItemMap, "metadata", bufferedChannel)

		metadataName = getJsonMapString(metadataMap, "name", bufferedChannel)

		if name != metadataName {
			continue
		}

//		log.Debugf("clusterItem = %+v", clusterItem)

		cc.Name = metadataName

		found = true

		statusMap = getJsonMapValue(clusterItemMap, "status", bufferedChannel)

//		value, exists := statusMap["conditions"]
//		log.Debugf("value = %v", value)
//		log.Debugf("type = %T", value)
//		log.Debugf("exists = %v", exists)

		if ok := jsonMapHasKey(statusMap, "conditions", bufferedChannel); ok {
			conditionsArray = getJsonArrayValue(statusMap, "conditions", bufferedChannel)
		} else {
			conditionsArray = nil
		}

		for _, conditionItem := range conditionsArray {
			var (
				conditionMap  map[string]any
				typeResult    string
				statusResult  string
				messageResult string
			)

			conditionMap = getJsonMap(conditionItem, bufferedChannel)

			typeResult = getJsonMapString(conditionMap, "type", bufferedChannel)
			statusResult = getJsonMapString(conditionMap, "status", bufferedChannel)
			if ok := jsonMapHasKey(conditionMap, "message", bufferedChannel); ok {
				messageResult = getJsonMapString(conditionMap, "message", bufferedChannel)
			}

			switch typeResult {
			case "Available":
				cc.Available = statusResult
				cc.AvailableMessage = messageResult
			case "Degraded":
				cc.Degraded = statusResult
				cc.DegradedMessage = messageResult
			case "Progressing":
				cc.Progressing = statusResult
				cc.ProgressingMessage = messageResult
			case "Upgradeable":
				cc.Upgradeable = statusResult
				cc.UpgradeableMessage = messageResult
			default:
				log.Debugf("getClusterOperator: unknown type %s", typeResult)
			}
		}
	}

	if !found {
		bufferedChannel<-fmt.Errorf("Could not find cluster operator named %s", name)
		return
	}

	return
}

func getDeployment(jsonDeployment map[string]any, bufferedChannel chan error) (cc clusterConditions) {
	var (
		statusMap       map[string]any
		conditionsArray []any
	)

	statusMap = getJsonMapValue(jsonDeployment, "status", bufferedChannel)
//	log.Debugf("getDeployment: statusMap = %+v", statusMap)

	if ok := jsonMapHasKey(statusMap, "conditions", bufferedChannel); ok {
		conditionsArray = getJsonArrayValue(statusMap, "conditions", bufferedChannel)
	} else {
		conditionsArray = nil
	}
//	log.Debugf("getDeployment: conditionsArray = %+v", conditionsArray)

	for _, conditionItem := range conditionsArray {
		var (
			conditionMap map[string]any
			typeResult   string
			statusResult string
		)

		conditionMap = getJsonMap(conditionItem, bufferedChannel)

		typeResult = getJsonMapString(conditionMap, "type", bufferedChannel)
		statusResult = getJsonMapString(conditionMap, "status", bufferedChannel)

		switch typeResult {
		case "Available":
			cc.Available = statusResult
		case "Progressing":
			cc.Progressing = statusResult
		default:
			log.Debugf("getDeployment: unknown type %s", typeResult)
		}
	}

	return
}

func getSecrets(jsonSecrets map[string]any, bufferedChannel chan error) (asecrets []secretInfo) {
	var (
		rootItemArray []any
	)

	rootItemArray = getJsonArrayValue(jsonSecrets, "items", bufferedChannel)

	asecrets = make([]secretInfo, 0)

	for _, rootItem := range rootItemArray {
		var (
			itemMap     map[string]any
			metadataMap map[string]any
			name        string
			namespace   string
		)

		itemMap = getJsonMap(rootItem, bufferedChannel)

		metadataMap = getJsonMapValue(itemMap, "metadata", bufferedChannel)
		name = getJsonMapString(metadataMap, "name", bufferedChannel)
		namespace = getJsonMapString(metadataMap, "namespace", bufferedChannel)

		asecrets = append(asecrets, secretInfo{
			Name:      name,
			Namespace: namespace,
		})
	}

	return
}

func getPods(jsonPods map[string]any, bufferedChannel chan error) (apods []podInfo) {
	var (
		rootItemArray []any
	)

	rootItemArray = getJsonArrayValue(jsonPods, "items", bufferedChannel)

	apods = make([]podInfo, 0)

	for _, rootItem := range rootItemArray {
		var (
			itemMap           map[string]any
			metadataMap       map[string]any
			name              string
			namespace         string
			statusMap         map[string]any
			phase             string
			containerStatuses []any
			acontainers       []containerInfo
		)

		itemMap = getJsonMap(rootItem, bufferedChannel)

		metadataMap = getJsonMapValue(itemMap, "metadata", bufferedChannel)
		name = getJsonMapString(metadataMap, "name", bufferedChannel)
		namespace = getJsonMapString(metadataMap, "namespace", bufferedChannel)

		statusMap = getJsonMapValue(itemMap, "status", bufferedChannel)
		phase = getJsonMapString(statusMap, "phase", bufferedChannel)

		if jsonMapHasKey(statusMap, "containerStatuses", bufferedChannel) {
			containerStatuses = getJsonArrayValue(statusMap, "containerStatuses", bufferedChannel)
		}

		acontainers = make([]containerInfo, 0)
		for _, containerStatus := range containerStatuses {
			var (
				containerStatusMap map[string]any
				containerName      string
				restartCount       float64
				stateMap           map[string]any
				state              string
				exitCode           float64
				reason             string
			)

			containerStatusMap = getJsonMap(containerStatus, bufferedChannel)
			containerName = getJsonMapString(containerStatusMap, "name", bufferedChannel)

			stateMap = getJsonMapValue(containerStatusMap, "state", bufferedChannel)

			state = ""
			if jsonMapHasKey(stateMap, "running", bufferedChannel) {
				state = "(running)"
			} else if jsonMapHasKey(stateMap, "terminated", bufferedChannel) {
				state = "(terminated)"
				terminatedMap := getJsonMapValue(stateMap, "terminated", bufferedChannel)
				exitCode = getJsonMapFloat64(terminatedMap, "exitCode", bufferedChannel)
			} else if jsonMapHasKey(stateMap, "waiting", bufferedChannel) {
				state = "(waiting)"
				waitingMap := getJsonMapValue(stateMap, "waiting", bufferedChannel)
				reason = getJsonMapString(waitingMap, "reason", bufferedChannel)
			}

			hasRestartCount := jsonMapHasKey(containerStatusMap, "restartCount", bufferedChannel)
			if hasRestartCount {
				restartCount = getJsonMapFloat64(containerStatusMap, "restartCount", bufferedChannel)
			}

			acontainers = append(acontainers, containerInfo{
				Name:            containerName,
				State:           state,
				HasRestartCount: hasRestartCount,
				RestartCount:    restartCount,
				ExitCode:        exitCode,
				Reason:          reason,
			})
		}

		apods = append(apods, podInfo{
			Name:       name,
			Namespace:  namespace,
			Phase:      phase,
			Containers: acontainers,
		})
	}

	return
}
