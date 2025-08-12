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
	priority := func (ro RunnableObject) int {
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
