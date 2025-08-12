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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"

	configv1 "github.com/openshift/api/config/v1"
)

type Metadata struct {
	ciMode         bool
	createMetadata CreateMetadata
	ciMetadata     CIMetadata
}

type CreateMetadata struct {
	ClusterName             string `json:"clusterName"`
	ClusterID               string `json:"clusterID"`
	InfraID                 string `json:"infraID"`
	ClusterPlatformMetadata `json:",inline"`
	FeatureSet              configv1.FeatureSet          `json:"featureSet"`
	CustomFeatureSet        *configv1.CustomFeatureGates `json:"customFeatureSet"`
}

type ClusterPlatformMetadata struct {
	PowerVS *PowerVSMetadata `json:"powervs,omitempty"`
}

type PowerVSMetadata struct {
	BaseDomain           string                            `json:"BaseDomain"`
	CISInstanceCRN       string                            `json:"cisInstanceCRN"`
	DNSInstanceCRN       string                            `json:"dnsInstanceCRN"`
	PowerVSResourceGroup string                            `json:"powerVSResourceGroup"`
	Region               string                            `json:"region"`
	VPCRegion            string                            `json:"vpcRegion"`
	Zone                 string                            `json:"zone"`
	ServiceInstanceGUID  string                            `json:"serviceInstanceGUID"`
	ServiceEndpoints     []configv1.PowerVSServiceEndpoint `json:"serviceEndpoints,omitempty"`
	TransitGateway       string                            `json:"transitGatewayName"`
	// Only in release-4.20
	VPC string `json:"vpcName"`
}

// {"region": "", "vpcRegion": "", "zone": "", "resourceGroup": "", "serviceInstance": "", "vpc": "", "transitGateway": ""}
type CIMetadata struct {
	Region          string `json:"region"`
	VPCRegion       string `json:"vpcRegion"`
	Zone            string `json:"zone"`
	ResourceGroup   string `json:"resourceGroup"`
	ServiceInstance string `json:"serviceInstance"`
	Vpc             string `json:"vpc"`
	TransitGateway  string `json:"transitGateway"`
}

func NewMetadataFromCCMetadata(filename string) (*Metadata, error) {
	var (
		content  []byte
		metadata Metadata
		err      error
	)

	content, err = ioutil.ReadFile(filename)
	if err != nil {
		log.Fatal("Error when opening file: ", err)
		return nil, err
	}

	log.Debugf("NewMetadataFromCCMetadata: content = %s", string(content))

	err = json.Unmarshal(content, &metadata.createMetadata)
	if err != nil {
		log.Fatal("Error during Unmarshal(): ", err)
		return nil, err
	}

	metadata.ciMode = false

	log.Debugf("NewMetadataFromCCMetadata: metadata = %+v", metadata)
	log.Debugf("NewMetadataFromCCMetadata: metadata.createMetadata = %+v", metadata.createMetadata)
	log.Debugf("NewMetadataFromCCMetadata: metadata.createMetadata.PowerVS = %+v", metadata.createMetadata.PowerVS)

	return &metadata, nil
}

func NewMetadataFromCIMetadata(filename string) (*Metadata, error) {
	var (
		content  []byte
		metadata Metadata
		err      error
	)

	content, err = ioutil.ReadFile(filename)
	if err != nil {
		log.Fatal("Error when opening file: ", err)
		return nil, err
	}

	log.Debugf("NewMetadataFromCIMetadata: content = %s", string(content))

	err = json.Unmarshal(content, &metadata.ciMetadata)
	if err != nil {
		log.Fatal("Error during Unmarshal(): ", err)
		return nil, err
	}

	metadata.ciMode = true

	log.Debugf("NewMetadataFromCIMetadata: metadata = %+v", metadata)
	log.Debugf("NewMetadataFromCIMetadata: metadata.ciMetadata = %+v", metadata.ciMetadata)

	metadata.createMetadata.PowerVS = &PowerVSMetadata{
		Region:               metadata.ciMetadata.Region,
		VPCRegion:            metadata.ciMetadata.VPCRegion,
		Zone:                 metadata.ciMetadata.Zone,
		PowerVSResourceGroup: metadata.ciMetadata.ResourceGroup,
	}

	return &metadata, nil
}

func (m *Metadata) GetObjectName(ro RunnableObject) (string, error) {
	if m.ciMode {
		return m.GetCIObjectName(ro)
	} else {
		return m.GetClusterObjectName(ro)
	}
}

func (m *Metadata) GetCIObjectName(ro RunnableObject) (string, error) {
	log.Debugf("GetCIObjectName: TypeOf = %s", reflect.TypeOf(ro).String())

	switch reflect.TypeOf(ro).String() {
	case "*main.ServiceInstance":
		return m.ciMetadata.ServiceInstance, nil
	case "*main.Vpc":
		return m.ciMetadata.Vpc, nil
	case "*main.TransitGateway":
		return m.ciMetadata.TransitGateway, nil
	}
	return "", fmt.Errorf("Unknown RunnableObject %+v", ro)
}

func (m *Metadata) GetClusterObjectName(ro RunnableObject) (string, error) {
	var (
		name string
	)

	log.Debugf("GetClusterObjectName: TypeOf = %s", reflect.TypeOf(ro).String())

	switch reflect.TypeOf(ro).String() {
	case "*main.CloudObjectStorage":
		return fmt.Sprintf("%s-cos", m.GetInfraID()), nil
	case "*main.LoadBalancer":
		return m.GetClusterName(), nil
	case "*main.ServiceInstance":
		return m.GetClusterName(), nil
	case "*main.Vpc":
		if m.GetVpcName() != "" {
			name = m.GetVpcName()
		} else {
			name = fmt.Sprintf("vpc-%s", m.GetClusterName())
		}
		return name, nil
	case "*main.VpcInstance":
		return m.GetClusterName(), nil
	case "*main.TransitGateway":
		if m.GetTransitGatewayName() != "" {
			name = m.GetTransitGatewayName()
		} else {
			name = fmt.Sprintf("%s-tg", m.GetInfraID())
		}
		return name, nil
	}
	return "", fmt.Errorf("Unknown RunnableObject %+v", ro)
}

func (m *Metadata) GetClusterName() string {
	return m.createMetadata.ClusterName
}

func (m *Metadata) GetInfraID() string {
	return m.createMetadata.InfraID
}

func (m *Metadata) GetBaseDomain() string {
	return m.createMetadata.PowerVS.BaseDomain
}

func (m *Metadata) GetCISInstanceCRN() string {
	return m.createMetadata.PowerVS.CISInstanceCRN
}

func (m *Metadata) GetRegion() string {
	return m.createMetadata.PowerVS.Region
}

func (m *Metadata) GetVPCRegion() (string, error) {
	var (
		vpcRegion string
		err       error
	)

	vpcRegion = m.createMetadata.PowerVS.VPCRegion

	if vpcRegion == "" {
		vpcRegion, err = VPCRegionForPowerVSRegion(m.createMetadata.PowerVS.Region)
		if err != nil {
			return "", err
		}

		m.createMetadata.PowerVS.VPCRegion = vpcRegion
	}

	return vpcRegion, nil
}

func (m *Metadata) GetZone() string {
	return m.createMetadata.PowerVS.Zone
}

func (m *Metadata) GetResourceGroup() string {
	return m.createMetadata.PowerVS.PowerVSResourceGroup
}

func (m *Metadata) GetServiceInstanceGUID() string {
	return m.createMetadata.PowerVS.ServiceInstanceGUID
}

func (m *Metadata) GetTransitGatewayName() string {
	return m.createMetadata.PowerVS.TransitGateway
}

func (m *Metadata) GetVpcName() string {
	return m.createMetadata.PowerVS.VPC
}
