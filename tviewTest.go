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
	"math/rand"
	"time"

	"github.com/rivo/tview"
)

func tviewTest() {
	// https://pkg.go.dev/github.com/rivo/tview
	// https://github.com/rivo/tview/wiki/Grid

	newPrimitive := func(text string) tview.Primitive {
		return tview.NewTextView().
			SetTextAlign(tview.AlignCenter).
			SetText(text)
	}

	app := tview.NewApplication()

	grid := tview.NewGrid().SetBorders(true)

	var windows = make([]tview.Primitive, 8)

	for i := 0; i < 8; i++ {
		windows[i] = newPrimitive(fmt.Sprintf("Row%d", i+1))
		grid.AddItem(windows[i], i+1, 0, 1, 1, 0, 0, false)
		textView, ok := windows[i].(*tview.TextView)
		if ok {
			textView.SetChangedFunc(func() {
				app.Draw()
			})
		} else {
			log.Debugf("!OK(%d) SetChangedFunc", i)
		}
	}

	go func(windows []tview.Primitive) {
		for true {
			windowNumber := rand.Intn(8)
			windowText := fmt.Sprintf("Row%d = %d", windowNumber+1, rand.Intn(100))
			textView, ok := windows[windowNumber].(*tview.TextView)
			if ok {
				textView.SetText(windowText)
			} else {
				log.Debugf("!OK(%d) SetText", windowNumber)
			}
			time.Sleep(1 * time.Second)
		}
	}(windows)

//	time.Sleep(10 * time.Second)

	if err := app.SetRoot(grid, true).SetFocus(grid).Run(); err != nil {
		panic(err)
	}
}
