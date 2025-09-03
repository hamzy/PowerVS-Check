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
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

var (
	// Replaced with:
	//   -ldflags="-X main.version=$(git describe --always --long --dirty)"
	version = "undefined"
	release = "undefined"

	shouldDebug  = false
	shouldDelete = false

	log *logrus.Logger
)

func printUsage(executableName string) {
	fmt.Fprintf(os.Stderr, "Usage: %s [ "+
		"check-ci | "+
		"check-create | "+
		"check-kubeconfig | "+
		"check-capi-kubeconfig | "+
		"create-jumpbox | "+
		"watch-create "+
		"]\n", executableName)
}

func main() {
	var (
		executableName           string
		checkCiFlags             *flag.FlagSet
		checkCreateFlags         *flag.FlagSet
		checkKubeconfigFlags     *flag.FlagSet
		checkCapiKubeconfigFlags *flag.FlagSet
		createJumpboxFlags       *flag.FlagSet
		watchCreateClusterFlags  *flag.FlagSet
		err                      error
	)

	executablePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting executable path: %v\n", err)
		os.Exit(1)
	}

	executableName = filepath.Base(executablePath)

	if len(os.Args) == 1 {
		printUsage(executableName)
		os.Exit(1)
	} else if len(os.Args) == 2 && os.Args[1] == "-version" {
		fmt.Fprintf(os.Stderr, "version = %v\nrelease = %v\n", version, release)
		os.Exit(1)
	}

	checkCiFlags = flag.NewFlagSet("check-ci", flag.ExitOnError)
	checkCreateFlags = flag.NewFlagSet("check-create", flag.ExitOnError)
	checkKubeconfigFlags = flag.NewFlagSet("check-kubeconfig", flag.ExitOnError)
	checkCapiKubeconfigFlags = flag.NewFlagSet("check-capi-kubeconfig", flag.ExitOnError)
	createJumpboxFlags = flag.NewFlagSet("create-jumpbox", flag.ExitOnError)
	watchCreateClusterFlags = flag.NewFlagSet("watch-create", flag.ExitOnError)

	switch strings.ToLower(os.Args[1]) {
	case "check-ci":
		err = checkCiCommand(checkCiFlags, os.Args[2:])

	case "check-create":
		err = checkCreateCommand(checkCreateFlags, os.Args[2:])

	case "check-kubeconfig":
		err = checkKubeconfigCommand(checkKubeconfigFlags, os.Args[2:])

	case "check-capi-kubeconfig":
		err = checkCapiKubeconfigCommand(checkCapiKubeconfigFlags, os.Args[2:])

	case "create-jumpbox":
		err = createJumpboxCommand(createJumpboxFlags, os.Args[2:])

	case "watch-create":
		err = watchCreateCommand(watchCreateClusterFlags, os.Args[2:])

	default:
		fmt.Fprintf(os.Stderr, "Error: Unknown command %s\n", os.Args[1])
		printUsage(executableName)
		os.Exit(1)
	}

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
