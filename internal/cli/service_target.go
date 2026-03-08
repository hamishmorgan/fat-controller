package cli

import "github.com/hamishmorgan/fat-controller/internal/app"

// serviceTarget is a CLI-local alias for app.ServiceTarget.
//
// The CLI keeps the unexported name so testable command cores can remain stable
// while service target resolution lives in the app layer.
type serviceTarget = app.ServiceTarget
