package apply

import (
	"fmt"
	"strings"

	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// ToServiceInstanceUpdateInput converts a DesiredDeploy to the Railway
// GraphQL input type. Only non-nil fields are set; others remain nil
// (Railway treats nil as "don't change").
func ToServiceInstanceUpdateInput(desired *config.DesiredDeploy) (railway.ServiceInstanceUpdateInput, error) {
	var input railway.ServiceInstanceUpdateInput
	if desired == nil {
		return input, nil
	}

	if desired.Builder != nil {
		b, ok := parseBuilder(*desired.Builder)
		if !ok {
			return input, fmt.Errorf("unknown builder: %q (valid: NIXPACKS, RAILPACK, PAKETO, HEROKU)", *desired.Builder)
		}
		input.Builder = &b
	}
	input.DockerfilePath = desired.DockerfilePath
	input.RootDirectory = desired.RootDirectory
	input.StartCommand = desired.StartCommand
	input.HealthcheckPath = desired.HealthcheckPath

	return input, nil
}

func parseBuilder(value string) (railway.Builder, bool) {
	switch strings.ToUpper(value) {
	case "NIXPACKS":
		return railway.BuilderNixpacks, true
	case "RAILPACK":
		return railway.BuilderRailpack, true
	case "PAKETO":
		return railway.BuilderPaketo, true
	case "HEROKU":
		return railway.BuilderHeroku, true
	default:
		return "", false
	}
}
