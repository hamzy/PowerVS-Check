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
	"os"
)

type RunnableObject interface {
	CRN() (string, error)
	Name() (string, error)
	ObjectName() (string, error)
	Run() error
	CiStatus(shouldClean bool)
	ClusterStatus()
	Priority() (int, error)
}

type NewRunnableObject func(*Services) (RunnableObject, error)
type NewRunnableObjects func(*Services) ([]RunnableObject, []error)

type NewRunnableObjectEntry struct {
	NRO  NewRunnableObject
	Name string
}

type NewRunnableObjectsEntry struct {
	NRO  NewRunnableObjects
	Name string
}

func BubbleSort(input []RunnableObject) []RunnableObject {
	priority := func(ro RunnableObject) int {
		p, e := ro.Priority()
		if e != nil {
			return -1
		}
		return p
	}
	swapped := true
	// While we have swapped at least one element...
	for swapped {
		swapped = false
		for i := 1; i < len(input); i++ {
			// Does the next element have a higher priority than this element?
			if priority(input[i]) > priority(input[i-1]) {
				// Then swap them!
				input[i], input[i-1] = input[i-1], input[i]
				swapped = true
			}
		}
	}
	return input
}

func initializeRunnableObjects(services *Services, robjsFuncs []NewRunnableObjectsEntry) ([]RunnableObject, error) {
	var (
		robjsResult    []RunnableObject
		errs           []error
		robjObjectName string
		crnName        string
		robjsCluster   = make([]RunnableObject, 0, 5)
		err            error
	)

	// Loop through New functions which return an array of runnable objects.
	for _, nroe := range robjsFuncs {
		fmt.Fprintf(os.Stderr, "Querying the %s...\n", nroe.Name)

		// Call the New function.
		robjsResult, errs = nroe.NRO(services)

		// Loop through the returned errors.
		for _, err = range errs {
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Could not create a %s object (%v)!\n", nroe.Name, err)
			}
		}

		// Loop through the array of returned results.
		for _, robj := range robjsResult {
			// What is the runnable object's name?
			robjObjectName, err = robj.ObjectName()
			if err != nil {
				return nil, fmt.Errorf("Error: Could not figure out the objects' name! (%s)\n", err)
			}

			// Also make sure the priority is valid.
			_, err = robj.Priority()
			if err != nil {
				return nil, fmt.Errorf("Error: Could not get the priority for %s: %s\n", robjObjectName, err)
			}

			// Append the runnable object.
			log.Debugf("Appending %s %+v", robjObjectName, robj)
			robjsCluster = append(robjsCluster, robj)

			// What is the runnable object's CRN?
			crnName, err = robj.CRN()
			if err == nil {
				log.Debugf("%s.CRN = %s", robjObjectName, crnName)
			} else {
				log.Debugf("ERROR: %s.CRN: %v", robjObjectName, err)
			}
		}
	}

	// Run each object.
	for _, robj := range robjsCluster {
		robjObjectName, _ = robj.ObjectName()
		fmt.Fprintf(os.Stderr, "Running the %s...\n", robjObjectName)

		err = robj.Run()
		if err != nil {
			return nil, err
		}
	}

	return robjsCluster, nil
}
