package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/waypoint-plugin-sdk/component"
	"github.com/hashicorp/waypoint-plugin-sdk/docs"
	"github.com/hashicorp/waypoint-plugin-sdk/terminal"
	"github.com/hashicorp/waypoint/builtin/docker"

	"github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/jobspec2"
)

const (
	metaId    = "waypoint.hashicorp.com/id"
	metaNonce = "waypoint.hashicorp.com/nonce"
)

// Config is the configuration structure for the Platform.
type Config struct {
	Jobspec string `hcl:"jobspec"`
	JobVars map[string]string `hcl:"job_vars,optional"`

	// The Nomad region to deploy to, defaults to "global"
	Region string `hcl:"region,optional"`

	AllowFS bool `hcl:"allow_fs,optional"`

	// The namespace of the job
	Namespace string `hcl:"namespace,optional"`

	// Environment variables that are meant to configure the application in a static
	// way. This might be control an image that has multiple modes of operation,
	// selected via environment variable. Most configuration should use the waypoint
	// config commands.
	StaticEnvVars map[string]string `hcl:"static_environment,optional"`

	// Port that your service is running on within the actual container.
	// Defaults to port 3000.
	// TODO Evaluate if this should remain as a default 3000, should be a required field,
	// or default to another port.
	ServicePort uint `hcl:"service_port,optional"`
}

// AuthConfig maps the the Nomad Docker driver 'auth' config block
// and is used to set credentials for pulling images from the registry
type AuthConfig struct {
	Username string `hcl:"username"`
	Password string `hcl:"password"`
}

// Platform is the Platform implementation for Nomad.
type Platform struct {
	config Config
}

// Config implements Configurable
func (p *Platform) Config() (interface{}, error) {
	return &p.config, nil
}

// Implement ConfigurableNotify
// func (p *Platform) ConfigSet(config interface{}) error {
// 	c, ok := config.(*DeployConfig)
// 	if !ok {
// 		// The Waypoint SDK should ensure this never gets hit
// 		return fmt.Errorf("Expected *DeployConfig as parameter")
// 	}

// 	// validate the config
// 	if c.Region == "" {
// 		return fmt.Errorf("Region must be set to a valid directory")
// 	}

// 	return nil
// }

// Implement Builder
func (p *Platform) DeployFunc() interface{} {
	// return a function which will be called by Waypoint
	return p.deploy
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

// In addition to default input parameters the registry.Artifact from the Build step
// can also be injected.
//
// The output parameters for BuildFunc must be a Struct which can
// be serialzied to Protocol Buffers binary format and an error.
// This Output Value will be made available for other functions
// as an input parameter.
// If an error is returned, Waypoint stops the execution flow and
// returns an error to the user.
// func (b *Platform) deploy(ctx context.Context, ui terminal.UI, artifact *registry.Artifact) (*Deployment, error) {
// 	u := ui.Status()
// 	defer u.Close()
// 	u.Update("Deploy application")

// 	return &Deployment{}, nil
// }

// Deploy deploys an image to Nomad.
func (p *Platform) deploy(
	ctx context.Context,
	log hclog.Logger,
	src *component.Source,
	img *docker.Image,
	deployConfig *component.DeploymentConfig,
	ui terminal.UI,
) (*Deployment, error) {
	// Create our deployment and set an initial ID
	var result Deployment
	id, err := component.Id()
	if err != nil {
		return nil, err
	}
	result.Id = id
	result.Name = strings.ToLower(fmt.Sprintf("%s-%s", src.App, id))

	log.Debug("hullo deploying ID / NAME:", result.Id, result.Name)
	// We'll update the user in real time
	st := ui.Status()
	defer st.Close()

	// Get our client
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		return nil, err
	}
	jobclient := client.Jobs()

	if p.config.ServicePort == 0 {
		p.config.ServicePort = 3000
	}

	// Build our env vars
	env := map[string]string{
		"PORT": fmt.Sprint(p.config.ServicePort),
	}

	for k, v := range p.config.StaticEnvVars {
		env[k] = v
	}

	for k, v := range deployConfig.Env() {
		env[k] = v
	}

	// jobEnvVars := map[string]string{
	jobEnvVars := map[string]interface{}{
		"NOMAD_VAR_waypoint_env":          env,
		// "NOMAD_VAR_waypoint_env":          fmt.Sprintf("%s", envString),
		"NOMAD_VAR_waypoint_image":        img.Name(),
		"NOMAD_VAR_waypoint_job_name":     result.Name,
		"NOMAD_VAR_waypoint_service_port": p.config.ServicePort,
	}
	for k, v := range p.config.JobVars {
    jobEnvVars[k] = v
	}

	jobEnvs := make([]string, len(jobEnvVars))
	for key, value := range jobEnvVars {
		if _, ok := value.(string); ok {
			jobEnvs = append(jobEnvs, fmt.Sprintf("%s=%s", key, value))
			continue
		}

		jsonValue, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("error applying jobspec: %s", err)
		}
		jobEnvs = append(jobEnvs, fmt.Sprintf("%s=%s", key, jsonValue))
	}
	log.Debug("Job env vars string slice: ", jobEnvs)
	// Determine if we have a job that we manage already
	job, _, err := jobclient.Info(result.Name, &api.QueryOptions{})
	if strings.Contains(err.Error(), "job not found") {
		job, err = jobspec2.ParseWithConfig(&jobspec2.ParseConfig{
			Path:    "", // IDK WHAT THIS IS FOR
			Body:    []byte(p.config.Jobspec),  // THE USER SUPPLIED JOBSPEC
			AllowFS: p.config.AllowFS, // FLAG SET BY THE USER. DEFAULTS TO TRUE
			Strict:  true, // SEEMS GOOD TO BE STRICT?
			Envs:    jobEnvs, // 
		})
		if err != nil {
			return nil, fmt.Errorf("error parsing jobspec config: %s", err)
		}

		job.ID = &result.Name
		job.Name = &result.Name

		err = nil
	}
	if err != nil {
		return nil, err
	}

	// Set our ID on the meta.
	job.SetMeta(metaId, result.Id)
	job.SetMeta(metaNonce, time.Now().UTC().Format(time.RFC3339Nano))

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

	if err := newMonitor(st, client).monitor(evalID); err != nil {
		return nil, err
	}
	st.Step(terminal.StatusOK, "Deployment successfully rolled out!")

	return &result, nil
}

func (p *Platform) Documentation() (*docs.Documentation, error) {
	doc, err := docs.New(docs.FromConfig(&Config{}), docs.FromFunc(p.DeployFunc()))
	if err != nil {
		return nil, err
	}

	doc.Description("Deploy to a nomad cluster as a service using docker")

	doc.Example(
		`
deploy {
        use "nomad" {
          region = "global"
          datacenter = "dc1"
          auth = {
            username = "username"
            password = "password"
          }
          static_environment = {
            "environment": "production",
            "LOG_LEVEL": "debug"
          }
          service_port = 3000
          replicas = 1
        }
}
`)

	doc.SetField(
		"region",
		"The Nomad region to deploy the job to.",
		docs.Default("global"),
	)

	doc.SetField(
		"datacenter",
		"The Nomad datacenter to deploy the job to.",
		docs.Default("dc1"),
	)

	doc.SetField(
		"namespace",
		"The Nomad namespace to deploy the job to.",
	)

	doc.SetField(
		"replicas",
		"The replica count for the job.",
		docs.Default("1"),
	)

	doc.SetField(
		"auth",
		"The credentials for docker registry.",
	)

	doc.SetField(
		"static_environment",
		"Environment variables to add to the job.",
	)

	doc.SetField(
		"service_port",
		"TCP port the job is listening on.",
	)

	return doc, nil
}

var (
	_ component.Platform     = (*Platform)(nil)
	_ component.Configurable = (*Platform)(nil)
	_ component.Destroyer    = (*Platform)(nil)
)
