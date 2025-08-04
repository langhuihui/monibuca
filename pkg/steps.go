package pkg

// StepName is a typed alias for all workflow step identifiers.
type StepName string

// StepDef defines a step with typed name and description.
type StepDef struct {
	Name        StepName
	Description string
}

// Standard, cross-plugin step name constants for pull/publish workflows.
// Plugin-specific step names should be defined in their respective plugin packages.
const (
	StepPublish    StepName = "publish"
	StepURLParsing StepName = "url_parsing"
	StepConnection StepName = "connection"
	StepHandshake  StepName = "handshake"
	StepParsing    StepName = "parsing"
	StepStreaming  StepName = "streaming"
)
