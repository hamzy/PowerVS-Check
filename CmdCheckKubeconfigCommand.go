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

	"github.com/sirupsen/logrus"
)

func checkKubeconfigCommand(checkKubeconfigFlags *flag.FlagSet, args []string) error {
	var (
		out            io.Writer
		ptrShouldDebug *string
		ptrKubeconfig  *string
		cmds           = []string{
			"oc --request-timeout=5s get clusterversion",
			"oc --request-timeout=5s get co",
			"oc --request-timeout=5s get nodes -o=wide",
			"oc --request-timeout=5s get pods -n openshift-machine-api",
			"oc --request-timeout=5s get machines.machine.openshift.io -n openshift-machine-api",
			"oc --request-timeout=5s get machineset.machine.openshift.io -n openshift-machine-api",
			"oc --request-timeout=5s logs -l k8s-app=controller -c machine-controller -n openshift-machine-api",
			"oc --request-timeout=5s get co/cloud-controller-manager",
			"oc --request-timeout=5s describe cm/cloud-provider-config -n openshift-config",
			"oc --request-timeout=5s get pods -n openshift-cloud-controller-manager-operator",
			"oc --request-timeout=5s get events -n openshift-cloud-controller-manager",
			"oc --request-timeout=5s logs -l k8s-app=powervs-cloud-controller-manager -n openshift-cloud-controller-manager",
			"oc --request-timeout=5s -n openshift-cloud-controller-manager-operator logs deployment/cluster-cloud-controller-manager-operator -c cluster-cloud-controller-manager",
			"oc --request-timeout=5s get co/network",
			"oc --request-timeout=5s get machines.machine.openshift.io -n openshift-machine-api",
			"oc --request-timeout=5s get machineset.m -n openshift-machine-api",
			"oc --request-timeout=5s get pods -n openshift-machine-api",
			"oc --request-timeout=5s describe co/machine-config",
		}
		pipeCmds = [][]string{
			{
				"oc --request-timeout=5s get pods -A -o=wide",
				"sed -e /\\(Running\\|Completed\\)/d",
			},
		}
		err error
	)

	ptrShouldDebug = checkKubeconfigFlags.String("shouldDebug", "false", "Should output debug output")
	ptrKubeconfig = checkKubeconfigFlags.String("kubeconfig", "", "The KUBECONFIG file")

	checkKubeconfigFlags.Parse(args)

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

	for _, cmd := range cmds {
		err = runCommand(*ptrKubeconfig, cmd)
		if err != nil {
			fmt.Printf("Error: could not run command: %v\n", err)
		}
	}

	for _, twoCmds := range pipeCmds {
		err = runTwoCommands(*ptrKubeconfig, twoCmds[0], twoCmds[1])
		if err != nil {
			fmt.Printf("Error: could not run command: %v\n", err)
		}
	}

	return nil
}
