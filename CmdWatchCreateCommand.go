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
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/sirupsen/logrus"
)

const (
	useTview = false
	useSavedJson = true
)

func watchCreateCommand(watchCreateClusterFlags *flag.FlagSet, args []string) error {
	var (
		out            io.Writer
		ptrShouldDebug *string
		ptrKubeconfig  *string
		err            error
	)

	ptrShouldDebug = watchCreateClusterFlags.String("shouldDebug", "false", "Should output debug output")
	ptrKubeconfig = watchCreateClusterFlags.String("kubeconfig", "", "The KUBECONFIG file")

	watchCreateClusterFlags.Parse(args)

	switch strings.ToLower(*ptrShouldDebug) {
	case "true":
		shouldDebug = true
	case "false":
		shouldDebug = false
	default:
		return fmt.Errorf("Error: shouldDebug is not true/false (%s)\n", *ptrShouldDebug)
	}

	if shouldDebug {
		out = os.Stderr
	} else {
		out = io.Discard
	}
	log = &logrus.Logger{
		Out:       out,
		Formatter: new(logrus.TextFormatter),
		Level:     logrus.DebugLevel,
	}

	if *ptrKubeconfig == "" {
		return fmt.Errorf("Error: No KUBECONFIG key set, use -kubeconfig")
	}

	fmt.Fprintf(os.Stderr, "Program version is %v, release = %v\n", version, release)

	kubeconfigCapi := filepath.Join(*ptrKubeconfig, ".clusterapi_output/envtest.kubeconfig")

	if _, err = os.Stat(kubeconfigCapi); errors.Is(err, os.ErrNotExist) && !useSavedJson {
		return err
	}

	err = watchCAPIPhase(kubeconfigCapi)
	if err != nil {
		return err
	}

	kubeconfigOpenshift := filepath.Join(*ptrKubeconfig, "auth/kubeconfig")

	if _, err = os.Stat(kubeconfigOpenshift); errors.Is(err, os.ErrNotExist) && !useSavedJson {
		return err
	}

	err = watchOpenshiftPhase(kubeconfigOpenshift)
	if err != nil {
		return err
	}

	return nil
}

func updateWindow(capiWindows map[string]*tview.TextView, element string, text string) {
	if useTview {
		capiWindows[element].SetText(text)
	} else {
		fmt.Println(text)
	}
}

func updateCAPIPhase1(kubeconfig string, app *tview.Application, capiWindows map[string]*tview.TextView, chanResult chan<- error) {
	var (
		cmdOcGetPVSCluster = []string{
			"oc", "get", "ibmpowervscluster", "-n", "openshift-cluster-api-guests", "-o", "json",
		}
		jsonPVSCluster     map[string]interface{}
		aconditions        []statusCondition
		clusterReady       bool
		ok                 bool
		conditionsReady    bool
		err                error
	)

	for true {
		if useSavedJson {
			jsonPVSCluster, err = parseJsonFile("ibmpowervscluster2.json")
		} else {
			jsonPVSCluster, err = runSplitCommandJson(kubeconfig, cmdOcGetPVSCluster)
		}
		if err != nil {
			err = fmt.Errorf("Error: could not run command: %v", err)
			break
		}
//		log.Debugf("updateCAPIPhase1: jsonPVSCluster = %+v", jsonPVSCluster)

		// https://medium.com/@sumit-s/mastering-go-channels-from-beginner-to-pro-9c1eaba0da9e

		// @TODO is there a way to avoid the large hardcoded value?
		bufferedChannel := make(chan error, 100)

		aconditions, clusterReady = getPVSCluster(jsonPVSCluster, bufferedChannel)

		err = gatherBufferedErrors(bufferedChannel)
		if err != nil {
			break
		}

		conditionsReady = true
		for _, condition := range aconditions {
			_, ok = capiWindows[condition.Type]
			log.Debugf("updateCAPIPhase1: condition = %+v, ok = %v", condition, ok)
			if !ok {
				continue
			}
			if condition.Status {
				updateWindow(capiWindows, condition.Type, fmt.Sprintf("%s is READY", condition.Type))
			} else {
				conditionsReady = false
				updateWindow(capiWindows, condition.Type, fmt.Sprintf("%s is NOT READY", condition.Type))
			}
		}

		if conditionsReady {
			log.Debugf("updateCAPIPhase1: conditionsReady = %v, len(aconditions) = %d", conditionsReady, len(aconditions))
			log.Debugf("updateCAPIPhase1: clusterReady = %v", clusterReady)

			if len(aconditions) == 8 && clusterReady {
				err = nil
				break
			}
		} else {
			log.Debugf("updateCAPIPhase1: conditionsReady = %v", conditionsReady)
		}

		time.Sleep(10 * time.Second)
	}

	if useTview {
		app.Stop()
	}

	chanResult <- err

	log.Debugf("updateCAPIPhase1: DONE!")
}

func updateCAPIPhase2(kubeconfig string, app *tview.Application, capiWindows map[string]*tview.TextView, chanResult chan<- error) {
	var (
		cmdOcGetPVSImage = []string{
			"oc", "get", "ibmpowervsimage", "-n", "openshift-cluster-api-guests", "-o", "json",
		}
		jsonPVSImage       map[string]interface{}
		aconditions        []statusCondition
		conditionsReady    bool
		err                error
	)

	for true {
		if useSavedJson {
			jsonPVSImage, err = parseJsonFile("ibmpowervsimage.json")
		} else {
			jsonPVSImage, err = runSplitCommandJson(kubeconfig, cmdOcGetPVSImage)
		}
		if err != nil {
			err = fmt.Errorf("Error: could not run command: %v", err)
			break
		}
//		log.Debugf("updateCAPIPhase2: jsonPVSImage = %+v", jsonPVSImage)

		// https://medium.com/@sumit-s/mastering-go-channels-from-beginner-to-pro-9c1eaba0da9e

		// @TODO is there a way to avoid the large hardcoded value?
		bufferedChannel := make(chan error, 100)

		aconditions = getPVSImage(jsonPVSImage, bufferedChannel)

		err = gatherBufferedErrors(bufferedChannel)
		if err != nil {
			break
		}

		conditionsReady = true
		for _, condition := range aconditions {
			log.Debugf("updateCAPIPhase2: condition = %+v", condition)
			if condition.Status {
				updateWindow(capiWindows, condition.Type, fmt.Sprintf("%s is READY", condition.Type))
			} else {
				conditionsReady = false
				updateWindow(capiWindows, condition.Type, fmt.Sprintf("%s is NOT READY", condition.Type))
			}
		}

		if conditionsReady {
			log.Debugf("updateCAPIPhase2: conditionsReady = %v, len(aconditions) = %d", conditionsReady, len(aconditions))
			if len(aconditions) == 2 {
				err = nil
				break
			}
		} else {
			log.Debugf("updateCAPIPhase2: conditionsReady = %v", conditionsReady)
		}

		time.Sleep(10 * time.Second)
	}

	if useTview {
		app.Stop()
	}

	chanResult <- err

	log.Debugf("updateCAPIPhase2: DONE!")
}

func updateCAPIPhase3(kubeconfig string, app *tview.Application, capiWindows map[string]*tview.TextView, chanResult chan<- error) {
	var (
		cmdOcGetPVSMachines = []string{
			"oc", "get", "ibmpowervsmachines", "-n", "openshift-cluster-api-guests", "-o", "json",
		}
		jsonPVSMachines    map[string]interface{}
		aconditions        []statusCondition
		conditionsReady    bool
		err                error
	)

	for true {
		if useSavedJson {
			jsonPVSMachines, err = parseJsonFile("ibmpowervsmachines2.json")
		} else {
			jsonPVSMachines, err = runSplitCommandJson(kubeconfig, cmdOcGetPVSMachines)
		}
		if err != nil {
			err = fmt.Errorf("Error: could not run command: %v", err)
			break
		}
//		log.Debugf("updateCAPIPhase3: jsonPVSMachines = %+v", jsonPVSMachines)

		// https://medium.com/@sumit-s/mastering-go-channels-from-beginner-to-pro-9c1eaba0da9e

		// @TODO is there a way to avoid the large hardcoded value?
		bufferedChannel := make(chan error, 100)

		aconditions = getPVSMachines(jsonPVSMachines, bufferedChannel)

		err = gatherBufferedErrors(bufferedChannel)
		if err != nil {
			break
		}

		conditionsReady = true
		for _, condition := range aconditions {
			log.Debugf("updateCAPIPhase3: condition = %+v", condition)
			if condition.Status {
				updateWindow(capiWindows, condition.Type, fmt.Sprintf("%s is READY", condition.Type))
			} else {
				conditionsReady = false
				updateWindow(capiWindows, condition.Type, fmt.Sprintf("%s is NOT READY", condition.Type))
			}
		}

		if conditionsReady {
			log.Debugf("updateCAPIPhase3: conditionsReady = %v, len(aconditions) = %d", conditionsReady, len(aconditions))
			if len(aconditions) == 8 {
				err = nil
				break
			}
		} else {
			log.Debugf("updateCAPIPhase3: conditionsReady = %v", conditionsReady)
			break // @REMOVE
		}

		time.Sleep(10 * time.Second)
	}

	if useTview {
		app.Stop()
	}

	chanResult <- err

	log.Debugf("updateCAPIPhase3: DONE!")
}

func watchCAPIPhase(kubeconfig string) error {
	var (
		app         *tview.Application
		grid        *tview.Grid
		windowList   = []string {
			"COSInstanceCreated", "LoadBalancerReady", "NetworkReady", "ServiceInstanceReady", "TransitGatewayReady", "VPCReady", "VPCSecurityGroupReady", "VPCSubnetReady",
	}
		capiWindows map[string]*tview.TextView
		chanResult  chan error
		err         error
	)

	newTextView := func(name string) *tview.TextView {
		tv := tview.NewTextView()
		tv.SetText(fmt.Sprintf("%s is unknown", name))
		return tv
	}

	if useTview {
		app = tview.NewApplication()
		grid = tview.NewGrid().SetBorders(true)
	}

	capiWindows = make(map[string]*tview.TextView)

	if useTview {
		for _, name := range windowList {
			capiWindows[name] = newTextView(name)
		}

		position := 1
		for _, name := range windowList {
			grid.AddItem(capiWindows[name], position, 0, 1, 1, 0, 0, false)
			position += 1
		}
	}

	chanResult = make(chan error)

	if useTview {
		go updateCAPIPhase1(kubeconfig, app, capiWindows, chanResult)

//		time.Sleep(15*time.Second)

		if err = app.SetRoot(grid, true).SetFocus(grid).Run(); err != nil {
			return err
		}
	} else {
		go updateCAPIPhase1(kubeconfig, app, capiWindows, chanResult)
		log.Debugf("watchCAPIPhase: updateCAPIPhase1: chan = %+v", <-chanResult)

		go updateCAPIPhase2(kubeconfig, app, capiWindows, chanResult)
		log.Debugf("watchCAPIPhase: updateCAPIPhase2: chan = %+v", <-chanResult)

		go updateCAPIPhase3(kubeconfig, app, capiWindows, chanResult)
		log.Debugf("watchCAPIPhase: updateCAPIPhase3: chan = %+v", <-chanResult)
	}

	return nil
}

func watchOpenshiftPhase(kubeconfig string) error {
	var (
		cmdOcGetCo = []string{
			"oc", "--request-timeout=5s", "get", "co", "-o", "json",
		}
		jsonCo     map[string]interface{}
		cc         clusterConditions
		err        error
	)

if true {
	return nil
}

	jsonCo, err = runSplitCommandJson(kubeconfig, cmdOcGetCo)
	if err != nil {
		return fmt.Errorf("Error: could not run command: %v", err)
	}

	// @TODO is there a way to avoid the large hardcoded value?
	bufferedChannel := make(chan error, 100)

	cc = getClusterOperator(jsonCo, "authentication", bufferedChannel)

	err = gatherBufferedErrors(bufferedChannel)
	if err != nil {
		log.Debugf("watchOpenshiftPhase: getClusterOperator returns %v", err)
	} else {
		log.Debugf("watchOpenshiftPhase: cc = %+v", cc)
	}

	return err
}
