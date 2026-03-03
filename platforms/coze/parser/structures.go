package parser

// CozeDSL represents the root structure of Coze DSL
type CozeDSL struct {
	WorkflowID     string         `yaml:"workflowid" json:"workflowid"`
	Name           string         `yaml:"name" json:"name"`
	Description    string         `yaml:"description" json:"description"`
	Version        string         `yaml:"version" json:"version"`
	CreateTime     int64          `yaml:"createtime" json:"createtime"`
	UpdateTime     int64          `yaml:"updatetime" json:"updatetime"`
	Schema         CozeSchema     `yaml:"schema" json:"schema"`
	Nodes          []CozeNode     `yaml:"nodes" json:"nodes"`
	Edges          []CozeRootEdge `yaml:"edges" json:"edges"`
	Metadata       CozeMetadata   `yaml:"metadata" json:"metadata"`
	Dependencies   []CozeDep      `yaml:"dependencies" json:"dependencies"`
	ExportFormat   string         `yaml:"exportformat" json:"exportformat"`
	SerializedData string         `yaml:"serializeddata" json:"serializeddata"`
}

// CozeSchema contains schema information
type CozeSchema struct {
	Edges    []CozeSchemaEdge `yaml:"edges" json:"edges"`
	Nodes    []CozeSchemaNode `yaml:"nodes" json:"nodes"`
	Versions CozeVersions     `yaml:"versions" json:"versions"`
}

// CozeSchemaEdge represents schema edge
type CozeSchemaEdge struct {
	SourceNodeID string `yaml:"sourceNodeID" json:"sourceNodeID"`
	SourcePortID string `yaml:"sourcePortID,omitempty" json:"sourcePortID,omitempty"`
	TargetNodeID string `yaml:"targetNodeID" json:"targetNodeID"`
	TargetPortID string `yaml:"targetPortID,omitempty" json:"targetPortID,omitempty"`
}

// CozeSchemaNode represents schema node
type CozeSchemaNode struct {
	Data   CozeSchemaNodeData `yaml:"data" json:"data"`
	ID     string             `yaml:"id" json:"id"`
	Meta   CozeNodeMeta       `yaml:"meta" json:"meta"`
	Type   string             `yaml:"type" json:"type"`
	Blocks []interface{}      `yaml:"blocks,omitempty" json:"blocks,omitempty"` // For iteration nodes
	Edges  []interface{}      `yaml:"edges,omitempty" json:"edges,omitempty"`   // For iteration nodes
}

// CozeSchemaNodeData contains schema node data
type CozeSchemaNodeData struct {
	NodeMeta          CozeNodeMetaInfo `yaml:"nodeMeta" json:"nodeMeta"`
	Outputs           []CozeOutput     `yaml:"outputs,omitempty" json:"outputs,omitempty"`
	TriggerParameters []CozeOutput     `yaml:"trigger_parameters,omitempty" json:"trigger_parameters,omitempty"`
	Inputs            *CozeInputs      `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	TerminatePlan     string           `yaml:"terminatePlan,omitempty" json:"terminatePlan,omitempty"`
}

// CozeNodeMetaInfo contains node metadata info
type CozeNodeMetaInfo struct {
	Description string `yaml:"description" json:"description"`
	Icon        string `yaml:"icon" json:"icon"`
	SubTitle    string `yaml:"subTitle" json:"subTitle"`
	Title       string `yaml:"title" json:"title"`
}

// CozeInputs contains node inputs
type CozeInputs struct {
	InputParameters []CozeInputParam `yaml:"inputParameters" json:"inputParameters"`
}

// CozeInputParam represents input parameter
type CozeInputParam struct {
	Name  string    `yaml:"name" json:"name"`
	Input CozeInput `yaml:"input" json:"input"`
}

// CozeInput represents input configuration
type CozeInput struct {
	Type  string         `yaml:"type" json:"type"`
	Value CozeInputValue `yaml:"value" json:"value"`
}

// CozeInputValue represents input value
type CozeInputValue struct {
	Content CozeInputContent `yaml:"content" json:"content"`
	RawMeta CozeRawMeta      `yaml:"rawMeta" json:"rawMeta"`
	Type    string           `yaml:"type" json:"type"`
}

// CozeInputContent represents input content
type CozeInputContent struct {
	BlockID string `yaml:"blockID" json:"blockID"`
	Name    string `yaml:"name" json:"name"`
	Source  string `yaml:"source" json:"source"`
}

// CozeRawMeta contains raw metadata
type CozeRawMeta struct {
	Type int `yaml:"type" json:"type"`
}

// CozeVersions contains version information
type CozeVersions struct {
	Loop string `yaml:"loop" json:"loop"`
}

// CozeNode represents a node in the workflow
type CozeNode struct {
	ID   string `yaml:"id" json:"id"`
	Type string `yaml:"type" json:"type"`

	// New format fields (root level) - for newer Coze YAML exports
	Title       string        `yaml:"title,omitempty" json:"title,omitempty"`
	Description string        `yaml:"description,omitempty" json:"description,omitempty"`
	Icon        string        `yaml:"icon,omitempty" json:"icon,omitempty"`
	Position    *CozePosition `yaml:"position,omitempty" json:"position,omitempty"`
	Parameters  interface{}   `yaml:"parameters,omitempty" json:"parameters,omitempty"`

	// Old format fields (nested) - for older Coze exports
	Meta CozeNodeMeta `yaml:"meta,omitempty" json:"meta,omitempty"`
	Data CozeNodeData `yaml:"data,omitempty" json:"data,omitempty"`

	Blocks  []interface{} `yaml:"blocks,omitempty" json:"blocks,omitempty"`
	Edges   []interface{} `yaml:"edges,omitempty" json:"edges,omitempty"`
	Version string        `yaml:"version,omitempty" json:"version,omitempty"`
}

// CozeNodeMeta contains node positioning metadata
type CozeNodeMeta struct {
	Position CozePosition `yaml:"position" json:"position"`
}

// CozePosition represents node position
type CozePosition struct {
	X float64 `yaml:"x" json:"x"`
	Y float64 `yaml:"y" json:"y"`
}

// CozeNodeData contains node configuration data
type CozeNodeData struct {
	Meta    CozeDataMeta    `yaml:"meta" json:"meta"`
	Outputs []CozeOutput    `yaml:"outputs,omitempty" json:"outputs,omitempty"`
	Inputs  *CozeNodeInputs `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	Size    interface{}     `yaml:"size" json:"size"`
}

// CozeDataMeta contains data metadata
type CozeDataMeta struct {
	Title       string `yaml:"title" json:"title"`
	Description string `yaml:"description" json:"description"`
	Icon        string `yaml:"icon" json:"icon"`
	Subtitle    string `yaml:"subTitle" json:"subTitle"`
	MainColor   string `yaml:"maincolor" json:"maincolor"`
}

// CozeNodeInputs contains node inputs for the main nodes section
type CozeNodeInputs struct {
	InputParameters    []CozeNodeInputParam `yaml:"inputParameters,omitempty" json:"inputParameters,omitempty"`
	InputParametersAlt []CozeNodeInputParam `yaml:"inputparameters,omitempty" json:"inputparameters,omitempty"` // Alternative lowercase version
	Branches           []interface{}        `yaml:"branches,omitempty" json:"branches,omitempty"`               // For selector nodes
	SettingOnError     interface{}          `yaml:"settingonerror" json:"settingonerror"`
	NodeBatchInfo      interface{}          `yaml:"nodebatchinfo" json:"nodebatchinfo"`
	LLMParam           interface{}          `yaml:"llmparam" json:"llmparam"`
	OutputEmitter      interface{}          `yaml:"outputemitter" json:"outputemitter"`
	Exit               *CozeExit            `yaml:"exit,omitempty" json:"exit,omitempty"`
	LLM                interface{}          `yaml:"llm" json:"llm"`
	Loop               interface{}          `yaml:"loop" json:"loop"`
	Selector           interface{}          `yaml:"selector" json:"selector"`
	TextProcessor      interface{}          `yaml:"textprocessor" json:"textprocessor"`
	SubWorkflow        interface{}          `yaml:"subworkflow" json:"subworkflow"`
	IntentDetector     interface{}          `yaml:"intentdetector" json:"intentdetector"`
	DatabaseNode       interface{}          `yaml:"databasenode" json:"databasenode"`
	HTTPRequestNode    interface{}          `yaml:"httprequestnode" json:"httprequestnode"`
	Knowledge          interface{}          `yaml:"knowledge" json:"knowledge"`
	CodeRunner         interface{}          `yaml:"coderunner" json:"coderunner"`
	PluginAPIParam     interface{}          `yaml:"pluginapiparam" json:"pluginapiparam"`
	VariableAggregator interface{}          `yaml:"variableaggregator" json:"variableaggregator"`
	VariableAssigner   interface{}          `yaml:"variableassigner" json:"variableassigner"`
	QA                 interface{}          `yaml:"qa" json:"qa"`
	Batch              interface{}          `yaml:"batch" json:"batch"`
	Comment            interface{}          `yaml:"comment" json:"comment"`
	InputReceiver      interface{}          `yaml:"inputreceiver" json:"inputreceiver"`
}

// CozeNodeInputParam represents node input parameter
type CozeNodeInputParam struct {
	Name      string        `yaml:"name" json:"name"`
	Input     CozeNodeInput `yaml:"input" json:"input"`
	Left      interface{}   `yaml:"left" json:"left"`
	Right     interface{}   `yaml:"right" json:"right"`
	Variables []interface{} `yaml:"variables" json:"variables"`
}

// CozeNodeInput represents node input
type CozeNodeInput struct {
	Type  string             `yaml:"Type" json:"Type"`
	Value CozeNodeInputValue `yaml:"Value" json:"Value"`
}

// CozeNodeInputValue represents node input value
type CozeNodeInputValue struct {
	Type    string               `yaml:"type" json:"type"`
	Content CozeNodeInputContent `yaml:"content" json:"content"`
	RawMeta CozeNodeInputRawMeta `yaml:"rawmeta" json:"rawmeta"`
}

// CozeNodeInputContent represents node input content
type CozeNodeInputContent struct {
	BlockID string `yaml:"blockID" json:"blockID"`
	Name    string `yaml:"name" json:"name"`
	Source  string `yaml:"source" json:"source"`
}

// CozeNodeInputRawMeta contains node input raw metadata
type CozeNodeInputRawMeta struct {
	Type int `yaml:"type" json:"type"`
}

// CozeExit contains exit configuration for end nodes
type CozeExit struct {
	TerminatePlan string `yaml:"terminateplan" json:"terminateplan"`
}

// CozeOutput represents node output specification
type CozeOutput struct {
	Name     string      `yaml:"name" json:"name"`
	Required bool        `yaml:"required" json:"required"`
	Type     string      `yaml:"type" json:"type"`
	Schema   interface{} `yaml:"schema,omitempty" json:"schema,omitempty"` // Flexible schema support for arrays, objects, etc.
}

// CozeOutputSchema represents output schema information - support flexible schema formats
type CozeOutputSchema struct {
	Type   string      `yaml:"type,omitempty" json:"type,omitempty"`
	Schema interface{} `yaml:"schema,omitempty" json:"schema,omitempty"` // Flexible schema support
	Name   string      `yaml:"name,omitempty" json:"name,omitempty"`
}

// CozeEdge represents connection between nodes (schema format)
type CozeEdge struct {
	FromNode string `yaml:"sourceNodeID" json:"sourceNodeID"`
	FromPort string `yaml:"sourcePortID" json:"sourcePortID"`
	ToNode   string `yaml:"targetNodeID" json:"targetNodeID"`
	ToPort   string `yaml:"targetPortID" json:"targetPortID"`
}

// CozeRootEdge represents connection between nodes (root level format)
type CozeRootEdge struct {
	FromNode string `yaml:"from_node" json:"from_node"`
	FromPort string `yaml:"from_port" json:"from_port"`
	ToNode   string `yaml:"to_node" json:"to_node"`
	ToPort   string `yaml:"to_port" json:"to_port"`
}

// CozeMetadata contains workflow metadata
type CozeMetadata struct {
	ContentType string `yaml:"content_type" json:"content_type"`
	CreatorID   string `yaml:"creator_id" json:"creator_id"`
	Mode        string `yaml:"mode" json:"mode"`
	SpaceID     string `yaml:"space_id" json:"space_id"`
}

// CozeDep represents dependency information
type CozeDep struct {
	Metadata     CozeDepMetadata `yaml:"metadata" json:"metadata"`
	ResourceID   string          `yaml:"resource_id" json:"resource_id"`
	ResourceName string          `yaml:"resource_name" json:"resource_name"`
	ResourceType string          `yaml:"resource_type" json:"resource_type"`
}

// CozeDepMetadata contains dependency metadata
type CozeDepMetadata struct {
	NodeType string `yaml:"node_type" json:"node_type"`
}
