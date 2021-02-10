package release

import (
	"context"
	"fmt"
	"regexp"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/waypoint-plugin-sdk/terminal"
	"github.com/jeffwecan/waypoint-plugin-nomad-traefik/platform"

	"github.com/hashicorp/waypoint-plugin-sdk/component"
	"github.com/hashicorp/nomad/api"
)

type ReleaseConfig struct {
	Domain string `hcl:"domain"`
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
	return rm.release
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
func (rm *ReleaseManager) release(ctx context.Context, ui terminal.UI, target *platform.Deployment, log hclog.Logger) (*Release, error) {
	u := ui.Status()
	log.Debug("release thinger", target)
	defer u.Close()

	log.Debug("Attempting to find job %s...", target.Name)
	client, err := api.NewClient(api.DefaultConfig())
	jobclient := client.Jobs()
	// find existing job / deployment
	job, _, err := jobclient.Info(target.Name, &api.QueryOptions{})
	if err != nil {
		// if not existing one bomb out
		return nil, err
	}
	// our magic tag thing?
	re := regexp.MustCompile("waypoint.release-router=(.*)")

	for _, tg := range job.TaskGroups {
		// log.Debug("%s: tg.services::", tg.Name)
		for _, svc := range tg.Services {
			log.Debug("task group service tags", tg.Name, svc.Name, svc.Tags)
			routerName := ""
			for _, tag := range svc.Tags {
				// See if this service has our magic tag thing
				match := re.FindStringSubmatch(tag)
				if len(match) >= 1 {
					routerName = match[1]
				}
			}
			if routerName != "" {
				svc.Tags = append(svc.Tags, fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s`)", routerName, rm.config.Domain))
				log.Debug("updated task group service tags", tg.Name, svc.Name, svc.Tags)
			}
		}
		// for _, task := range tg.Tasks {
		// 	// log.Debug("%s: task.services::", task.Name)
		// 	for _, svc := range task.Services {
		// 		log.Debug("task service tags", task.Name, svc.Name, svc.Tags)
		// 	}
		// }
	}
	log.Debug("Job found!: %s", job.ID)

	// Register job
	u.Update("Updating job...")
	regResult, _, err := jobclient.Register(job, nil)
	if err != nil {
		return nil, err
	}

	evalID := regResult.EvalID
	log.Debug("released job evalID", evalID)

	u.Step(terminal.StatusOK, "Deployment successfully releaseddd!")

	// Create our deployment and set an initial ID
	var result Release
	result.Id = target.Id
	result.Name = target.Name
	result.Url = fmt.Sprintf("https://%s", rm.config.Domain)
	return &result, nil
}

// URL is a URL.
func (r *Release) URL() string { return r.Url }

var (
	_ component.ReleaseManager = (*ReleaseManager)(nil)
	_ component.Configurable   = (*ReleaseManager)(nil)
)
