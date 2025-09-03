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

func checkCapiKubeconfigCommand(checkCapiKubeconfigFlags *flag.FlagSet, args []string) error {
	var (
		out            io.Writer
		ptrShouldDebug *string
		ptrKubeconfig  *string
		cmds           = [][]string{
			{
				"oc get ibmpowervscluster -n openshift-cluster-api-guests -o json",
				"jq -r .items[].status.conditions[]",
			},
			{
				"oc get ibmpowervsimage -n openshift-cluster-api-guests -o json",
				"jq -r .items[].status.conditions[]",
			},
			{
				"oc get ibmpowervsmachines -n openshift-cluster-api-guests -o json",
				"jq -r .items[].status.conditions[]",
			},
		}
		err error
	)

	ptrShouldDebug = checkCapiKubeconfigFlags.String("shouldDebug", "false", "Should output debug output")
	ptrKubeconfig = checkCapiKubeconfigFlags.String("kubeconfig", "", "The KUBECONFIG file")

	checkCapiKubeconfigFlags.Parse(args)

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

	for _, twoCmds := range cmds {
		err = runTwoCommands(ptrKubeconfig, twoCmds[0], twoCmds[1])
		if err != nil {
			fmt.Printf("Error: could not run command: %v\n", err)
		}
	}

	if idxCapiOutput := strings.Index(*ptrKubeconfig, ".clusterapi_output"); idxCapiOutput > -1 {
		firstPart := (*ptrKubeconfig)[0:idxCapiOutput]

		err = runSplitCommand([]string{
			"grep",
			"--color",
			"-E",
			"(Cluster API|level=info|level=warning|level=error|Created manifest|InfraReady|PostProvision|[Ff]ound internal [Ii][Pp] for VM from DHCP lease)",
			fmt.Sprintf("%s%s", firstPart, ".openshift_install.log"),
		})
		if err != nil {
			fmt.Printf("Error: could not run command: %v\n", err)
		}
	}

	return nil
}
