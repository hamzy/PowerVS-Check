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
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/sirupsen/logrus"
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

	err = watchCAPIPhase(ptrKubeconfig)
	if err != nil {
		return err
	}

	err = watchOpenshiftPhase(ptrKubeconfig)
	if err != nil {
		return err
	}

	return nil
}

func updateCAPIPhase(ptrKubeconfig *string, app *tview.Application, capiWindows map[string]*tview.TextView, chanResult chan<- error) {
	var (
		cmdOcGetPVSCluster = []string{
			"oc", "get", "ibmpowervscluster", "-n", "openshift-cluster-api-guests", "-o", "json",
		}
		jsonPVSCluster     map[string]interface{}
		aconditions        []statusCondition
		ok                 bool
		ready              bool
		err                error
	)

	for true {
		if false {
			jsonPVSCluster, err = parseBob()
		} else {
			jsonPVSCluster, err = runSplitCommandJson(ptrKubeconfig, cmdOcGetPVSCluster)
		}
		if err != nil {
			err = fmt.Errorf("Error: could not run command: %v", err)
			break
		}
		log.Debugf("jsonPVSCluster = %+v", jsonPVSCluster)

		aconditions, err = getPVSCluster(jsonPVSCluster)
		if err != nil {
			break
		}

		ready = true
		for _, condition := range aconditions {
			_, ok = capiWindows[condition.Type]
			log.Debugf("condition = %+v, ok = %v", condition, ok)
			if !ok {
				continue
			}
			if condition.Status {
				capiWindows[condition.Type].SetText(fmt.Sprintf("%s is READY", condition.Type))
			} else {
				ready = false
				capiWindows[condition.Type].SetText(fmt.Sprintf("%s is NOT READY", condition.Type))
			}
		}

		if ready {
			log.Debugf("ready = %v, len(aconditions) = %d", ready, len(aconditions))
			if len(aconditions) == 8 {
				err = nil
				break
			}
		} else {
			log.Debugf("ready = %v", ready)
		}

		time.Sleep(10 * time.Second)
	}

	app.Stop()

	chanResult <- err
}

func watchCAPIPhase(ptrKubeconfig *string) error {
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

	app = tview.NewApplication()

	grid = tview.NewGrid().SetBorders(true)

	capiWindows = make(map[string]*tview.TextView)
	for _, name := range windowList {
		capiWindows[name] = newTextView(name)
	}

	position := 1
	for _, name := range windowList {
		grid.AddItem(capiWindows[name], position, 0, 1, 1, 0, 0, false)
		position += 1
	}

	chanResult = make(chan error)

	go updateCAPIPhase(ptrKubeconfig, app, capiWindows, chanResult)

//	time.Sleep(15*time.Second)

	if err = app.SetRoot(grid, true).SetFocus(grid).Run(); err != nil {
		return err
	}

	log.Debugf("chan = %+v", <-chanResult)

	return nil
}

func watchOpenshiftPhase(ptrKubeconfig *string) error {
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

	jsonCo, err = runSplitCommandJson(ptrKubeconfig, cmdOcGetCo)
	if err != nil {
		return fmt.Errorf("Error: could not run command: %v", err)
	}

	cc, err = getClusterOperator(jsonCo, "authentication")
	if err != nil {
		log.Debugf("getClusterOperator returns %v", err)
	} else {
		log.Debugf("cc = %+v", cc)
	}

	return nil
}
