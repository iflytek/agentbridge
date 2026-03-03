package parser

import (
	"fmt"
	"github.com/iflytek/agentbridge/internal/models"
	"strings"
)

// EndNodeParser parses Coze end nodes.
type EndNodeParser struct {
	*BaseNodeParser
	skippedNodeIDs map[string]bool // Track skipped node IDs
}

func NewEndNodeParser(vrs *models.VariableReferenceSystem) NodeParser {
	return &EndNodeParser{
		BaseNodeParser: NewBaseNodeParser("2", vrs),
		skippedNodeIDs: make(map[string]bool),
	}
}

// SetSkippedNodeIDs sets skipped node IDs.
func (p *EndNodeParser) SetSkippedNodeIDs(skippedNodeIDs map[string]bool) {
	p.skippedNodeIDs = skippedNodeIDs
}

// GetSupportedType returns supported node type.
func (p *EndNodeParser) GetSupportedType() string {
	return "2"
}

// ParseNode parses Coze end node.
func (p *EndNodeParser) ParseNode(cozeNode CozeNode) (*models.Node, error) {
	// Validate node
	if err := p.ValidateNode(cozeNode); err != nil {
		return nil, err
	}

	// Create basic node information
	node := p.parseBasicNodeInfo(cozeNode)
	node.Type = models.NodeTypeEnd

	// Parse end node specific configuration
	config := models.EndConfig{
		OutputMode:   "template", // Coze uses template mode by default
		StreamOutput: true,       // Default to streaming output based on schema
	}

	// Parse template from node inputs if available - check both formats
	var inputParams []CozeNodeInputParam
	if cozeNode.Data.Inputs != nil {
		if cozeNode.Data.Inputs.InputParameters != nil {
			inputParams = cozeNode.Data.Inputs.InputParameters
		} else if cozeNode.Data.Inputs.InputParametersAlt != nil {
			inputParams = cozeNode.Data.Inputs.InputParametersAlt
		}
	}

	if len(inputParams) > 0 {
		// For Coze end nodes, inputs represent what gets output
		for _, param := range inputParams {
			input := models.Input{
				Name:        param.Name,
				Type:        p.convertDataType(param.Input.Type),
				Description: "",
			}

			// Parse variable reference
			if param.Input.Value.Type == "ref" {
				sourceNodeID := param.Input.Value.Content.BlockID

				// Skip references from filtered nodes if needed
				if p.skippedNodeIDs != nil && p.skippedNodeIDs[sourceNodeID] {
					continue // Skip this input
				}

				outputName := param.Input.Value.Content.Name

				// Apply variable reference system mapping first (for iteration output mappings)
				if p.variableRefSystem != nil {
					outputName = p.variableRefSystem.ResolveOutputName(sourceNodeID, outputName)
				}

				// Then apply platform-specific output name mapping
				outputName = p.mapOutputName(outputName)

				input.Reference = &models.VariableReference{
					Type:       models.ReferenceTypeNodeOutput,
					NodeID:     sourceNodeID,
					OutputName: outputName,
					DataType:   p.convertDataType(param.Input.Type),
				}
			}

			node.Inputs = append(node.Inputs, input)
		}
	}

	// Set template from exit configuration if available
	if cozeNode.Data.Inputs != nil && cozeNode.Data.Inputs.Exit != nil {
		if cozeNode.Data.Inputs.Exit.TerminatePlan == "returnVariables" {
			config.OutputMode = "variables"
		}
	}

	// Try to extract template from schema data if available
	if len(node.Inputs) > 0 {
		// Generate template from inputs
		template := p.generateTemplateFromInputs(node.Inputs)
		config.Template = template
	}

	node.Config = config

	return node, nil
}

// ValidateNode validates Coze end node.
func (p *EndNodeParser) ValidateNode(cozeNode CozeNode) error {
	// Use base validation
	if err := p.BaseNodeParser.ValidateNode(cozeNode); err != nil {
		return err
	}

	// End node specific validation - support both numeric and string types
	if cozeNode.Type != "2" && cozeNode.Type != "end" {
		return fmt.Errorf("node type must be '2' or 'end', got '%s'", cozeNode.Type)
	}

	return nil
}

// generateTemplateFromInputs generates a template string from inputs
func (p *EndNodeParser) generateTemplateFromInputs(inputs []models.Input) string {
	if len(inputs) == 0 {
		return ""
	}

	// Generate template with proper formatting
	var templateParts []string
	for _, input := range inputs {
		if input.Reference != nil {
			// Generate variable reference with type information
			templateVar := fmt.Sprintf("{{%s}}", input.Name)
			templateParts = append(templateParts, templateVar)
		} else {
			// Handle literal values
			templateVar := fmt.Sprintf("{{%s}}", input.Name)
			templateParts = append(templateParts, templateVar)
		}
	}

	return strings.Join(templateParts, ", ")
}

// mapOutputName maps output names with platform-specific handling
func (p *EndNodeParser) mapOutputName(outputName string) string {
	// Coze -> Unified DSL output name mapping
	switch outputName {
	case "text":
		// Coze LLM node output mapping
		return "output"
	case "result":
		// Coze code node output mapping
		return "result"
	case "answer":
		// Coze QA node output mapping
		return "answer"
	default:
		// Pass through other output names unchanged
		return outputName
	}
}
