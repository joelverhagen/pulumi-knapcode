// Copyright 2016-2020, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"time"

	"github.com/pulumi/pulumi/pkg/v2/resource/provider"
	"github.com/pulumi/pulumi/sdk/v2/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v2/go/common/resource/plugin"
	logger "github.com/pulumi/pulumi/sdk/v2/go/common/util/logging"
	rpc "github.com/pulumi/pulumi/sdk/v2/proto/go"

	pbempty "github.com/golang/protobuf/ptypes/empty"
)

type xyzProvider struct {
	host    *provider.HostClient
	name    string
	version string
}

func makeProvider(host *provider.HostClient, name, version string) (rpc.ResourceProviderServer, error) {
	// Return the new provider
	return &xyzProvider{
		host:    host,
		name:    name,
		version: version,
	}, nil
}

// CheckConfig validates the configuration for this provider.
func (k *xyzProvider) CheckConfig(ctx context.Context, req *rpc.CheckRequest) (*rpc.CheckResponse, error) {
	return &rpc.CheckResponse{Inputs: req.GetNews()}, nil
}

// DiffConfig diffs the configuration for this provider.
func (k *xyzProvider) DiffConfig(ctx context.Context, req *rpc.DiffRequest) (*rpc.DiffResponse, error) {
	return &rpc.DiffResponse{}, nil
}

// Configure configures the resource provider with "globals" that control its behavior.
func (k *xyzProvider) Configure(_ context.Context, req *rpc.ConfigureRequest) (*rpc.ConfigureResponse, error) {
	return &rpc.ConfigureResponse{}, nil
}

// Invoke dynamically executes a built-in function in the provider.
func (k *xyzProvider) Invoke(_ context.Context, req *rpc.InvokeRequest) (*rpc.InvokeResponse, error) {
	tok := req.GetTok()
	return nil, fmt.Errorf("Unknown Invoke token '%s'", tok)
}

// StreamInvoke dynamically executes a built-in function in the provider. The result is streamed
// back as a series of messages.
func (k *xyzProvider) StreamInvoke(req *rpc.InvokeRequest, server rpc.ResourceProvider_StreamInvokeServer) error {
	tok := req.GetTok()
	return fmt.Errorf("Unknown StreamInvoke token '%s'", tok)
}

// Check validates that the given property bag is valid for a resource of the given type and returns
// the inputs that should be passed to successive calls to Diff, Create, or Update for this
// resource. As a rule, the provider inputs returned by a call to Check should preserve the original
// representation of the properties as present in the program inputs. Though this rule is not
// required for correctness, violations thereof can negatively impact the end-user experience, as
// the provider inputs are using for detecting and rendering diffs.
func (k *xyzProvider) Check(ctx context.Context, req *rpc.CheckRequest) (*rpc.CheckResponse, error) {
	urn := resource.URN(req.GetUrn())
	ty := urn.Type()

	switch ty {

	case "xyz:index:PrepareAppForWebSignIn":

	default:
		return nil, fmt.Errorf("Check: unknown resource type '%s'", ty)

	}

	return &rpc.CheckResponse{Inputs: req.News, Failures: nil}, nil
}

// Diff checks what impacts a hypothetical update will have on the resource's properties.
func (k *xyzProvider) Diff(ctx context.Context, req *rpc.DiffRequest) (*rpc.DiffResponse, error) {
	urn := resource.URN(req.GetUrn())
	ty := urn.Type()

	olds, err := plugin.UnmarshalProperties(req.GetOlds(), plugin.MarshalOptions{KeepUnknowns: true, SkipNulls: true})
	if err != nil {
		return nil, err
	}

	news, err := plugin.UnmarshalProperties(req.GetNews(), plugin.MarshalOptions{KeepUnknowns: true, SkipNulls: true})
	if err != nil {
		return nil, err
	}

	var replaces []string
	var changes rpc.DiffResponse_DiffChanges

	switch ty {

	case "xyz:index:PrepareAppForWebSignIn":
		d := olds.Diff(news)
		if d == nil {
			changes = rpc.DiffResponse_DIFF_NONE
			replaces = []string{}
		} else {
			if d.Changed("objectId") {
				changes = rpc.DiffResponse_DIFF_SOME
				replaces = append(replaces, "objectId")
			}
			if d.Changed("hostName") {
				changes = rpc.DiffResponse_DIFF_SOME
				replaces = append(replaces, "hostName")
			}
		}

	default:
		return nil, fmt.Errorf("Diff: unknown resource type '%s'", ty)

	}

	return &rpc.DiffResponse{
		Changes:  changes,
		Replaces: replaces,
	}, nil
}

type aadAppUpdateAPI struct {
	RequestAccessTokenVersion int `json:"requestedAccessTokenVersion"`
}

type aadAppUpdateWeb struct {
	HomePageURL  string   `json:"homePageUrl"`
	RedirectUris []string `json:"redirectUris"`
	LogoutURL    string   `json:"logoutUrl"`
}

type aadAppUpdate struct {
	API            aadAppUpdateAPI `json:"api"`
	SignInAudience string          `json:"signInAudience"`
	Web            aadAppUpdateWeb `json:"web"`
}

// Create allocates a new instance of the provided resource and returns its unique ID afterwards.
func (k *xyzProvider) Create(ctx context.Context, req *rpc.CreateRequest) (*rpc.CreateResponse, error) {
	urn := resource.URN(req.GetUrn())
	ty := urn.Type()

	inputs, err := plugin.UnmarshalProperties(req.GetProperties(), plugin.MarshalOptions{KeepUnknowns: true, SkipNulls: true})
	if err != nil {
		return nil, err
	}

	var outputs map[string]interface{}
	var result string

	switch ty {

	case "xyz:index:PrepareAppForWebSignIn":
		if !inputs["objectId"].IsString() {
			return nil, fmt.Errorf("Expected input property 'objectId' of type 'string' but got '%s", inputs["string"].TypeString())
		}

		if !inputs["hostName"].IsString() {
			return nil, fmt.Errorf("Expected input property 'hostName' of type 'string' but got '%s", inputs["string"].TypeString())
		}

		objectID := inputs["objectId"].StringValue()

		err = waitForApp(objectID, true)

		if err != nil {
			return nil, err
		}

		hostName := inputs["hostName"].StringValue()
		result = objectID

		jsonBytes, err := json.Marshal(aadAppUpdate{
			API: aadAppUpdateAPI{
				RequestAccessTokenVersion: 2,
			},
			SignInAudience: "AzureADandPersonalMicrosoftAccount",
			Web: aadAppUpdateWeb{
				HomePageURL: fmt.Sprintf("https://%s", hostName),
				RedirectUris: []string{
					fmt.Sprintf("https://%s/signin-oidc", hostName),
				},
				LogoutURL: fmt.Sprintf("https://%s/signout-oidc", hostName),
			},
		})

		if err != nil {
			return nil, err
		}

		err = execute("az", "rest",
			"--method", "PATCH",
			"--headers", "Content-Type=application/json",
			"--uri", fmt.Sprintf("https://graph.microsoft.com/v1.0/applications/%s", objectID),
			"--body", string(jsonBytes),
			"--verbose")

		if err != nil {
			return nil, err
		}

		outputs = map[string]interface{}{
			"objectId": objectID,
			"hostName": hostName,
		}

	default:
		return nil, fmt.Errorf("Create: unknown resource type '%s'", ty)

	}

	outputProperties, err := plugin.MarshalProperties(
		resource.NewPropertyMapFromMap(outputs),
		plugin.MarshalOptions{KeepUnknowns: true, SkipNulls: true},
	)

	if err != nil {
		return nil, err
	}
	return &rpc.CreateResponse{
		Id:         result,
		Properties: outputProperties,
	}, nil
}

// Read the current live state associated with a resource.
func (k *xyzProvider) Read(ctx context.Context, req *rpc.ReadRequest) (*rpc.ReadResponse, error) {
	panic("Read not implemented.")
}

// Update updates an existing resource with new values.
func (k *xyzProvider) Update(ctx context.Context, req *rpc.UpdateRequest) (*rpc.UpdateResponse, error) {
	panic("Update not implemented")
}

// Delete tears down an existing resource with the given ID.  If it fails, the resource is assumed
// to still exist.
func (k *xyzProvider) Delete(ctx context.Context, req *rpc.DeleteRequest) (*pbempty.Empty, error) {
	urn := resource.URN(req.GetUrn())
	ty := urn.Type()

	inputs, err := plugin.UnmarshalProperties(req.GetProperties(), plugin.MarshalOptions{KeepUnknowns: true, SkipNulls: true})
	if err != nil {
		return nil, err
	}

	switch ty {

	case "xyz:index:PrepareAppForWebSignIn":
		if !inputs["objectId"].IsString() {
			return nil, fmt.Errorf("Expected input property 'objectId' of type 'string' but got '%s", inputs["string"].TypeString())
		}

		objectID := inputs["objectId"].StringValue()

		err = execute("az", "rest",
			"--method", "DELETE",
			"--headers", "Content-Type=application/json",
			"--uri", fmt.Sprintf("https://graph.microsoft.com/v1.0/applications/%s", objectID),
			"--verbose")

		if err != nil {
			return nil, err
		}

		err = waitForApp(objectID, false)

		if err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("Delete: unknown resource type '%s'", ty)

	}

	return &pbempty.Empty{}, nil
}

// Construct creates a new component resource.
func (k *xyzProvider) Construct(_ context.Context, _ *rpc.ConstructRequest) (*rpc.ConstructResponse, error) {
	panic("Construct not implemented")
}

// GetPluginInfo returns generic information about this plugin, like its version.
func (k *xyzProvider) GetPluginInfo(context.Context, *pbempty.Empty) (*rpc.PluginInfo, error) {
	return &rpc.PluginInfo{
		Version: k.version,
	}, nil
}

// GetSchema returns the JSON-serialized schema for the provider.
func (k *xyzProvider) GetSchema(ctx context.Context, req *rpc.GetSchemaRequest) (*rpc.GetSchemaResponse, error) {
	return &rpc.GetSchemaResponse{}, nil
}

// Cancel signals the provider to gracefully shut down and abort any ongoing resource operations.
// Operations aborted in this way will return an error (e.g., `Update` and `Create` will either a
// creation error or an initialization error). Since Cancel is advisory and non-blocking, it is up
// to the host to decide how long to wait after Cancel is called before (e.g.)
// hard-closing any gRPC connection.
func (k *xyzProvider) Cancel(context.Context, *pbempty.Empty) (*pbempty.Empty, error) {
	// TODO
	return &pbempty.Empty{}, nil
}

func waitForApp(objectID string, waitForAvailable bool) error {

	// Poll for the application to become available.
	attempt := 0
	for true {
		attempt++

		err := execute("az", "rest",
			"--method", "GET",
			"--uri", fmt.Sprintf("https://graph.microsoft.com/v1.0/applications/%s", objectID),
			"--query", "id")

		notFound := false

		if err != nil {
			matches, regexpErr := regexp.MatchString("(?i)Not ?Found", err.Error())
			// If the command returned and error and it was not a 404, fail.
			if regexpErr != nil && !matches {
				return err
			}

			notFound = true
		}

		if waitForAvailable != notFound {
			return nil
		}

		if attempt < 30 {
			time.Sleep(1 * time.Second)
		} else if err != nil {
			return err
		} else {
			break
		}
	}

	return fmt.Errorf("application with object ID %s could not be found", objectID)
}

func execute(name string, arg ...string) error {
	cmd := exec.Command(name, arg...)

	logger.V(9).Infof("Executing command: %v", cmd.Args)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	logger.V(9).Infof("stdout: %v", stdout.String())
	logger.V(9).Infof("stderr: %v", stderr.String())

	if err != nil {
		logger.V(9).Infof("err: %v", err)
		err = fmt.Errorf("%s failed with %v\n%v", name, err, stderr.String())
	}

	return err
}
