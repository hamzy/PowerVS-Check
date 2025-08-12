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
	"context"
	"fmt"
	gohttp "net/http"
	"strings"
	"time"

	"github.com/IBM/go-sdk-core/v5/core"

	"github.com/IBM/networking-go-sdk/transitgatewayapisv1"

	"github.com/IBM/platform-services-go-sdk/resourcecontrollerv2"
	"github.com/IBM/platform-services-go-sdk/resourcemanagerv2"

	"github.com/IBM/vpc-go-sdk/vpcv1"

	"github.com/IBM-Cloud/bluemix-go"
	"github.com/IBM-Cloud/bluemix-go/authentication"
	"github.com/IBM-Cloud/bluemix-go/http"
	"github.com/IBM-Cloud/bluemix-go/rest"
	bxsession "github.com/IBM-Cloud/bluemix-go/session"
	// https://raw.githubusercontent.com/IBM-Cloud/bluemix-go/refs/heads/master/session/session.go

	"github.com/golang-jwt/jwt"
)

var (
	defaultTimeout = 5 * time.Minute
)

type Services struct {
	//
	apiKey         string

	//
	metadata       *Metadata

	//
	bxSession      *bxsession.Session

	//
	user           *User

	// type VpcV1 struct
	vpcSvc         *vpcv1.VpcV1

	// type ResourceControllerV2
	controllerSvc  *resourcecontrollerv2.ResourceControllerV2

	// type TransitGatewayApisV1
	tgClient       *transitgatewayapisv1.TransitGatewayApisV1

	// type ResourceManagerV2
	managementSvc   *resourcemanagerv2.ResourceManagerV2

	//
	ctx             context.Context

	//
	resourceGroupID string
}

type User struct {
	ID         string
	Email      string
	Account    string
	cloudName  string
	cloudType  string
	generation int
}

func NewServices(metadata *Metadata, apiKey string) (*Services, error) {
	var (
		ctx             context.Context
		bxSession       *bxsession.Session
		user            *User
		region          string
		vpcRegion       string
		vpcSvc          *vpcv1.VpcV1
		controllerSvc   *resourcecontrollerv2.ResourceControllerV2
		tgClient        *transitgatewayapisv1.TransitGatewayApisV1
		managementSvc   *resourcemanagerv2.ResourceManagerV2
		services        *Services
		resourceGroupID string
		err             error
	)

	ctx = context.Background()

	bxSession, err = InitBXService(apiKey)
	if err != nil {
		return nil, err
	}
	log.Debugf("NewServices: bxSession = %+v", bxSession)

	user, err = fetchUserDetails(bxSession, 2)
	if err != nil {
		return nil, err
	}

	region = metadata.GetRegion()
	log.Debugf("NewServices: region = %s", region)

	vpcRegion, err = metadata.GetVPCRegion()
	if err != nil {
		return nil, err
	}
	log.Debugf("NewServices: vpcRegion = %s", vpcRegion)

	vpcSvc, err = initVPCService(apiKey, vpcRegion)
	if err != nil {
		return nil, err
	}
	if vpcSvc == nil {
		return nil, fmt.Errorf("NewServices could not create vpcSvc")
	}

	controllerSvc, err = initCloudObjectStorageService(apiKey)
	if err != nil {
		log.Fatalf("Error: NewServices: initCloudObjectStorageService returns %v", err)
		return nil, err
	}
	log.Debugf("NewServices: controllerSvc = %+v", controllerSvc)

	tgClient, err = initTransitGatewayClient (apiKey)
	if err != nil {
		log.Fatalf("Error: NewServices: initTransitGatewayClient returns %v", err)
		return nil, err
	}
	log.Debugf("NewServices: tgClient = %+v", tgClient)

	managementSvc, err = initManagementService(apiKey)
	if err != nil {
		log.Fatalf("Error: NewServices: initManagementService returns %v", err)
		return nil, err
	}
	log.Debugf("NewServices: managementSvc = %+v", managementSvc)

	resourceGroupID = metadata.GetResourceGroup()
	log.Debugf("NewServices: resourceGroupID = %s", resourceGroupID)

	services = &Services{
		apiKey:         apiKey,
		metadata:       metadata,
		bxSession:      bxSession,
		user:           user,
		vpcSvc:         vpcSvc,
		controllerSvc:  controllerSvc,
		tgClient:       tgClient,
		managementSvc:  managementSvc,
		ctx:            ctx,
		resourceGroupID: resourceGroupID,
	}

	resourceGroupID, err = services.ResourceGroupNameToID(resourceGroupID)
	if err != nil {
		log.Fatalf("Error: NewServices: ResourceGroupNameToID returns %v", err)
		return nil, err
	}
	log.Debugf("NewServices: resourceGroupID = %s", resourceGroupID)
	services.resourceGroupID = resourceGroupID

	return services, nil
}

func (svc *Services) GetApiKey() string {
	return svc.apiKey
}

func (svc *Services) GetMetadata() *Metadata {
	return svc.metadata
}

func (svc *Services) GetVpcSvc() *vpcv1.VpcV1 {
	return svc.vpcSvc
}

func (svc *Services) GetControllerSvc() *resourcecontrollerv2.ResourceControllerV2 {
	return svc.controllerSvc
}

func (svc *Services) GetTgClient() *transitgatewayapisv1.TransitGatewayApisV1 {
	return svc.tgClient
}

func (svc *Services) GetUser() *User {
	return svc.user
}

func (svc *Services) GetContextWithTimeout() (context.Context, context.CancelFunc) {
        return context.WithTimeout(svc.ctx, defaultTimeout)
}

func (svc *Services) GetResourceGroupID() string {
	return svc.resourceGroupID
}

func (svc *Services) ResourceGroupNameToID(resourceGroupName string) (string, error) {
	var (
		listResourceGroupsOptions *resourcemanagerv2.ListResourceGroupsOptions
		resourceGroups            *resourcemanagerv2.ResourceGroupList
		err                       error
	)

	listResourceGroupsOptions = svc.managementSvc.NewListResourceGroupsOptions()
	listResourceGroupsOptions.AccountID = &svc.user.Account

	resourceGroups, _, err = svc.managementSvc.ListResourceGroups(listResourceGroupsOptions)
	if err != nil {
		return "", err
	}

	for _, resourceGroup := range resourceGroups.Resources {
		if *resourceGroup.Name == resourceGroupName {
			return *resourceGroup.ID, nil
		}
	}

	return "", fmt.Errorf("resource group name (%s) not found", resourceGroupName)
}

func InitBXService(apiKey string) (*bxsession.Session, error) {
	var (
		bxSession             *bxsession.Session
		tokenProviderEndpoint string = "https://iam.cloud.ibm.com"
		err                   error
	)

	bxSession, err = bxsession.New(&bluemix.Config{
		BluemixAPIKey:         apiKey,
		TokenProviderEndpoint: &tokenProviderEndpoint,
		Debug:                 false,
	})
	if err != nil {
		return nil, fmt.Errorf("Error bxsession.New: %v", err)
	}
	log.Debugf("InitBXService: bxSession = %v", bxSession)

	tokenRefresher, err := authentication.NewIAMAuthRepository(bxSession.Config, &rest.Client{
		DefaultHeader: gohttp.Header{
			"User-Agent": []string{http.UserAgent()},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("Error authentication.NewIAMAuthRepository: %v", err)
	}
	log.Debugf("InitBXService: tokenRefresher = %v", tokenRefresher)

	err = tokenRefresher.AuthenticateAPIKey(bxSession.Config.BluemixAPIKey)
	if err != nil {
		return nil, fmt.Errorf("Error tokenRefresher.AuthenticateAPIKey: %v", err)
	}

	return bxSession, nil
}

func fetchUserDetails(bxSession *bxsession.Session, generation int) (*User, error) {
	var (
		bluemixToken string
	)

	config := bxSession.Config
	user := User{}

	if len(config.IAMAccessToken) == 0 {
		return nil, fmt.Errorf("fetchUserDetails config.IAMAccessToken is empty")
	}

	if strings.HasPrefix(config.IAMAccessToken, "Bearer") {
		bluemixToken = config.IAMAccessToken[7:len(config.IAMAccessToken)]
	} else {
		bluemixToken = config.IAMAccessToken
	}

	token, err := jwt.Parse(bluemixToken, func(token *jwt.Token) (interface{}, error) {
		return "", nil
	})
	if err != nil && !strings.Contains(err.Error(), "key is of invalid type") {
		return &user, err
	}

	claims := token.Claims.(jwt.MapClaims)
	if email, ok := claims["email"]; ok {
		user.Email = email.(string)
	}
	user.ID = claims["id"].(string)
	user.Account = claims["account"].(map[string]interface{})["bss"].(string)
	iss := claims["iss"].(string)
	if strings.Contains(iss, "https://iam.cloud.ibm.com") {
		user.cloudName = "bluemix"
	} else {
		user.cloudName = "staging"
	}
	user.cloudType = "public"
	user.generation = generation

	log.Debugf("fetchUserDetails: user.ID         = %v", user.ID)
	log.Debugf("fetchUserDetails: user.Email      = %v", user.Email)
	log.Debugf("fetchUserDetails: user.Account    = %v", user.Account)
	log.Debugf("fetchUserDetails: user.cloudType  = %v", user.cloudType)
	log.Debugf("fetchUserDetails: user.generation = %v", user.generation)

	return &user, nil
}

func initVPCService(apiKey string, region string) (*vpcv1.VpcV1, error) {
	var (
		authenticator core.Authenticator = &core.IamAuthenticator{
			ApiKey: apiKey,
		}

		// type VpcV1 struct
		vpcSvc *vpcv1.VpcV1

		err error
	)

	// https://raw.githubusercontent.com/IBM/vpc-go-sdk/master/vpcv1/vpc_v1.go
	vpcSvc, err = vpcv1.NewVpcV1(&vpcv1.VpcV1Options{
		Authenticator: authenticator,
		URL:           "https://" + region + ".iaas.cloud.ibm.com/v1",
	})
	log.Debugf("initVPCService: vpc.vpcSvc = %v", vpcSvc)
	if err != nil {
		log.Fatalf("Error: vpcv1.NewVpcV1 returns %v", err)
		return nil, err
	}
	if vpcSvc == nil {
		panic(fmt.Errorf("Error: vpcSvc is empty?"))
	}

	return vpcSvc, nil
}

func initCloudObjectStorageService(apiKey string) (*resourcecontrollerv2.ResourceControllerV2, error) {
	var (
		authenticator core.Authenticator = &core.IamAuthenticator{
			ApiKey: apiKey,
		}
		controllerSvc *resourcecontrollerv2.ResourceControllerV2
		err           error
	)

	controllerSvc, err = resourcecontrollerv2.NewResourceControllerV2(&resourcecontrollerv2.ResourceControllerV2Options{
		Authenticator: authenticator,
	})
	if err != nil {
		log.Fatalf("Error: resourcecontrollerv2.NewResourceControllerV2 returns %v", err)
		return nil, err
	}
	if controllerSvc == nil {
		panic(fmt.Errorf("Error: controllerSvc is empty?"))
	}

	return controllerSvc, nil
}

func initResourceControllerService(apiKey string) (*resourcecontrollerv2.ResourceControllerV2, error) {
	var (
		authenticator core.Authenticator
		controllerSvc *resourcecontrollerv2.ResourceControllerV2
		err           error
	)

	authenticator = &core.IamAuthenticator{
		ApiKey: apiKey,
	}

	err = authenticator.Validate()
	if err != nil {
		return nil, fmt.Errorf("NewServiceInstance: authenticator.Validate: %w", err)
	}

	// Instantiate the service with an API key based IAM authenticator
	controllerSvc, err = resourcecontrollerv2.NewResourceControllerV2(&resourcecontrollerv2.ResourceControllerV2Options{
		Authenticator: authenticator,
		ServiceName:   "cloud-object-storage",
		URL:           "https://resource-controller.cloud.ibm.com",
	})
	if err != nil {
		return nil, fmt.Errorf("NewServiceInstance: creating ControllerV2 Service: %w", err)
	}

	return controllerSvc, nil
}

func initTransitGatewayClient (apiKey string) (*transitgatewayapisv1.TransitGatewayApisV1, error) {
	var (
		authenticator core.Authenticator
		versionDate   = "2023-07-04"
		tgOptions     *transitgatewayapisv1.TransitGatewayApisV1Options
		tgClient      *transitgatewayapisv1.TransitGatewayApisV1
		err           error
	)

	authenticator = &core.IamAuthenticator{
		ApiKey: apiKey,
	}

	err = authenticator.Validate()
	if err != nil {
		return nil, fmt.Errorf("initTransitGatewayClient: authenticator.Validate: %w", err)
	}

	tgOptions = &transitgatewayapisv1.TransitGatewayApisV1Options{
		Authenticator: authenticator,
		Version:       &versionDate,
	}

	tgClient, err = transitgatewayapisv1.NewTransitGatewayApisV1(tgOptions)
	if err != nil {
		return nil, fmt.Errorf("initTransitGatewayClient: NewTransitGatewayApisV1: %w", err)
	}
	log.Debugf("initTransitGatewayClient: tgClient = %+v", tgClient)

	return tgClient, nil
}

func initManagementService(apiKey string) (*resourcemanagerv2.ResourceManagerV2, error) {
	var (
		authenticator core.Authenticator
		options       *resourcemanagerv2.ResourceManagerV2Options
		managementSvc *resourcemanagerv2.ResourceManagerV2
		err           error
	)

	authenticator = &core.IamAuthenticator{
		ApiKey: apiKey,
	}

	options = &resourcemanagerv2.ResourceManagerV2Options{
		Authenticator: authenticator,
	}

	managementSvc, err = resourcemanagerv2.NewResourceManagerV2(options)
	if err != nil {
		return nil, err
	}

	return managementSvc, nil
}
