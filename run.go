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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func runCommand(ptrKubeconfig *string, cmdline string) error {
	var (
		acmdline []string
		ctx      context.Context
		cancel   context.CancelFunc
		cmd      *exec.Cmd
		out      []byte
		err      error
	)

	ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	// Split the space separated line into an array of strings
	acmdline = strings.Fields(cmdline)

	if len(acmdline) == 0 {
		return fmt.Errorf("runCommand has empty command")
	} else if len(acmdline) == 1 {
		cmd = exec.CommandContext(ctx, acmdline[0])
	} else {
		cmd = exec.CommandContext(ctx, acmdline[0], acmdline[1:]...)
	}

	cmd.Env = append(
		os.Environ(),
		fmt.Sprintf("KUBECONFIG=%s", *ptrKubeconfig),
	)

	fmt.Println("8<--------8<--------8<--------8<--------8<--------8<--------8<--------8<--------")
	fmt.Println(cmdline)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))

	return err
}

func runSplitCommand(acmdline []string) error {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		cmd    *exec.Cmd
		out    []byte
		err    error
	)

	ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	if len(acmdline) == 0 {
		return fmt.Errorf("runSplitCommand has empty command")
	} else if len(acmdline) == 1 {
		cmd = exec.CommandContext(ctx, acmdline[0])
	} else {
		cmd = exec.CommandContext(ctx, acmdline[0], acmdline[1:]...)
	}

	fmt.Println("8<--------8<--------8<--------8<--------8<--------8<--------8<--------8<--------")
	fmt.Println(acmdline)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))

	return err
}

func runSplitCommandJson(ptrKubeconfig *string, acmdline []string) (map[string]interface{}, error) {
	var (
		ctx      context.Context
		cancel   context.CancelFunc
		cmd      *exec.Cmd
		out      []byte
		jsonData map[string]interface{}
		err      error
	)

	ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	if len(acmdline) == 0 {
		return nil, fmt.Errorf("runSplitCommandJson has empty command")
	} else if len(acmdline) == 1 {
		cmd = exec.CommandContext(ctx, acmdline[0])
	} else {
		cmd = exec.CommandContext(ctx, acmdline[0], acmdline[1:]...)
	}

	cmd.Env = append(
		os.Environ(),
		fmt.Sprintf("KUBECONFIG=%s", *ptrKubeconfig),
	)

//	log.Debugf("Running: %+v", acmdline)
	out, err = cmd.CombinedOutput()
//	fmt.Println("out:")
//	fmt.Printf("%s", string(out))

	err = json.Unmarshal(out, &jsonData)
	if err != nil {
	}
//	log.Debugf("jsonData = %+v", jsonData)

	return jsonData, err
}

func runTwoCommands(ptrKubeconfig *string, cmdline1 string, cmdline2 string) error {
	var (
		acmdline1 []string
		acmdline2 []string
		ctx       context.Context
		cancel    context.CancelFunc
		cmd1      *exec.Cmd
		cmd2      *exec.Cmd
		readPipe  *os.File
		writePipe *os.File
		buffer    bytes.Buffer
		out       []byte
		err       error
	)

	ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	log.Debugf("cmdline1 = %s", cmdline1)
	log.Debugf("cmdline2 = %s", cmdline2)

	// Split the space separated line into an array of strings
	acmdline1 = strings.Fields(cmdline1)
	acmdline2 = strings.Fields(cmdline2)

	if len(acmdline1) == 0 {
		return fmt.Errorf("runTwoCommands has empty command")
	} else if len(acmdline1) == 1 {
		cmd1 = exec.CommandContext(ctx, acmdline1[0])
	} else {
		cmd1 = exec.CommandContext(ctx, acmdline1[0], acmdline1[1:]...)
	}

	cmd1.Env = append(
		os.Environ(),
		fmt.Sprintf("KUBECONFIG=%s", *ptrKubeconfig),
	)

	if len(acmdline2) == 0 {
		return fmt.Errorf("runTwoCommands has empty command")
	} else if len(acmdline2) == 1 {
		cmd2 = exec.CommandContext(ctx, acmdline2[0])
	} else {
		cmd2 = exec.CommandContext(ctx, acmdline2[0], acmdline2[1:]...)
	}

	readPipe, writePipe, err = os.Pipe()
	if err != nil {
		return fmt.Errorf("Error returned from os.Pipe: %v", err)
	}

	defer readPipe.Close()

	cmd1.Stdout = writePipe

	err = cmd1.Start()
	if err != nil {
		return fmt.Errorf("Error returned from cmd1.Start: %v", err)
	}

	defer cmd1.Wait()

	writePipe.Close()

	cmd2.Stdin = readPipe
	cmd2.Stdout = &buffer
	cmd2.Stderr = &buffer

	cmd2.Run()

	out = buffer.Bytes()

	fmt.Println("8<--------8<--------8<--------8<--------8<--------8<--------8<--------8<--------")
	fmt.Printf("%s | %s\n", cmdline1, cmdline2)
	fmt.Println(string(out))

	return nil
}

func convertMap(mapFrom map[string]interface{}) (map[string]any, error) {
	var (
		mapTo    map[string]any
		newValue any
		err      error
	)

	mapTo = make(map[string]any)

	for k, v := range mapFrom {
		newValue, err = convertValue(v)
		if err != nil {
			return nil, err
		}
		mapTo[k] = newValue
	}
//	log.Debugf("convertMap: DONE!")

	return mapTo, nil
}

func convertValue(v any) (any, error) {
//	log.Debugf("convertValue: v type %T = %+v", v, v)
	switch value := v.(type) {
	case map[string]any:
		convertedValue, err := convertMap(value)
		return convertedValue, err
	case []interface{}:
		newArray := make([]any, 0)
		for _, elm := range value {
			convertedValue, err := convertValue(elm)
			if err != nil {
				return nil, err
			}
			newArray = append(newArray, convertedValue)
		}
		return newArray, nil
	case string, bool, float64:
		return value, nil
	default:
		log.Debugf("convertValue: unhandled type %T", v)
		return nil, fmt.Errorf("")
	}
}
