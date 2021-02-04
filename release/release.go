package release

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/nomad/api"
	"github.com/hashicorp/waypoint-plugin-sdk/terminal"
	"github.com/jeffwecan/waypoint-plugin-nomad-traefik/platform"
)

type ReleaseConfig struct {
	Active bool "hcl:directory,optional"
	// The Nomad region to deploy to, defaults to "global"
	Region string `hcl:"region,optional"`

	// The datacenters to deploy to, defaults to ["dc1"]
	Datacenter string `hcl:"datacenter,optional"`

	// The namespace of the job
	Namespace string `hcl:"namespace,optional"`

	// The number of replicas of the service to maintain. If this number is maintained
	// outside waypoint, do not set this variable.
	Count int `hcl:"replicas,optional"`

	// Environment variables that are meant to configure the application in a static
	// way. This might be control an image that has multiple modes of operation,
	// selected via environment variable. Most configuration should use the waypoint
	// config commands.
	StaticEnvVars map[string]string `hcl:"static_environment,optional"`

	// Port that your service is running on within the actual container.
	// Defaults to port 3000.
	// TODO Evaluate if this should remain as a default 3000, should be a required field,
	// or default to another port.
	ServicePort      uint     `hcl:"service_port,optional"`
	ServicePortLabel string   `hcl:"service_port_label,optional"`
	ServiceTags      []string `hcl:"service_tags,optional"`
}

type ReleaseManager struct {
	config ReleaseConfig
}

// Implement Configurable
func (rm *ReleaseManager) Config() (interface{}, error) {
	return &rm.config, nil
}

// Implement ConfigurableNotify
func (rm *ReleaseManager) ConfigSet(config interface{}) error {
	_, ok := config.(*ReleaseConfig)
	if !ok {
		// The Waypoint SDK should ensure this never gets hit
		return fmt.Errorf("Expected *ReleaseConfig as parameter")
	}

	// validate the config

	return nil
}

// Implement Builder
func (rm *ReleaseManager) ReleaseFunc() interface{} {
	// return a function which will be called by Waypoint
	return rm.Release
}

// A BuildFunc does not have a strict signature, you can define the parameters
// you need based on the Available parameters that the Waypoint SDK provides.
// Waypoint will automatically inject parameters as specified
// in the signature at run time.
//
// Available input parameters:
// - context.Context
// - *component.Source
// - *component.JobInfo
// - *component.DeploymentConfig
// - *datadir.Project
// - *datadir.App
// - *datadir.Component
// - hclog.Logger
// - terminal.UI
// - *component.LabelSet

// In addition to default input parameters the platform.Deployment from the Deploy step
// can also be injected.
//
// The output parameters for ReleaseFunc must be a Struct which can
// be serialzied to Protocol Buffers binary format and an error.
// This Output Value will be made available for other functions
// as an input parameter.
//
// If an error is returned, Waypoint stops the execution flow and
// returns an error to the user.
// func (rm *ReleaseManager) Release(
// 	ctx context.Context,
// 	ui terminal.UI,
// 	d *platform.Deployment,
// ) (*Release, error) {

// 	u := ui.Status()
// 	defer u.Close()

// 	return &Release{}, nil
// }

func (rm *ReleaseManager) Release(
	ctx context.Context,
	ui terminal.UI,
	d *platform.Deployment,
) (*Release, error) {
	ui.Output("Deployment ID: %s, Deployment Name: %s", d.Id, d.Name)

	// We'll update the user in real time
	st := ui.Status()
	defer st.Close()
	shortName := strings.Split(d.Name, "-")[0]
	// Get our client
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		return nil, err
	}
	jobclient := client.Jobs()

	if rm.config.ServicePort == 0 {
		rm.config.ServicePort = 3000
	}

	if rm.config.Datacenter == "" {
		rm.config.Datacenter = "dc1"
	}

	if rm.config.ServicePortLabel == "" {
		rm.config.ServicePortLabel = "waypoint"
	}

	// Determine if we have a job that we manage already
	job, _, err := jobclient.Info(d.Name, &api.QueryOptions{})
	// if strings.Contains(err.Error(), "job not found") {
	// 	job = api.NewServiceJob(d.Name, d.Name, rm.config.Region, 10)
	// 	job.Datacenters = []string{rm.config.Datacenter}
	tg := api.NewTaskGroup(d.Name, 1)
	tg.Networks = []*api.NetworkResource{
		{
			Mode: "host",
			DynamicPorts: []api.Port{
				{
					Label: rm.config.ServicePortLabel,
					To:    int(rm.config.ServicePort),
				},
			},
		},
	}
	checkName := fmt.Sprintf("%s-service-port", shortName)
	checkInterval, _ := time.ParseDuration("30s")
	checkTimeout, _ := time.ParseDuration("30s")
	tg.Services = []*api.Service{
		{
			Name:      shortName,
			PortLabel: rm.config.ServicePortLabel,
			Tags:      rm.config.ServiceTags,
			Checks: []api.ServiceCheck{
				{
					Name:      checkName,
					Type:      "tcp",
					PortLabel: rm.config.ServicePortLabel,
					Interval:  checkInterval,
					Timeout:   checkTimeout,
				},
			},
		},
	}
	job.AddTaskGroup(tg)
	tg.AddTask(&api.Task{
		Name:   d.Name,
		Driver: "docker",
	})
	err = nil
	// }
	if err != nil {
		return nil, err
	}

	// Build our env vars
	env := map[string]string{
		"PORT": fmt.Sprint(rm.config.ServicePort),
	}

	for k, v := range rm.config.StaticEnvVars {
		env[k] = v
	}

	// for k, v := range ReleaseConfig.Env() {
	// 	env[k] = v
	// }

	// If no count is specified, presume that the user is managing the replica
	// count some other way (perhaps manual scaling, perhaps a pod autoscaler).
	// Either way if they don't specify a count, we should be sure we don't send one.
	if rm.config.Count > 0 {
		job.TaskGroups[0].Count = &rm.config.Count
	}

	// // Set our ID on the meta.
	// job.SetMeta(metaId, result.Id)
	// job.SetMeta(metaNonce, time.Now().UTC().Format(time.RFC3339Nano))

	// config := map[string]interface{}{
	// 	"image": img.Name(),
	// 	"ports": []string{"waypoint"},
	// }

	// if rm.config.Auth != nil {
	// 	config["auth"] = map[string]interface{}{
	// 		"username": rm.config.Auth.Username,
	// 		"password": rm.config.Auth.Password,
	// 	}
	// }

	// job.TaskGroups[0].Tasks[0].Config = config
	// job.TaskGroups[0].Tasks[0].Env = env

	// Register job
	st.Update("Registering job...")
	regResult, _, err := jobclient.Register(job, nil)
	if err != nil {
		return nil, err
	}

	evalID := regResult.EvalID
	st.Step(terminal.StatusOK, "Job registration successful")

	// Wait on the allocation
	st.Update(fmt.Sprintf("Monitoring evaluation %q", evalID))

	// if err := newMonitor(st, client).monitor(evalID); err != nil {
	// 	return nil, err
	// }
	st.Step(terminal.StatusOK, "Deployment successfully rolled out!")

	return &Release{}, nil
}
