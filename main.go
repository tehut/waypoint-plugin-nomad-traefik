package main

import (
	sdk "github.com/hashicorp/waypoint-plugin-sdk"
	"github.com/jeffwecan/waypoint-plugin-nomad-traefik/platform"
	"github.com/jeffwecan/waypoint-plugin-nomad-traefik/release"
)

func main() {

	sdk.Main(sdk.WithComponents(
		&platform.Platform{},
		&release.ReleaseManager{},
	))
}
