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
)

// Since there is no API to query these, we have to hard-code them here.

// Region describes resources associated with a region in Power VS.
// We're using a few items from the IBM Cloud VPC offering. The region names
// for VPC are different so another function of this is to correlate those.
type Region struct {
	Description string
	VPCRegion   string
	COSRegion   string
	Zones       map[string]Zone
	VPCZones    []string
}

// Zone holds the sysTypes for a zone in a IBM Power VS region.
type Zone struct {
	SysTypes []string
}

// Regions holds the regions for IBM Power VS, and descriptions used during the survey.
var Regions = map[string]Region{
	"dal": {
		Description: "Dallas, USA",
		VPCRegion:   "us-south",
		COSRegion:   "us-south",
		Zones: map[string]Zone{
			"dal10": {
				SysTypes: []string{"s922", "s1022", "e980", "e1080"},
			},
			"dal12": {
				SysTypes: []string{"s922", "e980"},
			},
		},
		VPCZones: []string{"us-south-1", "us-south-2", "us-south-3"},
	},
	"eu-de": {
		Description: "Frankfurt, Germany",
		VPCRegion:   "eu-de",
		COSRegion:   "eu-de",
		Zones: map[string]Zone{
			"eu-de-1": {
				SysTypes: []string{"s922", "s1022", "e980"},
			},
			"eu-de-2": {
				SysTypes: []string{"s922", "e980"},
			},
		},
		VPCZones: []string{"eu-de-1", "eu-de-2", "eu-de-3"},
	},
	"lon": {
		Description: "London, UK",
		VPCRegion:   "eu-gb",
		COSRegion:   "eu-gb",
		Zones: map[string]Zone{
			"lon06": {
				SysTypes: []string{"s922", "e980"},
			},
		},
		VPCZones: []string{"eu-gb-1", "eu-gb-2", "eu-gb-3"},
	},
	"mad": {
		Description: "Madrid, Spain",
		VPCRegion:   "eu-es",
		COSRegion:   "eu-de", // @HACK - PowerVS says COS not supported in this region
		Zones: map[string]Zone{
			"mad02": {
				SysTypes: []string{"s922", "s1022", "e980"},
			},
			"mad04": {
				SysTypes: []string{"s1022", "e980", "e1080"},
			},
		},
		VPCZones: []string{"eu-es-1", "eu-es-2"},
	},
	"osa": {
		Description: "Osaka, Japan",
		VPCRegion:   "jp-osa",
		COSRegion:   "jp-osa",
		Zones: map[string]Zone{
			"osa21": {
				SysTypes: []string{"s922", "s1022", "e980"},
			},
		},
		VPCZones: []string{"jp-osa-1", "jp-osa-2", "jp-osa-3"},
	},
	"sao": {
		Description: "São Paulo, Brazil",
		VPCRegion:   "br-sao",
		COSRegion:   "br-sao",
		Zones: map[string]Zone{
			"sao01": {
				SysTypes: []string{"s922", "e980"},
			},
			"sao04": {
				SysTypes: []string{"s922", "e980"},
			},
		},
		VPCZones: []string{"br-sao-1", "br-sao-2", "br-sao-3"},
	},
	"syd": {
		Description: "Sydney, Australia",
		VPCRegion:   "au-syd",
		COSRegion:   "au-syd",
		Zones: map[string]Zone{
			"syd04": {
				SysTypes: []string{"s922", "e980"},
			},
			"syd05": {
				SysTypes: []string{"s922", "e980"},
			},
		},
		VPCZones: []string{"au-syd-1", "au-syd-2", "au-syd-3"},
	},
	"tor": {
		Description: "Toronto, Canada",
		VPCRegion:   "ca-tor",
		COSRegion:   "ca-tor",
		Zones: map[string]Zone{
			"tor01": {
				SysTypes: []string{"s922", "e980"},
			},
		},
		VPCZones: []string{"ca-tor-1", "ca-tor-2", "ca-tor-3"},
	},
	"us-east": {
		Description: "Washington DC, USA",
		VPCRegion:   "us-east",
		COSRegion:   "us-east",
		Zones: map[string]Zone{
			"us-east": {
				SysTypes: []string{"s922", "e980"},
			},
		},
		VPCZones: []string{"us-east-1", "us-east-2", "us-east-3"},
	},
	"us-south": {
		Description: "Dallas, USA",
		VPCRegion:   "us-south",
		COSRegion:   "us-south",
		Zones: map[string]Zone{
			"us-south": {
				SysTypes: []string{"s922", "e980"},
			},
		},
		VPCZones: []string{"us-south-1", "us-south-2", "us-south-3"},
	},
	"wdc": {
		Description: "Washington DC, USA",
		VPCRegion:   "us-east",
		COSRegion:   "us-east",
		Zones: map[string]Zone{
			"wdc06": {
				SysTypes: []string{"s922", "e980"},
			},
			"wdc07": {
				SysTypes: []string{"s922", "s1022", "e980", "e1080"},
			},
		},
		VPCZones: []string{"us-east-1", "us-east-2", "us-east-3"},
	},
}

// VPCRegionForPowerVSRegion returns the VPC region for the specified PowerVS region.
func VPCRegionForPowerVSRegion(region string) (string, error) {
	if r, ok := Regions[region]; ok {
		return r.VPCRegion, nil
	}

	return "", fmt.Errorf("VPC region corresponding to a PowerVS region %s not found ", region)
}
